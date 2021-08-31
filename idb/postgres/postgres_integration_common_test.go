package postgres

import (
	"database/sql"
	"testing"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/test_util"
)

func setupIdb(t *testing.T, genesis bookkeeping.Genesis, genesisBlock bookkeeping.Block) (*IndexerDb /*db*/, func() /*shutdownFunc*/) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)

	idb, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = idb.LoadGenesis(genesis)
	require.NoError(t, err)

	err = idb.AddBlock(&genesisBlock)
	require.NoError(t, err)

	return idb, shutdownFunc
}

// Helper to execute a query returning an integer, for example COUNT(*). Returns -1 on an error.
func queryInt(db *sql.DB, queryString string, args ...interface{}) int {
	row := db.QueryRow(queryString, args...)

	var count int
	err := row.Scan(&count)
	if err != nil {
		return -1
	}
	return count
}
