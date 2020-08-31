// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package idb

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"

	"github.com/algorand/indexer/types"
)

const migrationMetastateKey = "migration"

type MigrationState struct {
	NextMigration int `json:"next"`

	// Used for a long migration to checkpoint progress
	NextRound int64 `json:"round,omitempty"`

	NextAssetId int64 `json:"assetid,omitempty"`
}

// A migration function should take care of writing back to metastate migration row
type migrationFunc func(*PostgresIndexerDb, *MigrationState) error

type migrationStruct struct {
	migrate migrationFunc

	// The system should wait for this migration to finish before trying to import new data or serve data.
	preventStartup bool
}

var migrations []migrationStruct

func anyBlockers(nextMigration int) bool {
	for i := nextMigration; i < len(migrations); i++ {
		if migrations[i].preventStartup {
			return true
		}
	}
	return false
}

func (db *PostgresIndexerDb) runAvailableMigrations(migrationStateJson string) (err error) {
	var state MigrationState
	if len(migrationStateJson) > 0 {
		err = json.Decode([]byte(migrationStateJson), &state)
		if err != nil {
			return fmt.Errorf("bad metastate migration json, %v", err)
		}
	}

	if state.NextMigration >= len(migrations) {
		return nil
	}
	if anyBlockers(state.NextMigration) {
		// run migrations synchronously
		log.Print("Indexer startup will be delayed until database migration completes")
		return db.runMigrationState(&state, true)
	} else {
		log.Print("database migrations running in background...")
		go db.runMigrationState(&state, false)
		return nil
	}
}

func (db *PostgresIndexerDb) runMigrationState(state *MigrationState, blocking bool) (err error) {
	nextMigration := state.NextMigration
	for true {
		err = migrations[nextMigration].migrate(db, state)
		if err != nil {
			return fmt.Errorf("error in migration %d: %v", nextMigration, err)
		}
		nextMigration++
		if nextMigration >= len(migrations) {
			return nil
		}
		state.NextMigration = nextMigration
		if blocking && !anyBlockers(nextMigration) {
			log.Print("database migrations will complete in background...")
			go db.runMigrationState(state, false)
			return nil
		}
	}
	return nil
}

// after setting up a new database, mark state as if all migrations had been done
func (db *PostgresIndexerDb) markMigrationsAsDone() (err error) {
	state := MigrationState{
		NextMigration: len(migrations),
	}
	migrationStateJson := string(json.Encode(state))
	return db.SetMetastate(migrationMetastateKey, migrationStateJson)
}

func m0fixupTxid(db *PostgresIndexerDb, state *MigrationState) error {
	mtxid := &txidFiuxpMigrationContext{db: db, state: state}
	return mtxid.asyncTxidFixup()
}

func m1fixupBlockTime(db *PostgresIndexerDb, state *MigrationState) error {
	sqlLines := []string{
		`UPDATE block_header SET realtime = to_timestamp(coalesce(header ->> 'ts', '0')::bigint) AT TIME ZONE 'UTC'`,
	}
	return sqlMigration(db, state, sqlLines)
}

func m2apps(db *PostgresIndexerDb, state *MigrationState) error {
	sqlLines := []string{
		`CREATE TABLE IF NOT EXISTS app (
  index bigint PRIMARY KEY,
  creator bytea, -- account address
  params jsonb
);`,
		`CREATE TABLE IF NOT EXISTS account_app (
  addr bytea,
  app bigint,
  localstate jsonb,
  PRIMARY KEY (addr, app)
);`,
	}
	return sqlMigration(db, state, sqlLines)
}

func m3acfgFix(db *PostgresIndexerDb, state *MigrationState) (err error) {
	log.Print("asset config fix migration starting")
	rows, err := db.db.Query(`SELECT index FROM asset WHERE index >= $1 ORDER BY 1`, state.NextAssetId)
	if err != nil {
		log.Printf("acfg fix err getting assetids, %v", err)
		return err
	}
	assetIds := make([]int64, 0, 1000)
	for rows.Next() {
		var aid int64
		err = rows.Scan(&aid)
		if err != nil {
			log.Printf("acfg fix err getting assetid row, %v", err)
			rows.Close()
			return
		}
		assetIds = append(assetIds, aid)
	}
	rows.Close()
	for true {
		nexti, err := m3acfgFixAsyncInner(db, state, assetIds)
		if err != nil {
			log.Printf("acfg fix chunk, %v", err)
			return err
		}
		if nexti < 0 {
			break
		}
		assetIds = assetIds[nexti:]
	}
	log.Print("acfg fix migration finished")
	return nil
}

