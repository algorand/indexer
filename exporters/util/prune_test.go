package util_test

import (
	"context"
	"testing"

	"github.com/algorand/indexer/exporters/util"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres"
	pgtest "github.com/algorand/indexer/idb/postgres/testing"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

var logger *logrus.Logger

func init() {
	logger, _ = test.NewNullLogger()
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
