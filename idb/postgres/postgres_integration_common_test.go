package postgres

import (
	"context"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/indexer/idb"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
	"github.com/algorand/indexer/util/test"
)

func setupIdbWithConnectionString(t *testing.T, connStr string, genesis sdk.Genesis) *IndexerDb {
	idb, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = idb.LoadGenesis(genesis)
	require.NoError(t, err)

	return idb
}

func setupIdb(t *testing.T, genesis sdk.Genesis) (*IndexerDb, func()) {

	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)

	db := setupIdbWithConnectionString(t, connStr, genesis)
	newShutdownFunc := func() {
		db.Close()
		shutdownFunc()
	}
	// todo: update when AddBlock interface gets updated
	vb := ledgercore.MakeValidatedBlock(test.MakeGenesisBlock(), ledgercore.StateDelta{})
	db.AddBlock(&vb)

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
