// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/migration"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
)

const firstAvailableMigration = 12

func init() {
	migrations = []migrationStruct{
		{FixFreezeLookupMigration, false, "Fix search by asset freeze address."},
		{ClearAccountDataMigration, false, "clear account data for accounts that have been closed"},
		{MakeDeletedNotNullMigration, false, "make all \"deleted\" columns NOT NULL"},
	}
}

// MigrationState is metadata used by the postgres migrations.
type MigrationState struct {
	NextMigration int `json:"next"`

	// NextRound used for m0,m9 to checkpoint progress.
	NextRound int64 `json:"round,omitempty"`

	// NextAssetID used for m3 to checkpoint progress.
	NextAssetID int64 `json:"assetid,omitempty"`

	// The following two are used for m7 to save progress.
	PointerRound *int64 `json:"pointerRound,omitempty"`
	PointerIntra *int64 `json:"pointerIntra,omitempty"`

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

// migrationStateBlocked returns true if a migration is required for running in read only mode.
func migrationStateBlocked(state MigrationState) bool {
	index := 0
	if state.NextMigration >= firstAvailableMigration {
		index = state.NextMigration - firstAvailableMigration
	}

	for index < len(migrations) {
		if migrations[index].blocking {
			return true
		}
		index++
	}
	return false
}

// needsMigration returns true if there is an incomplete migration.
func needsMigration(state MigrationState) bool {
	return state.NextMigration < firstAvailableMigration+len(migrations)
}

// upsertMigrationStateTx updates the migration state, and optionally increments the next counter with an existing
// transaction.
func upsertMigrationStateTx(tx *sql.Tx, state *MigrationState, incrementNextMigration bool) (err error) {
	if incrementNextMigration {
		state.NextMigration++
	}
	migrationStateJSON := encoding.EncodeJSON(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)

	return err
}

// upsertMigrationState updates the migration state, and optionally increments the next counter.
func upsertMigrationState(db *IndexerDb, state *MigrationState, incrementNextMigration bool) (err error) {
	if incrementNextMigration {
		state.NextMigration++
	}
	migrationStateJSON := encoding.EncodeJSON(state)
	_, err = db.db.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)

	return err
}

func (db *IndexerDb) runAvailableMigrations(migrationStateJSON string) (err error) {
	var state MigrationState
	if len(migrationStateJSON) > 0 {
		err = encoding.DecodeJSON([]byte(migrationStateJSON), &state)
		if err != nil {
			return fmt.Errorf("(%s) bad metastate migration json, %v", migrationStateJSON, err)
		}
	}

	if state.NextMigration < firstAvailableMigration {
		return fmt.Errorf(
			"The database is outdated: next migration is %d but the first available "+
				"migration is %d. Please create a new database.",
			state.NextMigration, firstAvailableMigration)
	}

	// Make migration tasks
	index := state.NextMigration - firstAvailableMigration
	tasks := make([]migration.Task, 0)
	for index < len(migrations) {
		tasks = append(tasks, migration.Task{
			Handler:       wrapPostgresHandler(migrations[index].migrate, db, &state),
			MigrationID:   index + firstAvailableMigration,
			Description:   migrations[index].description,
			DBUnavailable: migrations[index].blocking,
		})
		index++
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
		NextMigration: firstAvailableMigration + len(migrations),
	}
	migrationStateJSON := encoding.EncodeJSON(state)
	return db.setMetastate(nil, migrationMetastateKey, string(migrationStateJSON))
}

func (db *IndexerDb) getMigrationState() (*MigrationState, error) {
	migrationStateJSON, err := db.getMetastate(nil, migrationMetastateKey)
	if err == sql.ErrNoRows {
		// no previous state, ok
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("getMigrationState() get state err: %w", err)
	}
	var txstate MigrationState
	err = encoding.DecodeJSON([]byte(migrationStateJSON), &txstate)
	if err != nil {
		return nil, fmt.Errorf("getMigrationState() decode state err: %w", err)
	}
	return &txstate, nil
}

