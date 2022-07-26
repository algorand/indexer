package postgres

import (
	"context"
	"testing"

	"github.com/algorand/indexer/idb/postgres/internal/schema"
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

func TestConvertBigIntType(t *testing.T) {
	pdb, _, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	db := IndexerDb{db: pdb}
	defer db.Close()

	// set up db with old schema
	_, err := pdb.Exec(context.Background(), schema.BigIntSchema)
	require.NoError(t, err)

	// old table schema. insert a record with round 18446744073709551613
	query := "INSERT INTO txn_participation(addr,round,intra) VALUES('\\x013d7d16d7ad4fefb61bd95b765c8ceb'::bytea,18446744073709551613,1)"
	_, err = pdb.Exec(context.Background(), query)
	assert.Contains(t, err.Error(), "ERROR: bigint out of range")

	// run type conversion
	migrationState := types.MigrationState{
		NextMigration: 6,
	}
	err = db.setMigrationState(nil, &migrationState)
	require.NoError(t, err)

	err = convertBigIntType(&db, &migrationState)
	require.NoError(t, err)

	migrationState, err = db.getMigrationState(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, types.MigrationState{NextMigration: 7}, migrationState)

	// after table schema is updated. insert a record with round 18446744073709551613
	_, err = pdb.Exec(context.Background(), query)
	assert.NoError(t, err)

}
