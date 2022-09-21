package util

import (
	"context"
	"sync"
	"time"

	"github.com/algorand/indexer/idb"
	"github.com/sirupsen/logrus"
)

// Frequency determines how often to delete data
type Frequency string

const (
	once           Frequency = "once"
	daily          Frequency = "daily"
	day                      = 24 * time.Hour
	defaultTimeout uint64    = 5
)

// PruneConfigurations contains the configurations for data pruning
type PruneConfigurations struct {
	Frequency Frequency `yaml:"frequency"`
	Rounds    uint64    `yaml:"rounds"`
	Timeout   uint64    `yaml:"timeout"`
}

// DataManager is a data pruning interface
type DataManager interface {
	Delete(*sync.WaitGroup, chan error)
}

type postgresql struct {
	config *PruneConfigurations
	db     idb.IndexerDb
	logger *logrus.Logger
	ctx    context.Context
	cf     context.CancelFunc
}

// MakeDataManager initializes resources need for removing data from data source
func MakeDataManager(ctx context.Context, cfg *PruneConfigurations, db idb.IndexerDb, logger *logrus.Logger) DataManager {
	c, cf := context.WithCancel(ctx)
	dm := postgresql{
		config: cfg,
		db:     db,
		logger: logger,
		ctx:    c,
		cf:     cf,
	}
	return dm
}

// Delete removes data from the txn table in Postgres DB
func (p postgresql) Delete(wg *sync.WaitGroup, errChan chan error) {
	defer wg.Done()
	timeout := defaultTimeout
	if p.config.Timeout > 0 {
		timeout = p.config.Timeout
	}
	// exec pruning job base on configured interval
	p.logger.Info("start data pruning")
	if p.config.Frequency == once {
		_, err := p.db.DeleteTransactions(p.ctx, p.config.Rounds, time.Duration(timeout)*time.Second)
		if err != nil {
			p.logger.Warnf("exec: data pruning err: %v", err)
			errChan <- err
		}
		return
	} else if p.config.Frequency == daily {
		// set up a 24-hour ticker
		ticker := time.NewTicker(day)

		// execute recurring delete
		exec := make(chan bool, 1)
		exec <- true
		for {
			select {
			case <-p.ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				exec <- true
			case <-exec:
				_, err := p.db.DeleteTransactions(p.ctx, p.config.Rounds, time.Duration(timeout)*time.Second)
				if err != nil {
					p.logger.Warnf("exec: data pruning err: %v", err)
					errChan <- err
					return
				}
				ticker.Reset(day)
			}
		}

	} else {
		p.logger.Warnf("%s pruning interval is not supported", p.config.Frequency)
	}

}
