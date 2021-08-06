package postgres

import (
	"database/sql"
	"flag"
	"fmt"
	"testing"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/postgres"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/util/test"
)

var testpg = flag.String("test-pg", "", "postgres connection string")

// setupPostgres starts a gnomock postgres DB then returns the connection string and a shutdown function.
func setupPostgres(t *testing.T) (*sql.DB, string, func()) {
	if testpg != nil && *testpg != "" {
		// TODO: Drop schema?

		// use non-docker Postgresql
		shutdownFunc := func() {
			// nothing to do, psql db setup/teardown is external
		}
		connStr := *testpg
		db, err := sql.Open("postgres", connStr)
		require.NoError(t, err, "Error opening pg connection")

		return db, connStr, shutdownFunc
	}

	p := postgres.Preset(
		postgres.WithVersion("12.5"),
		postgres.WithUser("gnomock", "gnomick"),
		postgres.WithDatabase("mydb"),
	)
	container, err := gnomock.Start(p)
	require.NoError(t, err, "Error starting gnomock")

	shutdownFunc := func() {
		err = gnomock.Stop(container)
		require.NoError(t, err, "Error stoping gnomock")
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s  dbname=%s sslmode=disable",
		container.Host, container.DefaultPort(),
		"gnomock", "gnomick", "mydb",
	)

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err, "Error opening pg connection")

	return db, connStr, shutdownFunc
}

func setupIdb(t *testing.T, genesis bookkeeping.Genesis) (*IndexerDb /*db*/, func() /*shutdownFunc*/) {
	_, connStr, shutdownFunc := setupPostgres(t)

	idb, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = idb.LoadGenesis(genesis)
	require.NoError(t, err)

	return idb, shutdownFunc
}

func importTxns(t *testing.T, db *IndexerDb, round uint64, txns ...*transactions.SignedTxnWithAD) {
	block, err := test.MakeBlockForTxns(round, txns...)
	require.NoError(t, err)

	_, err = importer.NewDBImporter(db).ImportDecodedBlock(&block)
	require.NoError(t, err)
}

func accountTxns(t *testing.T, db *IndexerDb, round uint64, txns ...*idb.TxnRow) {
	cache, err := db.GetDefaultFrozen()
	require.NoError(t, err)

	state := accounting.New(cache)
	err = state.InitRoundParts(round, test.FeeAddr, test.RewardAddr, 0)
	require.NoError(t, err)

	for _, txn := range txns {
		err := state.AddTransaction(txn)
		require.NoError(t, err)
	}

	err = db.CommitRoundAccounting(state.RoundUpdates, round, &bookkeeping.BlockHeader{})
	require.NoError(t, err)
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
