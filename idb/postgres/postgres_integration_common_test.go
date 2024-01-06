package postgres

import (
	"context"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/indexer/v3/types"
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/v3/idb"
	pgtest "github.com/algorand/indexer/v3/idb/postgres/internal/testing"
	"github.com/algorand/indexer/v3/util/test"
)

func setupIdbWithConnectionString(t testing.TB, connStr string, genesis sdk.Genesis, maxConns *int32, tuning *TuningParams, logger *log.Logger) *IndexerDb {
	opts := idb.IndexerDbOptions{}
	if maxConns != nil {
		opts.MaxConns = *maxConns
	}
	idb, _, err :=  openPostgres(connStr, opts, tuning, logger) // OpenPostgres(connStr, opts, logger)
	require.NoError(t, err)

	err = idb.LoadGenesis(genesis)
	require.NoError(t, err)

	return idb
}

func setupIdb(t *testing.T, genesis sdk.Genesis) (*IndexerDb, func()) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	return setupIdbImpl(t, connStr, genesis, shutdownFunc, nil, nil, nil)
}

func setupIdbWithPgVersion(t testing.TB, genesis sdk.Genesis, pgImage string, maxConns *int32, tuning *TuningParams, pgLogger *log.Logger) (*IndexerDb, func()) {
	_, connStr, shutdownFunc := pgtest.SetupGnomockPgWithVersion(t, pgImage)
	return setupIdbImpl(t, connStr, genesis, shutdownFunc, maxConns, tuning, pgLogger)
}

func setupIdbImpl(t testing.TB, connStr string, genesis sdk.Genesis, shutdownFunc func(), maxConns *int32, tuning *TuningParams, logger *log.Logger) (*IndexerDb, func()) {
	db := setupIdbWithConnectionString(t, connStr, genesis, maxConns, tuning, logger)
	newShutdownFunc := func() {
		db.Close()
		shutdownFunc()
	}
	vb := types.ValidatedBlock{
		Block: test.MakeGenesisBlock(),
		Delta: sdk.LedgerStateDelta{},
	}
	err := db.AddBlock(&vb)
	require.NoError(t, err)

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
