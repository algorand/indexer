package util_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/algorand/indexer/exporters/util"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres"
	pgtest "github.com/algorand/indexer/idb/postgres/testing"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

var logger *logrus.Logger

func init() {
	logger, _ = test.NewNullLogger()
}

func TestMakeDataManager(t *testing.T) {
	config := util.PruneConfigurations{
		Interval: "once",
		Rounds:   10,
	}
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	dm, err := util.MakeDataManager(&config, "postgres", connStr, logger)
	assert.NoError(t, err)
	assert.NotNil(t, dm)
}

func TestDataPruning(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	db, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	defer db.Close()

	dm := makeMockedDataManager(nil)
	ch := make(chan uint64)
	ctx := context.Background()
	dm.Delete(ctx, ch)
}
func TestDelete(t *testing.T) {
	logger.SetOutput(os.Stdout)
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	idb.Close()

	// add 20 records to txn table
	err = populateTxnTable(db)
	assert.NoError(t, err)
	assert.Equal(t, 20, rowsInTxnTable(db))

	config := util.PruneConfigurations{
		Interval: "once",
		Rounds:   10,
	}
	// data manager
	dm, err := util.MakeDataManager(&config, "postgres", connStr, logger)
	assert.NoError(t, err)
	ch := make(chan uint64)
	go dm.Delete(context.Background(), ch)
	// ch empty, nothing happens
	assert.Equal(t, 20, rowsInTxnTable(db))
	// send current round
	ch <- 20
	// wait
	time.Sleep(1 * time.Second)
	// 10 rounds removed
	assert.Equal(t, 10, rowsInTxnTable(db))
	// check remaining rounds are correct
	assert.True(t, validateTxnTable(db, 10, 20))
	// todo: test rollback
	// todo: test context closed.
	// todo: test daily
	// todo: test timeout
	// todo: test round < p.Config.Rounds
	// todo: test round == p.Config.Rounds
	close(ch)
}
func makeMockedDataManager(db *pgxpool.Pool) util.DataManager {
	return util.Postgressql{
		Config: &util.PruneConfigurations{
			Interval: "once",
			Rounds:   10,
		},
		DB:     db,
		Logger: nil,
	}
}

func populateTxnTable(db *pgxpool.Pool) error {
	batch := &pgx.Batch{}
	for i := 0; i < 20; i++ {
		query := "INSERT INTO txn(round, intra, typeenum,asset,txn,extra) VALUES ($1,$2,$3,$4,$5,$6)"
		batch.Queue(query, i, i, 1, 0, "{}", "{}")
	}
	results := db.SendBatch(context.Background(), batch)
	defer results.Close()
	for i := 0; i < batch.Len(); i++ {
		_, err := results.Exec()
		if err != nil {
			return fmt.Errorf("populateTxnTable() exec err: %w", err)
		}
	}
	return nil
}

func rowsInTxnTable(db *pgxpool.Pool) int {
	var rows int
	r, err := db.Query(context.Background(), "SELECT count(*) FROM txn")
	if err != nil {
		return 0
	}
	defer r.Close()
	for r.Next() {
		err = r.Scan(&rows)
		if err != nil {
			return 0
		}
	}
	return rows
}

func validateTxnTable(db *pgxpool.Pool, first, last uint64) bool {
	res, err := db.Query(context.Background(), "SELECT round FROM txn")
	if err != nil {
		return false
	}
	defer res.Close()
	var round uint64
	// expected round
	expected := first
	next := first + 1
	for res.Next() {
		err = res.Scan(&round)
		if err != nil || round != expected {
			return false
		}
		expected = next
		next++
	}
	return expected == last
}
