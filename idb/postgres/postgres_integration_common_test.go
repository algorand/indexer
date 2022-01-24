package postgres

import (
	"context"
	"testing"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
)

func setupIdbWithConnectionString(t *testing.T, connStr string, genesis bookkeeping.Genesis, genesisBlock bookkeeping.Block) *IndexerDb {

	postgresConfig, err := pgxpool.ParseConfig(connStr)
	require.NoError(t, err)

	idb, _, err := OpenPostgres(postgresConfig, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = idb.LoadGenesis(genesis)
	require.NoError(t, err)

	err = idb.AddBlock(&genesisBlock)
	require.NoError(t, err)

	return idb
}

func setupIdb(t *testing.T, genesis bookkeeping.Genesis, genesisBlock bookkeeping.Block) (*IndexerDb /*db*/, func() /*shutdownFunc*/) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)

	db := setupIdbWithConnectionString(t, connStr, genesis, genesisBlock)
	newShutdownFunc := func() {
		db.Close()
		shutdownFunc()
	}

	return db, newShutdownFunc
}

// Helper to execute a query returning an integer, for example COUNT(*). Returns -1 on an error.
func queryInt(db *pgxpool.Pool, queryString string, args ...interface{}) int {
	row := db.QueryRow(context.Background(), queryString, args...)

	var count int
	err := row.Scan(&count)
	if err != nil {
		return -1
	}
	return count
}
