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
	cf       context.CancelFunc
	duration time.Duration
}

// MakeDataManager initializes resources need for removing data from data source
func MakeDataManager(ctx context.Context, round uint64, cfg *PruneConfigurations, db idb.IndexerDb, logger *logrus.Logger) DataManager {
	c, cf := context.WithCancel(ctx)

	dm := &postgresql{
		config:   cfg,
		db:       db,
		logger:   logger,
		ctx:      c,
		cf:       cf,
		duration: d,
	}
	// delete transaction at start up when data pruning is enabled
	if round > cfg.Rounds {
		keep := round - cfg.Rounds + 1
		_, err := dm.db.DeleteTransactions(dm.ctx, keep)
		if err != nil {
			logger.Warnf("MakeDataManager(): data pruning err: %v", err)
		}
	}
	return dm
}

// Delete removes data from the txn table in Postgres DB
func (p *postgresql) Delete(wg *sync.WaitGroup, round *uint64) {

	defer func() {
		p.cf()
		wg.Done()
	}()
	// oldest round to keep
	keep := *round
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-time.After(p.duration):
			currentRound := *round
			if currentRound > p.config.Rounds && currentRound-keep >= uint64(p.config.Interval) {
				keep = currentRound - p.config.Rounds + 1
				_, err := p.db.DeleteTransactions(p.ctx, keep)
				if err != nil {
					p.logger.Warnf("Delete(): data pruning err: %v", err)
					return
				}
			}
		}
	}
}
