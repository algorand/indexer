package util

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/algorand/go-codec/codec"
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
var hook *test.Hook
var jsonCodecHandle *codec.JsonHandle
var ch chan uint64
var wg sync.WaitGroup

// encodeJSON converts an object into JSON
func encodeJSON(obj interface{}) []byte {
	var buf []byte
	enc := codec.NewEncoderBytes(&buf, jsonCodecHandle)
	enc.MustEncode(obj)
	return buf
}
func init() {
	logger, hook = test.NewNullLogger()
	jsonCodecHandle = new(codec.JsonHandle)
	ch = make(chan uint64)
}

var config = PruneConfigurations{
	Interval: -1,
	Rounds:   10,
	Timeout:  3,
}

type ImportState struct {
	NextRoundToAccount uint64 `codec:"next_account_round"`
}

func delete(dm DataManager, cancel context.CancelFunc, round uint64) {
	wg.Add(1)
	go dm.Delete(&wg, ch)
	ch <- round
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()
	wg.Wait()
}
func TestDeleteEmptyTxnTable(t *testing.T) {
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	defer idb.Close()

	// empty txn table
	_, err = populateTxnTable(db, 1, 0)
	assert.NoError(t, err)
	assert.Equal(t, 0, rowsInTxnTable(db))

	// data manager
	ctx, cancel := context.WithCancel(context.Background())
	dm := MakeDataManager(ctx, &config, idb, logger)
	delete(dm, cancel, 0)
	assert.Equal(t, 0, rowsInTxnTable(db))
}

func TestDeleteTxns(t *testing.T) {
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	defer idb.Close()

	// add 20 records to txn table
	ntxns := 20
	_, err = populateTxnTable(db, 1, ntxns)
	assert.NoError(t, err)
	assert.Equal(t, ntxns, rowsInTxnTable(db))

	// data manager
	ctx, cancel := context.WithCancel(context.Background())
	dm := MakeDataManager(ctx, &config, idb, logger)
	delete(dm, cancel, uint64(ntxns))
	// 10 rounds removed
	assert.Equal(t, 10, rowsInTxnTable(db))
	// check remaining rounds are correct
	assert.True(t, validateTxnTable(db, 11, 20))
}

func TestDeleteConfigs(t *testing.T) {
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	defer idb.Close()

	// add 3 records to txn table
	ntxns := 3
	_, err = populateTxnTable(db, 1, ntxns)
	assert.NoError(t, err)
	assert.Equal(t, ntxns, rowsInTxnTable(db))

	// config.Rounds > rounds in DB
	config = PruneConfigurations{
		Interval: -1,
		Rounds:   5,
	}
	ctx, cancel := context.WithCancel(context.Background())
	dm := MakeDataManager(ctx, &config, idb, logger)
	assert.NoError(t, err)
	delete(dm, cancel, uint64(ntxns))
	// delete didn't happen
	assert.Equal(t, 3, rowsInTxnTable(db))

	// config.Rounds == rounds in DB
	config.Rounds = 3
	ctx, cancel = context.WithCancel(context.Background())
	dm = MakeDataManager(ctx, &config, idb, logger)
	assert.NoError(t, err)
	delete(dm, cancel, uint64(ntxns))
	// delete didn't happen
	assert.Equal(t, 3, rowsInTxnTable(db))

	// unsupported interval
	config = PruneConfigurations{
		Rounds:   1,
		Interval: -2,
	}
	ctx, cancel = context.WithCancel(context.Background())
	dm = MakeDataManager(ctx, &config, idb, logger)
	assert.NoError(t, err)
	delete(dm, cancel, uint64(ntxns))
	// delete didn't happen
	assert.Equal(t, 3, rowsInTxnTable(db))
	assert.Contains(t, hook.LastEntry().Message, "Invalid Interval value")
}

func TestDeleteInterval(t *testing.T) {
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// init the tables
	idb, _, err := postgres.OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	defer idb.Close()

	// add 5 records to txn table
	ntxns := 5
	nextRound, err := populateTxnTable(db, 1, ntxns)
	assert.NoError(t, err)
	assert.Equal(t, ntxns, rowsInTxnTable(db))

	config = PruneConfigurations{
		Interval: 3,
		Rounds:   3,
		Timeout:  5,
	}
	ctx, cancel := context.WithCancel(context.Background())
	dm := MakeDataManager(ctx, &config, idb, logger)
	wg.Add(1)
	go dm.Delete(&wg, ch)
	// send current round; data removed at startup
	ch <- uint64(nextRound - 1)
	time.Sleep(1 * time.Second)
	assert.Equal(t, 3, rowsInTxnTable(db))

	// remove data every 3 round
	// add round 6. no data removed
	nextRound, err = populateTxnTable(db, nextRound, 1)
	assert.NoError(t, err)
	ch <- uint64(nextRound - 1)
	time.Sleep(1 * time.Second)
	assert.Equal(t, 4, rowsInTxnTable(db))

	// add round 7. no data removed
	nextRound, err = populateTxnTable(db, nextRound, 1)
	assert.NoError(t, err)
	ch <- uint64(nextRound - 1)
	time.Sleep(1 * time.Second)
	assert.Equal(t, 5, rowsInTxnTable(db))

	// add round 8. data removed
	nextRound, err = populateTxnTable(db, nextRound, 1)
	assert.NoError(t, err)
	ch <- uint64(nextRound - 1)
	time.Sleep(1 * time.Second)
	assert.Equal(t, 3, rowsInTxnTable(db))

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()
	wg.Wait()

	// reconnect
	config = PruneConfigurations{
		Interval: -1,
		Rounds:   1,
		Timeout:  5,
	}
	ctx, cancel = context.WithCancel(context.Background())
	dm = MakeDataManager(ctx, &config, idb, logger)
	delete(dm, cancel, uint64(nextRound-1))
	assert.Equal(t, 1, rowsInTxnTable(db))
}

// populate n records starting with round starting at r.
// return next round
func populateTxnTable(db *pgxpool.Pool, r int, n int) (int, error) {
	batch := &pgx.Batch{}
	// txn round starts at 1 because genesis block is empty
	for i := 1; i <= n; i++ {
		query := "INSERT INTO txn(round, intra, typeenum,asset,txn,extra) VALUES ($1,$2,$3,$4,$5,$6)"
		batch.Queue(query, r, i, 1, 0, "{}", "{}")
		r++
	}
	results := db.SendBatch(context.Background(), batch)
	defer results.Close()
	for i := 0; i < batch.Len(); i++ {
		_, err := results.Exec()
		if err != nil {
			return 0, fmt.Errorf("populateTxnTable() exec err: %w", err)
		}
	}
	return r, nil
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
	for res.Next() {
		err = res.Scan(&round)
		if err != nil || round != expected {
			return false
		}
		expected++
	}
	return expected-1 == last
}
