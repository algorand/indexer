// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package idb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"

	"github.com/algorand/indexer/types"
)

const migrationMetastateKey = "migration"

type MigrationState struct {
	NextMigration int `json:"done"`

	// Used for a long migration to checkpoint progress
	NextRound int64 `json:"round,omitempty"`
}

// A migration function should take care of writing back to metastate migration row
type migrationFunc func(*PostgresIndexerDb, *MigrationState) error

var migrations []migrationFunc

var asyncMigration = errors.New("Migration will continue asynchronously")

// TODO: don't actually need accountingStateJson ?
func (db *PostgresIndexerDb) migrate(accountingStateJson, migrationStateJson string) (err error) {
	var state MigrationState
	if len(migrationStateJson) > 0 {
		err = json.Decode([]byte(migrationStateJson), &state)
		if err != nil {
			return fmt.Errorf("bad metastate migration json, %v", err)
		}
	}
	nextMigration := state.NextMigration
	for nextMigration < len(migrations) {
		err = migrations[nextMigration](db, &state)
		if err == asyncMigration {
			// migration will continue asynchronously, it should call db.migrate() when it is done
			return nil
		}
		if err != nil {
			return fmt.Errorf("error in migration %d: %v", nextMigration, err)
		}
		nextMigration++
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
	go asyncTxidFixup(db, state)
	return asyncMigration
}

func init() {
	migrations = []migrationFunc{
		m0fixupTxid,
	}
}

const txidMigrationErrMsg = "ERROR migrating txns for txid, stopped, will retry on next indexer startup"

// read batches of at least 2 blocks or up to 10000 txns,
// write a temporary table, UPDATE from temporary table into txn.
// repeat until all txns consumed.
func asyncTxidFixup(db *PostgresIndexerDb, state *MigrationState) {
	prevRound := state.NextRound - 1
	txns := db.YieldTxns(context.Background(), prevRound)
	batch := make([]TxnRow, 15000)
	txInBatch := 0
	roundsInBatch := 0
	prevBatchRound := uint64(math.MaxUint64)
	var err error
	for txr := range txns {
		if txr.Error != nil {
			log.Printf("ERROR migrating txns for txid rewrite: %v", txr.Error)
			log.Print("txidMigrationErrMsg")
			return
		}
		if txr.Round != prevBatchRound {
			if txInBatch > 10000 {
				err = putTxidFixupBatch(db, state, batch[:txInBatch])
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
			err = putTxidFixupBatch(db, state, batch[:split])
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
		err = putTxidFixupBatch(db, state, batch[:txInBatch])
		if err != nil {
			return
		}
	}
}

type txidFixupRow struct {
	round uint64
	intra int
	txid  string // base32 string
}

func putTxidFixupBatch(db *PostgresIndexerDb, state *MigrationState, batch []TxnRow) error {
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
	headers, err := readHeaders(db, minRound, maxRound)
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
	_, err = tx.Exec(`CREATE TEMP TABLE txid_fix_batch (round bigint NOT NULL, intra smallint NOT NULL, txid bytea NOT NULL, PRIMARY KEY ( round, intra ))`)
	if err != nil {
		log.Printf("%s, create temp err: %v", txidMigrationErrMsg, err)
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

	_, err = tx.Exec(`UPDATE txn t SET t.txid = x.txid FROM txid_fix_batch x WHERE t.round = x.round AND t.intra = x.intra`)
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
	return nil
}

func readHeaders(db *PostgresIndexerDb, minRound, maxRound uint64) (map[uint64]types.Block, error) {
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
