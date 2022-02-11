// You can build without postgres by `go build --tags nopostgres` but it's on by default
//go:build !nopostgres
// +build !nopostgres

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/migration"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	cad "github.com/algorand/indexer/idb/postgres/internal/migrations/convert_account_data"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	"github.com/algorand/indexer/idb/postgres/internal/types"
)

func init() {
	// To deprecate old migrations change the functions to return a `unsupportedMigrationErrorMsg` error.
	// Make sure you set the blocking flag to true to avoid possible consistency issues during startup.
	migrations = []migrationStruct{
		// function, blocking, description
		{upgradeNotSupported, false, "Recompute the txid with corrected algorithm."},
		{upgradeNotSupported, true, "Adjust block time to UTC timezone."},
		{upgradeNotSupported, true, "Update DB Schema for Algorand application support."},
		{upgradeNotSupported, false, "Recompute asset configurations with corrected merge function."},

		// 2.2.2 hotfix
		{upgradeNotSupported, true, "Add indices to make sure account lookups remain fast when there are a lot of apps or assets."},

		// Migrations for 2.3.1 release
		{upgradeNotSupported, true, "record round at which txn json recording changes, for future migration to fixup prior records"},
		{upgradeNotSupported, true, "Update DB Schema for cumulative account reward support and creation dates."},
		{upgradeNotSupported, false, "Compute cumulative account rewards for all accounts."},

		// Migrations for 2.3.2 release
		{upgradeNotSupported, false, "clear some stale data from closed accounts"},
		{upgradeNotSupported, false, "some txn JSON encodings need app keys base64 encoded"},
		{upgradeNotSupported, false, "The initial m7 implementation would miss special accounts."},
		{upgradeNotSupported, true, "Fix asset holding freeze states."},

		{upgradeNotSupported, false, "Fix search by asset freeze address."},
		{upgradeNotSupported, false, "clear account data for accounts that have been closed"},
		{upgradeNotSupported, false, "make all \"deleted\" columns NOT NULL"},
		{upgradeNotSupported, true, "change import state format"},
		{upgradeNotSupported, true, "notify the user that upgrade is not supported"},
		{dropTxnBytesColumn, true, "drop txnbytes column"},
		{convertAccountData, true, "convert account.account_data column"},
	}
}

// A migration function should take care of writing back to metastate migration row
type postgresMigrationFunc func(*IndexerDb, *types.MigrationState) error

type migrationStruct struct {
	migrate postgresMigrationFunc

	blocking bool

	// Description of the migration
	description string
}

var migrations []migrationStruct

func wrapPostgresHandler(handler postgresMigrationFunc, db *IndexerDb, state *types.MigrationState) migration.Handler {
	return func() error {
		return handler(db, state)
	}
}

// migrationStateBlocked returns true if a migration is required for running in read only mode.
func migrationStateBlocked(state types.MigrationState) bool {
	for i := state.NextMigration; i < len(migrations); i++ {
		if migrations[i].blocking {
			return true
		}
	}
	return false
}

// needsMigration returns true if there is an incomplete migration.
func needsMigration(state types.MigrationState) bool {
	return state.NextMigration < len(migrations)
}

// Returns an error object and a channel that gets closed when blocking migrations
// finish running successfully.
func (db *IndexerDb) runAvailableMigrations() (chan struct{}, error) {
	state, err := db.getMigrationState(context.Background(), nil)
	if err == idb.ErrorNotInitialized {
		state = types.MigrationState{}
	} else if err != nil {
		return nil, fmt.Errorf("runAvailableMigrations() err: %w", err)
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
		return nil, err
	}

	ch := db.migration.RunMigrations()
	return ch, nil
}

// after setting up a new database, mark state as if all migrations had been done
func (db *IndexerDb) markMigrationsAsDone() (err error) {
	state := types.MigrationState{
		NextMigration: len(migrations),
	}
	migrationStateJSON := encoding.EncodeMigrationState(&state)
	return db.setMetastate(nil, schema.MigrationMetastateKey, string(migrationStateJSON))
}