// do a transactional batch of asset fixes
// updates asset rows and metastate
func m3acfgFixAsyncInner(db *PostgresIndexerDb, state *MigrationState, assetIds []int64) (next int, err error) {
	lastlog := time.Now()
	tx, err := db.db.Begin()
	if err != nil {
		log.Printf("acfg fix tx begin, %v", err)
		return -1, err
	}
	defer tx.Rollback() // ignored if .Commit() first
	setacfg, err := tx.Prepare(`INSERT INTO asset (index, creator_addr, params) VALUES ($1, $2, $3) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params`)
	if err != nil {
		log.Printf("acfg fix prepare set asset, %v", err)
		return
	}
	defer setacfg.Close()
	for i, aid := range assetIds {
		now := time.Now()
		if now.Sub(lastlog) > (5 * time.Second) {
			log.Printf("acfg fix next=%d", aid)
			state.NextAssetId = aid
			migrationStateJson := json.Encode(state)
			_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJson)
			if err != nil {
				log.Printf("acfg fix migration %d meta err: %v", state.NextMigration, err)
				return -1, err
			}
			err = tx.Commit()
			if err != nil {
				log.Printf("acfg fix migration %d commit err: %v", state.NextMigration, err)
				return -1, err
			}
			return i, nil
		}
		txrows := db.txTransactions(tx, TransactionFilter{TypeEnum: 3, AssetId: uint64(aid)})
		prevRound := uint64(0)
		first := true
		var params types.AssetParams
		var creator types.Address
		for txrow := range txrows {
			if txrow.Round < prevRound {
				log.Printf("acfg rows out of order %d < %d", txrow.Round, prevRound)
				return
			}
			var stxn types.SignedTxnInBlock
			err = msgpack.Decode(txrow.TxnBytes, &stxn)
			if err != nil {
				log.Printf("acfg fix bad txn bytes %d:%d, %v", txrow.Round, txrow.Intra, err)
				return
			}
			if first {
				params = stxn.Txn.AssetParams
				creator = stxn.Txn.Sender
				first = false
			} else if stxn.Txn.AssetParams == (types.AssetParams{}) {
				// delete asset
				params = stxn.Txn.AssetParams
			} else {
				params = types.MergeAssetConfig(params, stxn.Txn.AssetParams)
			}
		}
		outparams := json.Encode(params)
		_, err = setacfg.Exec(aid, creator[:], outparams)
		if err != nil {
			log.Printf("acfg fix asset update, %v", err)
			return -1, err
		}
	}
	state.NextAssetId = 0
	state.NextMigration++
	migrationStateJson := json.Encode(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJson)
	if err != nil {
		log.Printf("acfg fix migration %d meta err: %v", state.NextMigration, err)
		return -1, err
	}
	err = tx.Commit()
	if err != nil {
		log.Printf("acfg fix migration %d commit err: %v", state.NextMigration, err)
		return -1, err
	}
	return -1, nil
}

func m3acfgFixFlush(db *PostgresIndexerDb, state *MigrationState, tx *sql.Tx, out [][]interface{}) error {
	return nil
}

