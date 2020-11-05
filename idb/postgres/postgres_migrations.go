// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/migration"
	"github.com/algorand/indexer/types"
)

func init() {
	migrations = []migrationStruct{
		{m0fixupTxid, false, "Recompute the txid with corrected algorithm."},
		{m1fixupBlockTime, true, "Adjust block time to UTC timezone."},
		{m2apps, true, "Update DB Schema for Algorand application support."},
		{m3acfgFix, false, "Recompute asset configurations with corrected merge function."},
		{m4cumulativeRewardsDBUpdate, true, "Update DB Schema for cumulative account reward support."},
		{m5accountCumulativeRewardsUpdate, true, "Compute cumulative account rewards for all accounts."},
	}
}

// MigrationState is metadata used by the postgres migrations.
type MigrationState struct {
	NextMigration int `json:"next"`

	// NextRound used for m0 to checkpoint progress.
	NextRound int64 `json:"round,omitempty"`

	// NextAssetID used for m3 to checkpoint progress.
	NextAssetID int64 `json:"assetid,omitempty"`

	// NextAccount used for m5 to checkpoint progress.
	NextAccount []byte `json:"nextaccount,omitempty"`

	// Note: a generic "data" field here could be a good way to deal with this growing over time.
	//       It would require a mechanism to clear the data field between migrations to avoid using migration data
	//       from the previous migration.
}

// A migration function should take care of writing back to metastate migration row
type postgresMigrationFunc func(*IndexerDb, *MigrationState) error

type migrationStruct struct {
	migrate postgresMigrationFunc

	blocking bool

	// Description of the migration
	description string
}

var migrations []migrationStruct

func wrapPostgresHandler(handler postgresMigrationFunc, db *IndexerDb, state *MigrationState) migration.Handler {
	return func() error {
		return handler(db, state)
	}
}

func (db *IndexerDb) runAvailableMigrations(migrationStateJSON string) (err error) {
	var state MigrationState
	if len(migrationStateJSON) > 0 {
		err = json.Decode([]byte(migrationStateJSON), &state)
		if err != nil {
			return fmt.Errorf("bad metastate migration json, %v", err)
		}
	}

	// Make migration tasks
	nextMigration := state.NextMigration
	tasks := make([]migration.Task, 0)
	for nextMigration < len(migrations) {
		tasks = append(tasks, migration.Task{
			Handler:       wrapPostgresHandler(migrations[nextMigration].migrate, db, &state),
			MigrationID:   nextMigration,
			Description:   migrations[nextMigration].description,
			DBUnavailable: migrations[nextMigration].blocking,
		})
		nextMigration++
	}

	if len(tasks) > 0 {
		// Add a task to mark migrations as done instead of using a channel.
		tasks = append(tasks, migration.Task{
			MigrationID: 9999999,
			Handler: func() error {
				return db.markMigrationsAsDone()
			},
			Description: "Mark migrations done",
		})
	}

	db.migration, err = migration.MakeMigration(tasks, db.log)
	if err != nil {
		return err
	}

	go db.migration.RunMigrations()

	return nil
}

// after setting up a new database, mark state as if all migrations had been done
func (db *IndexerDb) markMigrationsAsDone() (err error) {
	state := MigrationState{
		NextMigration: len(migrations),
	}
	migrationStateJSON := string(json.Encode(state))
	return db.SetMetastate(migrationMetastateKey, migrationStateJSON)
}

func m0fixupTxid(db *IndexerDb, state *MigrationState) error {
	mtxid := &txidFiuxpMigrationContext{db: db, state: state}
	return mtxid.asyncTxidFixup()
}

func m1fixupBlockTime(db *IndexerDb, state *MigrationState) error {
	sqlLines := []string{
		`UPDATE block_header SET realtime = to_timestamp(coalesce(header ->> 'ts', '0')::bigint) AT TIME ZONE 'UTC'`,
	}
	return sqlMigration(db, state, sqlLines)
}

