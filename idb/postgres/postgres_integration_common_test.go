package postgres

import (
	"context"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/jackc/pgx/v4/pgxpool"
	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/util/test"

	"github.com/algorand/indexer/conduit/plugins/processors/blockprocessor"
	"github.com/algorand/indexer/idb"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
)

func setupIdbWithConnectionString(t *testing.T, connStr string, genesis sdk.Genesis) *IndexerDb {
	idb, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = idb.LoadGenesis(genesis)
	require.NoError(t, err)

	return idb
}

func setupIdb(t *testing.T, genesis sdk.Genesis) (*IndexerDb, func(), func(cert *rpcs.EncodedBlockCert) error, *ledger.Ledger) {

	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)

	db := setupIdbWithConnectionString(t, connStr, genesis)
	newShutdownFunc := func() {
		db.Close()
		shutdownFunc()
	}

	logger, _ := test2.NewNullLogger()
	l, err := test.MakeTestLedger(logger)
	require.NoError(t, err)
	proc, err := blockprocessor.MakeBlockProcessorWithLedger(logger, l, db.AddBlock)
	require.NoError(t, err, "failed to open ledger")

	f := blockprocessor.MakeBlockProcessorHandlerAdapter(&proc, db.AddBlock)

	return db, newShutdownFunc, f, l
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