func sqlMigration(db *PostgresIndexerDb, state *MigrationState, sqlLines []string) error {
	thisMigration := state.NextMigration
	tx, err := db.db.Begin()
	if err != nil {
		log.Printf("migration %d tx err: %v", thisMigration, err)
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first
	for i, cmd := range sqlLines {
		_, err = tx.Exec(cmd)
		if err != nil {
			log.Printf("migration %d sql[%d] err: %v", thisMigration, i, err)
			return err
		}
	}
	state.NextMigration++
	migrationStateJson := json.Encode(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJson)
	if err != nil {
		log.Printf("migration %d meta err: %v", thisMigration, err)
		return err
	}
	err = tx.Commit()
	if err != nil {
		log.Printf("migration %d commit err: %v", thisMigration, err)
		return err
	}
	log.Printf("migration %d done", thisMigration)
	return nil
}

func init() {
	migrations = []migrationStruct{
		// {func, preventStartup}
		{m0fixupTxid, false},
		{m1fixupBlockTime, true}, // UPDATE over all blocks
		{m2apps, true},           // schema change breaks other SQL
		{m3acfgFix, false},
	}
}

const txidMigrationErrMsg = "ERROR migrating txns for txid, stopped, will retry on next indexer startup"

type migrationContext struct {
	db      *PostgresIndexerDb
	state   *MigrationState
	lastlog time.Time
}
type txidFiuxpMigrationContext migrationContext

// read batches of at least 2 blocks or up to 10000 txns,
// write a temporary table, UPDATE from temporary table into txn.
// repeat until all txns consumed.
func (mtxid *txidFiuxpMigrationContext) asyncTxidFixup() (err error) {
	db := mtxid.db
	state := mtxid.state
	log.Print("txid fixup migration starting")
	prevRound := state.NextRound - 1
	txns := db.YieldTxns(context.Background(), prevRound)
	batch := make([]TxnRow, 15000)
	txInBatch := 0
	roundsInBatch := 0
	prevBatchRound := uint64(math.MaxUint64)
	for txr := range txns {
		if txr.Error != nil {
			log.Printf("ERROR migrating txns for txid rewrite: %v", txr.Error)
			log.Print("txidMigrationErrMsg")
			err = txr.Error
			return
		}
		if txr.Round != prevBatchRound {
			if txInBatch > 10000 {
				err = mtxid.putTxidFixupBatch(batch[:txInBatch])
				if err != nil {
					return
				}
				// start next batch
				batch[0] = txr
				txInBatch = 1
				roundsInBatch = 1
				prevBatchRound = txr.Round
				continue
			}
			roundsInBatch++
			prevBatchRound = txr.Round
		}
		batch[txInBatch] = txr
		txInBatch++
		if roundsInBatch > 2 && txInBatch > 10000 {
			// post the first complete rounds
			split := txInBatch - 1
			for batch[split].Round == txr.Round {
				split--
			}
			split++ // now the first txn of the incomplete current round
			err = mtxid.putTxidFixupBatch(batch[:split])
			if err != nil {
				return
			}
			// move incomplete round to next batch
			copy(batch, batch[split:txInBatch])
			txInBatch = txInBatch - split
			roundsInBatch = 1
			prevBatchRound = txr.Round
			continue
		}
	}
	if txInBatch > 0 {
		err = mtxid.putTxidFixupBatch(batch[:txInBatch])
		if err != nil {
			return
		}
	}
	// all done, mark migration state
	state.NextMigration++
	state.NextRound = 0
	migrationStateJson := string(json.Encode(state))
	err = db.SetMetastate(migrationMetastateKey, migrationStateJson)
	if err != nil {
		log.Printf("%s, error setting final migration state: %v", txidMigrationErrMsg, err)
		return
	}
	log.Print("txid fixup migration finished")
	return nil
}

type txidFixupRow struct {
	round uint64
	intra int
	txid  string // base32 string
}

func (mtxid *txidFiuxpMigrationContext) putTxidFixupBatch(batch []TxnRow) error {
	db := mtxid.db
	state := mtxid.state
	minRound := batch[0].Round
	maxRound := batch[0].Round
	for _, txr := range batch {
		if txr.Round < minRound {
			minRound = txr.Round
		}
		if txr.Round > maxRound {
			maxRound = txr.Round
		}
	}
	headers, err := mtxid.readHeaders(minRound, maxRound)
	if err != nil {
		return err
	}
	outrows := make([]txidFixupRow, len(batch))
	for i, txr := range batch {
		block := headers[txr.Round]
		proto, err := types.Protocol(string(block.CurrentProtocol))
		if err != nil {
			log.Printf("%s, proto: %v", txidMigrationErrMsg, err)
			return err
		}
		var stxn types.SignedTxnInBlock
		err = msgpack.Decode(txr.TxnBytes, &stxn)
		if err != nil {
			log.Printf("%s, txnb msgpack err: %v", txidMigrationErrMsg, err)
			return err
		}
		if stxn.HasGenesisID {
			stxn.Txn.GenesisID = block.GenesisID
		}
		if stxn.HasGenesisHash || proto.RequireGenesisHash {
			stxn.Txn.GenesisHash = block.GenesisHash
		}
		outrows[i].round = txr.Round
		outrows[i].intra = txr.Intra
		outrows[i].txid = crypto.TransactionIDString(stxn.Txn)
	}

	// do a transaction to update a batch
	tx, err := db.db.Begin()
	if err != nil {
		log.Printf("%s, batch tx err: %v", txidMigrationErrMsg, err)
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first
	// Check that migration state in db is still what we think it is
	row := tx.QueryRow(`SELECT v FROM metastate WHERE k = $1`, migrationMetastateKey)
	var migrationStateJson []byte
	err = row.Scan(&migrationStateJson)
	var txstate MigrationState
	if err == sql.ErrNoRows && state.NextMigration == 0 {
		// no previous state, ok
	} else if err != nil {
		log.Printf("%s, get m state err: %v", txidMigrationErrMsg, err)
		return err
	} else {
		err = json.Decode([]byte(migrationStateJson), &txstate)
		if err != nil {
			log.Printf("%s, migration json err: %v", txidMigrationErrMsg, err)
			return err
		}
		if state.NextMigration != txstate.NextMigration || state.NextRound != txstate.NextRound {
			log.Printf("%s, migration state changed when we werene't looking: %v -> %v", txidMigrationErrMsg, state, txstate)
		}
	}
	// _sometimes_ the temp table exists from the previous cycle.
	// So, 'create if not exists' and truncate.
	_, err = tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS txid_fix_batch (round bigint NOT NULL, intra smallint NOT NULL, txid bytea NOT NULL, PRIMARY KEY ( round, intra ))`)
	if err != nil {
		log.Printf("%s, create temp err: %v", txidMigrationErrMsg, err)
		return err
	}
	_, err = tx.Exec(`TRUNCATE TABLE txid_fix_batch`)
	if err != nil {
		log.Printf("%s, truncate temp err: %v", txidMigrationErrMsg, err)
		return err
	}
	batchadd, err := tx.Prepare(`COPY txid_fix_batch (round, intra, txid) FROM STDIN`)
	if err != nil {
		log.Printf("%s, temp prepare err: %v", txidMigrationErrMsg, err)
		return err
	}
	defer batchadd.Close()
	for _, tr := range outrows {
		_, err = batchadd.Exec(tr.round, tr.intra, tr.txid)
		if err != nil {
			log.Printf("%s, temp row err: %v", txidMigrationErrMsg, err)
			return err
		}
	}
	_, err = batchadd.Exec()
	if err != nil {
		log.Printf("%s, temp empty row err: %v", txidMigrationErrMsg, err)
		return err
	}
	err = batchadd.Close()
	if err != nil {
		log.Printf("%s, temp add close err: %v", txidMigrationErrMsg, err)
		return err
	}

	_, err = tx.Exec(`UPDATE txn SET txid = x.txid FROM txid_fix_batch x WHERE txn.round = x.round AND txn.intra = x.intra`)
	if err != nil {
		log.Printf("%s, update err: %v", txidMigrationErrMsg, err)
		return err
	}
	txstate.NextRound = int64(maxRound + 1)
	migrationStateJson = json.Encode(txstate)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJson)
	if err != nil {
		log.Printf("%s, set metastate err: %v", txidMigrationErrMsg, err)
		return err
	}
	err = tx.Commit()
	if err != nil {
		log.Printf("%s, batch commit err: %v", txidMigrationErrMsg, err)
		return err
	}
	mtxid.state = &txstate
	_, err = db.db.Exec(`DROP TABLE IF EXISTS txid_fix_batch`)
	if err != nil {
		log.Printf("warning txid migration, drop temp err: %v", err)
		// we don't actually care; psql should garbage collect the temp table eventually
	}
	now := time.Now()
	dt := now.Sub(mtxid.lastlog)
	if dt > 5*time.Second {
		mtxid.lastlog = now
		log.Printf("txid fixup migration through %d", maxRound)
	}
	return nil
}

func (mtxid *txidFiuxpMigrationContext) readHeaders(minRound, maxRound uint64) (map[uint64]types.Block, error) {
	db := mtxid.db
	rows, err := db.db.Query(`SELECT round, header FROM block_header WHERE round >= $1 AND round <= $2`, minRound, maxRound)
	if err != nil {
		log.Printf("%s, header err: %v", txidMigrationErrMsg, err)
		return nil, err
	}
	defer rows.Close()
	headers := make(map[uint64]types.Block)
	for rows.Next() {
		var round int64
		var headerjson []byte
		err = rows.Scan(&round, &headerjson)
		if err != nil {
			log.Printf("%s, header row err: %v", txidMigrationErrMsg, err)
			return nil, err
		}
		var tblock types.Block
		json.Decode(headerjson, &tblock)
		headers[uint64(round)] = tblock
	}
	return headers, nil
}