// sqlMigration executes a sql statements as the entire migration.
func sqlMigration(db *IndexerDb, state *MigrationState, sqlLines []string) error {
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	nextState := *state
	nextState.NextMigration++

	f := func(ctx context.Context, tx *sql.Tx) error {
		defer tx.Rollback()

		for _, cmd := range sqlLines {
			_, err := tx.Exec(cmd)
			if err != nil {
				return fmt.Errorf(
					"migration %d exec cmd: \"%s\" err: %w", state.NextMigration, cmd, err)
			}
		}
		migrationStateJSON := encoding.EncodeJSON(nextState)
		_, err := tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
		if err != nil {
			return fmt.Errorf("migration %d exec metastate err: %w", state.NextMigration, err)
		}
		return tx.Commit()
	}
	err := db.txWithRetry(context.Background(), serializable, f)
	if err != nil {
		return fmt.Errorf("migration %d commit err: %w", state.NextMigration, err)
	}

	*state = nextState
	return nil
}

// Reusable update batch function. Provide a query and an array of argument arrays to pass  to that query.
func updateBatch(db *IndexerDb, updateQuery string, data [][]interface{}) error {
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first

	update, err := tx.Prepare(updateQuery)
	if err != nil {
		return fmt.Errorf("error preparing update query: %v", err)
	}
	defer update.Close()

	for _, txpr := range data {
		_, err = update.Exec(txpr...)
		if err != nil {
			return fmt.Errorf("problem updating row (%v): %v", txpr, err)
		}
	}

	return tx.Commit()
}

// FixFreezeLookupMigration is a migration to add txn_participation entries for freeze address in freeze transactions.
func FixFreezeLookupMigration(db *IndexerDb, state *MigrationState) error {
	// Technically with this query no transactions are needed, and the accounting state doesn't need to be locked.
	updateQuery := "INSERT INTO txn_participation (addr, round, intra) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING"
	query := fmt.Sprintf("select decode(txn.txn->'txn'->>'fadd','base64'),round,intra from txn where typeenum = %d AND txn.txn->'txn'->'snd' != txn.txn->'txn'->'fadd'", idb.TypeEnumAssetFreeze)
	rows, err := db.db.Query(query)
	if err != nil {
		return fmt.Errorf("unable to query transactions: %v", err)
	}
	defer rows.Close()

	txprows := make([][]interface{}, 0)

	// Loop through all transactions and compute account data.
	db.log.Print("loop through all freeze transactions")
	for rows.Next() {
		var addr []byte
		var round, intra uint64
		err = rows.Scan(&addr, &round, &intra)
		if err != nil {
			return fmt.Errorf("error scanning row: %v", err)
		}

		txprows = append(txprows, []interface{}{addr, round, intra})

		if len(txprows) > 5000 {
			err = updateBatch(db, updateQuery, txprows)
			if err != nil {
				return fmt.Errorf("updating batch: %v", err)
			}
			txprows = txprows[:0]
		}
	}

	if rows.Err() != nil {
		return fmt.Errorf("error while processing freeze transactions: %v", rows.Err())
	}

	// Commit any leftovers
	if len(txprows) > 0 {
		err = updateBatch(db, updateQuery, txprows)
		if err != nil {
			return fmt.Errorf("updating batch: %v", err)
		}
	}

	// Update migration state
	return upsertMigrationState(db, state, true)
}

type account struct {
	address  sdk_types.Address
	closedAt uint64 // the round when the account was last closed
}

func getAccounts(db *sql.DB) ([]account, error) {
	query := "SELECT addr, closed_at FROM account WHERE closed_at IS NOT NULL AND deleted = false " +
		"AND account_data IS NOT NULL"
	rows, err := db.Query(query)
	if err != nil {
		return []account{}, err
	}
	defer rows.Close()

	var res []account

	for rows.Next() {
		var addrBytes []byte
		var closedAt sql.NullInt64

		err = rows.Scan(&addrBytes, &closedAt)
		if err != nil {
			return []account{}, err
		}

		var addr sdk_types.Address
		copy(addr[:], addrBytes)
		res = append(res, account{addr, uint64(closedAt.Int64)})
	}
	if err := rows.Err(); err != nil {
		return []account{}, err
	}

	return res, nil
}

