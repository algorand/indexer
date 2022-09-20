package util

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

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

var config = PruneConfigurations{
	Frequency: "once",
	Rounds:    10,
}

func TestMakeDataManager(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	dm, err := MakeDataManager(context.Background(), &config, "postgres", connStr, logger)
	assert.NoError(t, err)
	assert.NotNil(t, dm)
}

func TestDeleteEmptyTxnTable(t *testing.T) {
	logger.SetOutput(os.Stdout)
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	idb.Close()

	// empty txn table
	err = populateTxnTable(db, 0)
	assert.NoError(t, err)
	assert.Equal(t, 0, rowsInTxnTable(db))

	// data manager
	dm, err := MakeDataManager(context.Background(), &config, "postgres", connStr, logger)
	assert.NoError(t, err)
	go dm.Delete()
	// wait
	time.Sleep(1 * time.Second)
	assert.Equal(t, 0, rowsInTxnTable(db))
}

func TestDeleteTxns(t *testing.T) {
	logger.SetOutput(os.Stdout)
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	idb.Close()

	// add 20 records to txn table
	err = populateTxnTable(db, 20)
	assert.NoError(t, err)
	assert.Equal(t, 20, rowsInTxnTable(db))

	// data manager
	dm, err := MakeDataManager(context.Background(), &config, "postgres", connStr, logger)
	assert.NoError(t, err)
	go dm.Delete()
	// wait
	time.Sleep(1 * time.Second)
	// 10 rounds removed
	assert.Equal(t, 10, rowsInTxnTable(db))
	// check remaining rounds are correct
	assert.True(t, validateTxnTable(db, 10, 20))
	//	processor ended
	assert.True(t, dm.Closed())

}

func TestDeleteConfigs(t *testing.T) {
	logger.SetOutput(os.Stdout)
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	idb.Close()

	// add 3 record to txn table
	err = populateTxnTable(db, 3)
	assert.NoError(t, err)
	assert.Equal(t, 3, rowsInTxnTable(db))

	// config.Rounds > rounds in DB
	config = PruneConfigurations{
		Frequency: "once",
		Rounds:    5,
	}
	dm, err := MakeDataManager(context.Background(), &config, "postgres", connStr, logger)
	assert.NoError(t, err)
	go dm.Delete()
	// wait
	time.Sleep(1 * time.Second)
	// delete didn't happen
	assert.Equal(t, 3, rowsInTxnTable(db))

	// config.Rounds == rounds in DB
	config.Rounds = 3
	dm, err = MakeDataManager(context.Background(), &config, "postgres", connStr, logger)
	assert.NoError(t, err)
	go dm.Delete()
	// wait
	time.Sleep(1 * time.Second)
	// delete didn't happen
	assert.Equal(t, 3, rowsInTxnTable(db))
	//	processor ended
	assert.True(t, dm.Closed())
}

func TestDeleteDaily(t *testing.T) {
	logger.SetOutput(os.Stdout)
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	idb.Close()

	// add 20 record to txn table
	err = populateTxnTable(db, 20)
	assert.NoError(t, err)
	assert.Equal(t, 20, rowsInTxnTable(db))

	config = PruneConfigurations{
		Frequency: "daily",
		Rounds:    15,
	}
	ctx, cf := context.WithCancel(context.Background())
	dm, err := MakeDataManager(ctx, &config, "postgres", connStr, logger)
	assert.NoError(t, err)
	go dm.Delete()
	time.Sleep(1 * time.Second)
	assert.Equal(t, 15, rowsInTxnTable(db))
	// database connection stays open
	assert.False(t, dm.Closed())
	// cancel
	cf()
	// database connection should be closed
	time.Sleep(1 * time.Second)
	assert.True(t, dm.Closed())

	//	reconnect
	config = PruneConfigurations{
		Frequency: "daily",
		Rounds:    10,
	}
	dm, err = MakeDataManager(context.Background(), &config, "postgres", connStr, logger)
	assert.NoError(t, err)
	go dm.Delete()
	time.Sleep(1 * time.Second)
	assert.Equal(t, 10, rowsInTxnTable(db))
}

func TestDeleteRollback(t *testing.T) {
	logger.SetOutput(os.Stdout)
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	idb.Close()

	// remove metastate table
	_, err = db.Exec(context.Background(), "DROP TABLE metastate")
	assert.NoError(t, err)

	// add 10 record to txn table
	err = populateTxnTable(db, 10)
	assert.NoError(t, err)
	assert.Equal(t, 10, rowsInTxnTable(db))

	config = PruneConfigurations{
		Frequency: "once",
		Rounds:    5,
	}
	postgres := Postgressql{
		Config: &config,
		DB:     db,
		Logger: logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = postgres.deleteTransactions(ctx)
	// delete didn't happen
	assert.Equal(t, 10, rowsInTxnTable(db))
	// metastate update failed
	assert.Contains(t, err.Error(), "\"metastate\" does not exist")

}

func TestDeleteTimeout(t *testing.T) {
	logger.SetOutput(os.Stdout)
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	idb.Close()

	// add 10 record to txn table
	err = populateTxnTable(db, 10)
	assert.NoError(t, err)
	assert.Equal(t, 10, rowsInTxnTable(db))

	config = PruneConfigurations{
		Frequency: "once",
		Rounds:    5,
		Timeout:   0,
	}
	postgres := Postgressql{
		Config: &config,
		DB:     db,
		Logger: logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	err = postgres.deleteTransactions(ctx)
	// delete didn't happen
	assert.Equal(t, 10, rowsInTxnTable(db))
	// context deadline exceeded
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func populateTxnTable(db *pgxpool.Pool, n int) error {
	batch := &pgx.Batch{}
	for i := 0; i < n; i++ {
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
