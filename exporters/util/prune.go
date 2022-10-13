package util

import (
	"context"
	"sync"
	"time"

	"github.com/algorand/indexer/idb"
	"github.com/sirupsen/logrus"
)

// Interval determines how often to delete data
type Interval int

const (
	once     Interval = -1
	disabled Interval = 0
	d                 = 2 * time.Second
)

// PruneConfigurations contains the configurations for data pruning
type PruneConfigurations struct {
	// Rounds to keep
	Rounds uint64 `yaml:"rounds"`
	// Interval used to prune the data. The values can be -1 to run at startup,
	// 0 to disable or N to run every N rounds.
	Interval Interval `yaml:"interval"`
}

// DataManager is a data pruning interface
type DataManager interface {
	Delete(*sync.WaitGroup, *uint64)
}

type postgresql struct {
	config   *PruneConfigurations
	db       idb.IndexerDb
	logger   *logrus.Logger
	ctx      context.Context
	duration time.Duration
}

// MakeDataManager initializes resources need for removing data from data source
func MakeDataManager(ctx context.Context, cfg *PruneConfigurations, db idb.IndexerDb, logger *logrus.Logger) DataManager {

	dm := &postgresql{
		config:   cfg,
		db:       db,
		logger:   logger,
		ctx:      ctx,
		duration: d,
	}

	return dm
}

// Delete removes data from the txn table in Postgres DB
func (p *postgresql) Delete(wg *sync.WaitGroup, nextRound *uint64) {

	defer wg.Done()
	// round value used for interval calculation
	round := *nextRound
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-time.After(p.duration):
			currentRound := *nextRound
			keep := currentRound - p.config.Rounds
			if p.config.Interval == -1 {
				// delete transaction at start up when data pruning is enabled
				if currentRound > p.config.Rounds {
					// keep, remove data older than keep
					_, err := p.db.DeleteTransactions(p.ctx, keep)
					if err != nil {
						p.logger.Warnf("MakeDataManager(): data pruning err: %v", err)
					}
				}
				return
			} else {
				// *nextRound should increment as exporter receives new block
				if currentRound > p.config.Rounds && currentRound-round >= uint64(p.config.Interval) {
					_, err := p.db.DeleteTransactions(p.ctx, keep)
					if err != nil {
						p.logger.Warnf("Delete(): data pruning err: %v", err)
						return
					}
					// update round value for next interval calculation
					round = currentRound
				}
			}
		}
	}
}
