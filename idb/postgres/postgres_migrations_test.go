package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/data/basics"

	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
	"github.com/algorand/indexer/idb/postgres/internal/types"
)

func TestConvertAccountDataIncrementsMigrationNumber(t *testing.T) {
	pdb, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	db := IndexerDb{db: pdb}
	defer db.Close()

	migrationState := types.MigrationState{
		NextMigration: 5,
	}
	err := db.setMigrationState(nil, &migrationState)
	require.NoError(t, err)

	err = convertAccountData(&db, &migrationState, nil)
	require.NoError(t, err)

	migrationState, err = db.getMigrationState(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, types.MigrationState{NextMigration: 6}, migrationState)
}

func TestCreateAppBoxTable(t *testing.T) {
	pdb, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	db := IndexerDb{db: pdb}
	defer db.Close()

	migrationState := types.MigrationState{
		NextMigration: 19,
	}
	err := db.setMigrationState(nil, &migrationState)
	require.NoError(t, err)

	err = createAppBoxTable(&db, &migrationState, nil)
	require.NoError(t, err)

	migrationState, err = db.getMigrationState(context.Background(), nil)
	require.NoError(t, err)

	appBoxSQL := `SELECT app, name, value FROM app_box WHERE app = $1 AND name = $2`
	appIdx := basics.AppIndex(42)
	boxName := "I do not exist"
	var app basics.AppIndex
	var name, value []byte
	row := db.db.QueryRow(context.Background(), appBoxSQL, appIdx, []byte(boxName))
	err = row.Scan(&app, &name, &value)
	require.ErrorContains(t, err, "no rows in result set")

	assert.Equal(t, types.MigrationState{NextMigration: 20}, migrationState)
}
