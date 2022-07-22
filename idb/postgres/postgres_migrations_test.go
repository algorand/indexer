package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	err = convertAccountData(&db, &migrationState)
	require.NoError(t, err)

	migrationState, err = db.getMigrationState(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, types.MigrationState{NextMigration: 6}, migrationState)
}