func fixAuthAddr(db *IndexerDb, account account) error {
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	f := func(ctx context.Context, tx *sql.Tx) error {
		defer tx.Rollback()

		rowsCh := make(chan idb.TxnRow)

		// This will not work properly if the account was closed and reopened in the same round
		// but that's unlikely to happen.
		trueValue := true
		tf := idb.TransactionFilter{
			Address:     account.address[:],
			AddressRole: idb.AddressRoleSender,
			RekeyTo:     &trueValue,
			MinRound:    account.closedAt,
			Limit:       1,
		}
		go func() {
			db.yieldTxns(ctx, tx, tf, rowsCh)
			close(rowsCh)
		}()

		found := false
		for txnRow := range rowsCh {
			if txnRow.Error != nil {
				return txnRow.Error
			}
			found = true
		}

		if found {
			return nil
		}

		// No results. Delete the key.
		db.log.Printf("clearing auth addr for account %s", account.address.String())

		query := "UPDATE account SET account_data = account_data - 'spend' WHERE addr = $1"
		_, err := tx.Exec(query, account.address[:])
		if err != nil {
			return err
		}

		return tx.Commit()
	}

	return db.txWithRetry(context.Background(), serializable, f)
}

func fixKeyreg(db *IndexerDb, account account) error {
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	f := func(ctx context.Context, tx *sql.Tx) error {
		defer tx.Rollback()

		rowsCh := make(chan idb.TxnRow)

		tf := idb.TransactionFilter{
			Address:     account.address[:],
			AddressRole: idb.AddressRoleSender,
			MinRound:    account.closedAt,
			TypeEnum:    idb.TypeEnumKeyreg,
			Limit:       1,
		}
		go func() {
			db.yieldTxns(ctx, tx, tf, rowsCh)
			close(rowsCh)
		}()

		found := false
		for txnRow := range rowsCh {
			if txnRow.Error != nil {
				return txnRow.Error
			}
			found = true
		}

		if found {
			return nil
		}

		// No results. Delete keyreg fields.
		db.log.Printf("clearing keyreg fields for account %s", account.address.String())

		query := "UPDATE account SET account_data = account_data - 'vote' - 'sel' - 'onl' - " +
			"'voteFst' - 'voteLst' - 'voteKD' WHERE addr = $1"
		_, err := tx.Exec(query, account.address[:])
		if err != nil {
			return err
		}

		return tx.Commit()
	}

	return db.txWithRetry(context.Background(), serializable, f)
}

// ClearAccountDataMigration clears account data for accounts that have been closed.
func ClearAccountDataMigration(db *IndexerDb, state *MigrationState) error {
	// Clear account_data column for deleted accounts.
	query := "UPDATE account SET account_data = NULL WHERE deleted = true;"
	if _, err := db.db.Exec(query); err != nil {
		return fmt.Errorf("error clearing deleted accounts: %v", err)
	}

	// Clear account data for some reopened accounts.
	accounts, err := getAccounts(db.db)
	if err != nil {
		return fmt.Errorf("error getting accounts: %v", err)
	}
	db.log.Printf("checking %d accounts that are reopened and have account data", len(accounts))

	for _, account := range accounts {
		if err := fixAuthAddr(db, account); err != nil {
			return fmt.Errorf("error clearing auth addr: %v", err)
		}
		if err := fixKeyreg(db, account); err != nil {
			return fmt.Errorf("error clearing keyreg fields: %v", err)
		}
	}

	state.NextMigration++
	migrationStateJSON := encoding.EncodeJSON(state)
	_, err = db.db.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		return fmt.Errorf("metastate upsert error: %v", err)
	}

	return nil
}

// MakeDeletedNotNullMigration makes "deleted" columns NOT NULL in tables
// account, account_asset, asset, app, account_app.
func MakeDeletedNotNullMigration(db *IndexerDb, state *MigrationState) error {
	queries := []string{
		"UPDATE account SET deleted = false WHERE deleted is NULL",
		"ALTER TABLE account ALTER COLUMN deleted SET NOT NULL",
		"UPDATE account_asset SET deleted = false WHERE deleted is NULL",
		"ALTER TABLE account_asset ALTER COLUMN deleted SET NOT NULL",
		"UPDATE asset SET deleted = false WHERE deleted is NULL",
		"ALTER TABLE asset ALTER COLUMN deleted SET NOT NULL",
		"UPDATE app SET deleted = false WHERE deleted is NULL",
		"ALTER TABLE app ALTER COLUMN deleted SET NOT NULL",
		"UPDATE account_app SET deleted = false WHERE deleted is NULL",
		"ALTER TABLE account_app ALTER COLUMN deleted SET NOT NULL",
	}
	return sqlMigration(db, state, queries)
}