// Returns `idb.ErrorNotInitialized` if uninitialized.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getMigrationState(ctx context.Context, tx pgx.Tx) (types.MigrationState, error) {
	migrationStateJSON, err := db.getMetastate(
		ctx, tx, schema.MigrationMetastateKey)
	if err == idb.ErrorNotInitialized {
		return types.MigrationState{}, idb.ErrorNotInitialized
	} else if err != nil {
		return types.MigrationState{}, fmt.Errorf("getMigrationState() get state err: %w", err)
	}

	state, err := encoding.DecodeMigrationState([]byte(migrationStateJSON))
	if err != nil {
		return types.MigrationState{}, fmt.Errorf("getMigrationState() decode state err: %w", err)
	}

	return state, nil
}

// If `tx` is nil, use a normal query.
func (db *IndexerDb) setMigrationState(tx pgx.Tx, state *types.MigrationState) error {
	err := db.setMetastate(
		tx, schema.MigrationMetastateKey, string(encoding.EncodeMigrationState(state)))
	if err != nil {
		return fmt.Errorf("setMigrationState() err: %w", err)
	}

	return nil
}

// sqlMigration executes a sql statements as the entire migration.
//lint:ignore U1000 this function might be used in a future migration
func sqlMigration(db *IndexerDb, state *types.MigrationState, sqlLines []string) error {
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	nextState := *state
	nextState.NextMigration++

	f := func(tx pgx.Tx) error {
		for _, cmd := range sqlLines {
			_, err := tx.Exec(context.Background(), cmd)
			if err != nil {
				return fmt.Errorf(
					"migration %d exec cmd: \"%s\" err: %w", state.NextMigration, cmd, err)
			}
		}
		err := db.setMetastate(
			tx, schema.MigrationMetastateKey,
			string(encoding.EncodeMigrationState(&nextState)))
		if err != nil {
			return fmt.Errorf("migration %d exec metastate err: %w", state.NextMigration, err)
		}
		return nil
	}
	err := db.txWithRetry(serializable, f)
	if err != nil {
		return fmt.Errorf("migration %d commit err: %w", state.NextMigration, err)
	}

	*state = nextState
	return nil
}

const unsupportedMigrationErrorMsg = "unsupported migration: please downgrade to %s to run this migration"

// disabled creates a simple migration handler for unsupported migrations.
//lint:ignore U1000 this function might be used in the future
func disabled(version string) func(db *IndexerDb, migrationState *types.MigrationState) error {
	return func(_ *IndexerDb, _ *types.MigrationState) error {
		return fmt.Errorf(unsupportedMigrationErrorMsg, version)
	}
}

func upgradeNotSupported(db *IndexerDb, migrationState *types.MigrationState) error {
	return errors.New(
		"upgrading from this version is not supported; create a new database")
}

func dropTxnBytesColumn(db *IndexerDb, migrationState *types.MigrationState) error {
	return sqlMigration(
		db, migrationState, []string{"ALTER TABLE txn DROP COLUMN txnbytes"})
}

func convertAccountData(db *IndexerDb, migrationState *types.MigrationState) error {
	newMigrationState := *migrationState
	newMigrationState.NextMigration++

	f := func(tx pgx.Tx) error {
		err := cad.RunMigration(tx, 10000)
		if err != nil {
			return fmt.Errorf("convertAccountData() err: %w", err)
		}

		err = db.setMigrationState(tx, &newMigrationState)
		if err != nil {
			return fmt.Errorf("convertAccountData() err: %w", err)
		}

		return nil
	}
	err := db.txWithRetry(serializable, f)
	if err != nil {
		return fmt.Errorf("convertAccountData() err: %w", err)
	}

	*migrationState = newMigrationState
	return nil
}