func m2apps(db *IndexerDb, state *MigrationState) error {
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

func m3acfgFix(db *IndexerDb, state *MigrationState) (err error) {
	db.log.Printf("asset config fix migration starting")
	rows, err := db.db.Query(`SELECT index FROM asset WHERE index >= $1 ORDER BY 1`, state.NextAssetID)
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix err getting assetids")
		return err
	}
	assetIds := make([]int64, 0, 1000)
	for rows.Next() {
		var aid int64
		err = rows.Scan(&aid)
		if err != nil {
			db.log.WithError(err).Errorf("acfg fix err getting assetid row")
			rows.Close()
			return
		}
		assetIds = append(assetIds, aid)
	}
	rows.Close()
	for {
		nexti, err := m3acfgFixAsyncInner(db, state, assetIds)
		if err != nil {
			db.log.WithError(err).Errorf("acfg fix chunk")
			return err
		}
		if nexti < 0 {
			break
		}
		assetIds = assetIds[nexti:]
	}
	db.log.Printf("acfg fix migration finished")
	return nil
}

// do a transactional batch of asset fixes
// updates asset rows and metastate
func m3acfgFixAsyncInner(db *IndexerDb, state *MigrationState, assetIds []int64) (next int, err error) {
	lastlog := time.Now()
	tx, err := db.db.Begin()
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix tx begin")
		return -1, err
	}
	defer tx.Rollback() // ignored if .Commit() first
	setacfg, err := tx.Prepare(`INSERT INTO asset (index, creator_addr, params) VALUES ($1, $2, $3) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params`)
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix prepare set asset")
		return
	}
	defer setacfg.Close()
	for i, aid := range assetIds {
		now := time.Now()
		if now.Sub(lastlog) > (5 * time.Second) {
			db.log.Printf("acfg fix next=%d", aid)
			state.NextAssetID = aid
			migrationStateJSON := json.Encode(state)
			_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
			if err != nil {
				db.log.WithError(err).Errorf("acfg fix migration %d meta err", state.NextMigration)
				return -1, err
			}
			err = tx.Commit()
			if err != nil {
				db.log.WithError(err).Errorf("acfg fix migration %d commit err", state.NextMigration)
				return -1, err
			}
			return i, nil
		}
		txrows := db.txTransactions(tx, idb.TransactionFilter{TypeEnum: 3, AssetID: uint64(aid)})
		prevRound := uint64(0)
		first := true
		var params types.AssetParams
		var creator types.Address
		for txrow := range txrows {
			if txrow.Round < prevRound {
				db.log.Printf("acfg rows out of order %d < %d", txrow.Round, prevRound)
				return
			}
			var stxn types.SignedTxnInBlock
			err = msgpack.Decode(txrow.TxnBytes, &stxn)
			if err != nil {
				db.log.WithError(err).Errorf("acfg fix bad txn bytes %d:%d", txrow.Round, txrow.Intra)
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
			db.log.WithError(err).Errorf("acfg fix asset update")
			return -1, err
		}
	}
	state.NextAssetID = 0
	state.NextMigration++
	migrationStateJSON := json.Encode(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix migration %d meta err", state.NextMigration)
		return -1, err
	}
	err = tx.Commit()
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix migration %d commit err", state.NextMigration)
		return -1, err
	}
	return -1, nil
}

// m4cumulativeRewardsDBUpdate adds the new rewardstotal column to the account table.
func m4cumulativeRewardsDBUpdate(db *IndexerDb, state *MigrationState) error {
	sqlLines := []string{
		`ALTER TABLE account ADD COLUMN rewardstotal bigint NOT NULL DEFAULT 0`,
	}
	return sqlMigration(db, state, sqlLines)
}

const cumulativeRewardsUpdateErr = "cumulative rewards migration error"

