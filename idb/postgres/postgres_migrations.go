// You can build without postgres by `go build --tags nopostgres` but it's on by default
//go:build !nopostgres
// +build !nopostgres

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/migration"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
)

func init() {
	// To deprecate old migrations change the functions to return a `unsupportedMigrationErrorMsg` error.
	// Make sure you set the blocking flag to true to avoid possible consistency issues during startup.
	migrations = []migrationStruct{
		// function, blocking, description
		{disabled("2.5.0"), false, "Recompute the txid with corrected algorithm."},
		{disabled("2.5.0"), true, "Adjust block time to UTC timezone."},
		{disabled("2.5.0"), true, "Update DB Schema for Algorand application support."},
		{disabled("2.5.0"), false, "Recompute asset configurations with corrected merge function."},

		// 2.2.2 hotfix
		{disabled("2.5.0"), true, "Add indices to make sure account lookups remain fast when there are a lot of apps or assets."},

		// Migrations for 2.3.1 release
		{disabled("2.5.0"), true, "record round at which txn json recording changes, for future migration to fixup prior records"},
		{disabled("2.5.0"), true, "Update DB Schema for cumulative account reward support and creation dates."},
		{disabled("2.5.0"), false, "Compute cumulative account rewards for all accounts."},

		// Migrations for 2.3.2 release
		{disabled("2.5.0"), false, "clear some stale data from closed accounts"},
		{disabled("2.5.0"), false, "some txn JSON encodings need app keys base64 encoded"},
		{disabled("2.5.0"), false, "The initial m7 implementation would miss special accounts."},
		{disabled("2.5.0"), true, "Fix asset holding freeze states."},

		{disabled("2.5.0"), false, "Fix search by asset freeze address."},
		{disabled("2.5.0"), false, "clear account data for accounts that have been closed"},
		{disabled("2.5.0"), false, "make all \"deleted\" columns NOT NULL"},
		{disabled("2.6.1"), true, "change import state format"},
	}
}

// MigrationState is metadata used by the postgres migrations.
type MigrationState struct {
	NextMigration int `json:"next"`

	// The following are deprecated.
	NextRound    int64  `json:"round,omitempty"`
	NextAssetID  int64  `json:"assetid,omitempty"`
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
	for i := state.NextMigration; i < len(migrations); i++ {
		if migrations[i].blocking {
			return true
		}
	}
	return false
}

// needsMigration returns true if there is an incomplete migration.
func needsMigration(state MigrationState) bool {
	return state.NextMigration < len(migrations)
}

// upsertMigrationState updates the migration state, and optionally increments
// the next counter with an existing transaction.
// If `tx` is nil, use a normal query.
//lint:ignore U1000 this function might be used in a future migration
func upsertMigrationState(db *IndexerDb, tx pgx.Tx, state *MigrationState) error {
	migrationStateJSON := encoding.EncodeJSON(state)
	return db.setMetastate(tx, schema.MigrationMetastateKey, string(migrationStateJSON))
}

// Returns an error object and a channel that gets closed when blocking migrations
// finish running successfully.
func (db *IndexerDb) runAvailableMigrations() (chan struct{}, error) {
	state, err := db.getMigrationState()
	if err == idb.ErrorNotInitialized {
		state = MigrationState{}
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
	state := MigrationState{
		NextMigration: len(migrations),
	}
	migrationStateJSON := encoding.EncodeJSON(state)
	return db.setMetastate(nil, schema.MigrationMetastateKey, string(migrationStateJSON))
}

// Returns `idb.ErrorNotInitialized` if uninitialized.
func (db *IndexerDb) getMigrationState() (MigrationState, error) {
	migrationStateJSON, err := db.getMetastate(context.Background(), nil, schema.MigrationMetastateKey)
	if err == idb.ErrorNotInitialized {
		return MigrationState{}, idb.ErrorNotInitialized
	} else if err != nil {
		return MigrationState{}, fmt.Errorf("getMigrationState() get state err: %w", err)
	}

	var state MigrationState
	err = encoding.DecodeJSON([]byte(migrationStateJSON), &state)
	if err != nil {
		return MigrationState{}, fmt.Errorf("getMigrationState() decode state err: %w", err)
	}

	return state, nil
}

// sqlMigration executes a sql statements as the entire migration.
//lint:ignore U1000 this function might be used in a future migration
func sqlMigration(db *IndexerDb, state *MigrationState, sqlLines []string) error {
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
		migrationStateJSON := encoding.EncodeJSON(nextState)
		_, err := tx.Exec(
			context.Background(), setMetastateUpsert, schema.MigrationMetastateKey,
			migrationStateJSON)
		if err != nil {
			return fmt.Errorf("migration %d exec metastate err: %w", state.NextMigration, err)
		}
		return tx.Commit(context.Background())
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
func disabled(version string) func(db *IndexerDb, migrationState *MigrationState) error {
	return func(_ *IndexerDb, _ *MigrationState) error {
		return fmt.Errorf(unsupportedMigrationErrorMsg, version)
	}
}
