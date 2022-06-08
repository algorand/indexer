package postgres

import (
	"context"
	"testing"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/indexer/processor"
	"github.com/algorand/indexer/processor/blockprocessor"
	"github.com/algorand/indexer/util/test"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
)

func setupIdbWithConnectionString(t *testing.T, connStr string, genesis bookkeeping.Genesis, genesisBlock bookkeeping.Block) *IndexerDb {
	idb, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = idb.LoadGenesis(genesis)
	require.NoError(t, err)

	vb := ledgercore.MakeValidatedBlock(genesisBlock, ledgercore.StateDelta{})
	err = idb.AddBlock(&vb)
	require.NoError(t, err)

	return idb
}

func setupIdb(t *testing.T, genesis bookkeeping.Genesis, genesisBlock bookkeeping.Block) (*IndexerDb /*db*/, func() /*shutdownFunc*/, processor.Processor, *ledger.Ledger) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)

	db := setupIdbWithConnectionString(t, connStr, genesis, genesisBlock)
	newShutdownFunc := func() {
		db.Close()
		shutdownFunc()
	}

	l := test.MakeTestLedger("ledger")
	proc, err := blockprocessor.MakeProcessorWithLedger(l, db.AddBlock)
	require.NoError(t, err, "failed to open ledger")

	return db, newShutdownFunc, proc, l
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