func b32ToIndex(c byte) int {
	i := int(c) - 'A'
	if i < 0 {
		//b32 skips 0 and 1.
		return 26 + int(c) - '2'
	}
	return i
}

func addrToPercent(addr string) float64 {
	if len(addr) < 3 {
		return 0.0
	}

	val := b32ToIndex(addr[0])
	val = (val * 32) + b32ToIndex(addr[1])
	val = (val * 32) + b32ToIndex(addr[2])

	return float64(val) / (32 * 32 * 32) * 100
}

// m5accountCumulativeRewardsUpdate computes the cumulative rewards for each account one at a time. This is a BLOCKING
// migration because we don't want to handle the case where accounts are actively transacting while we fixup their
// table.
func m5accountCumulativeRewardsUpdate(db *IndexerDb, state *MigrationState) error {
	db.log.Println("account cumulative rewards migration starting")

	options := idb.AccountQueryOptions{}
	if len(state.NextAccount) != 0 {
		var address sdk_types.Address
		copy(address[:], state.NextAccount)
		db.log.Println("after " + address.String())
		options.GreaterThanAddress = state.NextAccount[:]
	}

	accountChan := db.GetAccounts(context.Background(), options)

	batchSize := 500
	batchNumber := 1
	// loop through all of the accounts, update them in batches of batchSize.
	accounts := make([]string, 0, batchSize)
	for acct := range accountChan {
		if acct.Error != nil {
			err := fmt.Errorf("%s: problem querying accounts: %v", cumulativeRewardsUpdateErr, acct.Error)
			db.log.Errorln(err.Error())
			return err
		}

		accounts = append(accounts, acct.Account.Address)

		if len(accounts) == batchSize {
			db.log.Printf("Cumulative rewards migration processing %.2f%% complete. Batch %d up through account %s",
				addrToPercent(accounts[0]),
				batchNumber,
				accounts[len(accounts)-1])
			m5accountCumulativeRewardsUpdateAccounts(db, state, accounts, false)
			accounts = accounts[:0]
			batchNumber++
		}
	}

	// Get the remainder
	if len(accounts) > 0 {
		db.log.Println("Processing final batch of accounts.")
		m5accountCumulativeRewardsUpdateAccounts(db, state, accounts, true)
		accounts = accounts[:0]
	}

	return nil
}

// m5accountCumulativeRewardsUpdateAccounts loops through the provided accounts and generates a bunch of updates in a
// single transactional commit.
func m5accountCumulativeRewardsUpdateAccounts(db *IndexerDb, state *MigrationState, accounts []string, finalBatch bool) error {
	tx, err := db.db.Begin()
	if err != nil {
		return fmt.Errorf("%s: tx begin: %v", cumulativeRewardsUpdateErr, err)
	}
	defer tx.Rollback() // ignored if .Commit() first

	setCumulativeReward, err := tx.Prepare(`UPDATE account SET rewardstotal = $1 WHERE addr = $2`)
	if err != nil {
		return fmt.Errorf("%s: set rewards prepare: %v", cumulativeRewardsUpdateErr, err)
	}
	defer setCumulativeReward.Close()

	var finalAddress []byte
	// loop through all of the accounts.
	for _, addressStr := range accounts {
		address, err := sdk_types.DecodeAddress(addressStr)
		if err != nil {
			return fmt.Errorf("%s: failed to decode address: %s", cumulativeRewardsUpdateErr, addressStr)
		}
		finalAddress = address[:]

		// for each account loop through all of the transactions
		txnrows := db.txTransactions(tx, idb.TransactionFilter{Address: address[:]})
		var cumulativeRewards types.MicroAlgos = 0
		for txn := range txnrows {
			// for each transaction add up the rewards for the current account
			var stxn types.SignedTxnWithAD
			err = msgpack.Decode(txn.TxnBytes, &stxn)
			if err != nil {
				return fmt.Errorf("%s: processing account %s: %v", cumulativeRewardsUpdateErr, addressStr, err)
			}

			if stxn.Txn.Sender == address {
				cumulativeRewards += stxn.ApplyData.SenderRewards
			}

			if stxn.Txn.Receiver == address {
				cumulativeRewards += stxn.ApplyData.ReceiverRewards
			}

			if stxn.Txn.CloseRemainderTo  == address {
				cumulativeRewards += stxn.ApplyData.CloseRewards
			}
		}

		// Update the account
		_, err = setCumulativeReward.Exec(cumulativeRewards, finalAddress[:])
		if err != nil {
			return fmt.Errorf("%s: failed to update %s with rewards %d: %v", cumulativeRewardsUpdateErr, addressStr, cumulativeRewards, err)
		}
	}

	// Update checkpoint
	if finalBatch {
		state.NextAccount = nil
	} else {
		state.NextAccount = finalAddress[:]
	}
	migrationStateJSON := json.Encode(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		return fmt.Errorf("%s: failed to update migration checkpoint: %v", cumulativeRewardsUpdateErr, err)
	}

	// Commit transactions
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("%s: failed to commit changes: %v", cumulativeRewardsUpdateErr, err)
	}

	return nil
}

