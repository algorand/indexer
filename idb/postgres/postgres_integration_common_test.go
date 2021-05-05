package postgres

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/postgres"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb"
)

// setupPostgres starts a gnomock postgres DB then returns the connection string and a shutdown function.
func setupPostgres(t *testing.T) (*sql.DB, string, func()) {
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

func setupIdb(t *testing.T, genesis bookkeeping.Genesis, genesisBlock bookkeeping.Block) (*IndexerDb /*db*/, func() /*shutdownFunc*/) {
	_, connStr, shutdownFunc := setupPostgres(t)

	idb, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = idb.LoadGenesis(genesis)
	require.NoError(t, err)

	err = idb.AddBlock(genesisBlock)
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