// sqlMigration executes a sql statements as the entire migration.
func sqlMigration(db *IndexerDb, state *MigrationState, sqlLines []string) error {
	thisMigration := state.NextMigration
	tx, err := db.db.Begin()
	if err != nil {
		db.log.WithError(err).Errorf("migration %d tx err", thisMigration)
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first
	for i, cmd := range sqlLines {
		_, err = tx.Exec(cmd)
		if err != nil {
			db.log.WithError(err).Errorf("migration %d sql[%d] err", thisMigration, i)
			return err
		}
	}
	state.NextMigration++
	migrationStateJSON := json.Encode(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("migration %d meta err", thisMigration)
		return err
	}
	err = tx.Commit()
	if err != nil {
		db.log.WithError(err).Errorf("migration %d commit err", thisMigration)
		return err
	}
	db.log.Printf("migration %d done", thisMigration)
	return nil
}

const txidMigrationErrMsg = "ERROR migrating txns for txid, stopped, will retry on next indexer startup"

type migrationContext struct {
	db      *IndexerDb
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
	db.log.Println("txid fixup migration starting")
	prevRound := state.NextRound - 1
	txns := db.YieldTxns(context.Background(), prevRound)
	batch := make([]idb.TxnRow, 15000)
	txInBatch := 0
	roundsInBatch := 0
	prevBatchRound := uint64(math.MaxUint64)
	for txr := range txns {
		if txr.Error != nil {
			db.log.WithError(txr.Error).Errorf("ERROR migrating txns for txid rewrite")
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
	migrationStateJSON := string(json.Encode(state))
	err = db.SetMetastate(migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("%s, error setting final migration state", txidMigrationErrMsg)
		return
	}
	db.log.Println("txid fixup migration finished")
	return nil
}

type txidFixupRow struct {
	round uint64
	intra int
	txid  string // base32 string
}

func (mtxid *txidFiuxpMigrationContext) putTxidFixupBatch(batch []idb.TxnRow) error {
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
			db.log.WithError(err).Errorf("%s, proto", txidMigrationErrMsg)
			return err
		}
		var stxn types.SignedTxnInBlock
		err = msgpack.Decode(txr.TxnBytes, &stxn)
		if err != nil {
			db.log.WithError(err).Errorf("%s, txnb msgpack err", txidMigrationErrMsg)
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
		db.log.WithError(err).Errorf("%s, batch tx err", txidMigrationErrMsg)
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first
	// Check that migration state in db is still what we think it is
	row := tx.QueryRow(`SELECT v FROM metastate WHERE k = $1`, migrationMetastateKey)
	var migrationStateJSON []byte
	err = row.Scan(&migrationStateJSON)
	var txstate MigrationState
	if err == sql.ErrNoRows && state.NextMigration == 0 {
		// no previous state, ok
	} else if err != nil {
		db.log.WithError(err).Errorf("%s, get m state err", txidMigrationErrMsg)
		return err
	} else {
		err = json.Decode([]byte(migrationStateJSON), &txstate)
		if err != nil {
			db.log.WithError(err).Errorf("%s, migration json err", txidMigrationErrMsg)
			return err
		}
		if state.NextMigration != txstate.NextMigration || state.NextRound != txstate.NextRound {
			db.log.Printf("%s, migration state changed when we weren't looking: %v -> %v", txidMigrationErrMsg, state, txstate)
		}
	}
	// _sometimes_ the temp table exists from the previous cycle.
	// So, 'create if not exists' and truncate.
	_, err = tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS txid_fix_batch (round bigint NOT NULL, intra smallint NOT NULL, txid bytea NOT NULL, PRIMARY KEY ( round, intra ))`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, create temp err", txidMigrationErrMsg)
		return err
	}
	_, err = tx.Exec(`TRUNCATE TABLE txid_fix_batch`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, truncate temp err", txidMigrationErrMsg)
		return err
	}
	batchadd, err := tx.Prepare(`COPY txid_fix_batch (round, intra, txid) FROM STDIN`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp prepare err", txidMigrationErrMsg)
		return err
	}
	defer batchadd.Close()
	for _, tr := range outrows {
		_, err = batchadd.Exec(tr.round, tr.intra, tr.txid)
		if err != nil {
			db.log.WithError(err).Errorf("%s, temp row err", txidMigrationErrMsg)
			return err
		}
	}
	_, err = batchadd.Exec()
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp empty row err", txidMigrationErrMsg)
		return err
	}
	err = batchadd.Close()
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp add close err", txidMigrationErrMsg)
		return err
	}

	_, err = tx.Exec(`UPDATE txn SET txid = x.txid FROM txid_fix_batch x WHERE txn.round = x.round AND txn.intra = x.intra`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, update err", txidMigrationErrMsg)
		return err
	}
	txstate.NextRound = int64(maxRound + 1)
	migrationStateJSON = json.Encode(txstate)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("%s, set metastate err", txidMigrationErrMsg)
		return err
	}
	err = tx.Commit()
	if err != nil {
		db.log.WithError(err).Errorf("%s, batch commit err", txidMigrationErrMsg)
		return err
	}
	mtxid.state = &txstate
	_, err = db.db.Exec(`DROP TABLE IF EXISTS txid_fix_batch`)
	if err != nil {
		db.log.WithError(err).Errorf("warning txid migration, drop temp err")
		// we don't actually care; psql should garbage collect the temp table eventually
	}
	now := time.Now()
	dt := now.Sub(mtxid.lastlog)
	if dt > 5*time.Second {
		mtxid.lastlog = now
		db.log.Printf("txid fixup migration through %d", maxRound)
	}
	return nil
}

func (mtxid *txidFiuxpMigrationContext) readHeaders(minRound, maxRound uint64) (map[uint64]types.Block, error) {
	db := mtxid.db
	rows, err := db.db.Query(`SELECT round, header FROM block_header WHERE round >= $1 AND round <= $2`, minRound, maxRound)
	if err != nil {
		db.log.WithError(err).Errorf("%s, header err", txidMigrationErrMsg)
		return nil, err
	}
	defer rows.Close()
	headers := make(map[uint64]types.Block)
	for rows.Next() {
		var round int64
		var headerjson []byte
		err = rows.Scan(&round, &headerjson)
		if err != nil {
			db.log.WithError(err).Errorf("%s, header row err", txidMigrationErrMsg)
			return nil, err
		}
		var tblock types.Block
		json.Decode(headerjson, &tblock)
		headers[uint64(round)] = tblock
	}
	return headers, nil
}
