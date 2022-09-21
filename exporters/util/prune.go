package util

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/sirupsen/logrus"
)

// Frequency determines how often to delete data
type Frequency string

const (
	once    Frequency = "once"
	daily   Frequency = "daily"
	timeout uint64    = 5
	day               = 24 * time.Hour
)

// PruneConfigurations contains the configurations for data pruning
type PruneConfigurations struct {
	Frequency Frequency `yaml:"frequency"`
	Rounds    uint64    `yaml:"rounds"`
	Timeout   uint64    `yaml:"timeout"`
}

// DataManager is a data pruning interface
type DataManager interface {
	Delete(*sync.WaitGroup)
	Closed() bool
}

type postgresql struct {
	config *PruneConfigurations
	db     *pgxpool.Pool
	logger *logrus.Logger
	ctx    context.Context
	cf     context.CancelFunc
	test   bool
}

// MakeDataManager initializes resources need for removing data from data source
func MakeDataManager(ctx context.Context, cfg *PruneConfigurations, dbname string, connection string, logger *logrus.Logger) (DataManager, error) {
	if dbname == "postgres" {
		postgresConfig, err := pgxpool.ParseConfig(connection)
		if err != nil {
			return nil, fmt.Errorf("Couldn't parse config: %v", err)
		}
		db, err := pgxpool.ConnectConfig(context.Background(), postgresConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to postgres: %w", err)
		}

		c, cf := context.WithCancel(ctx)
		dm := postgresql{
			config: cfg,
			db:     db,
			logger: logger,
			ctx:    c,
			cf:     cf,
		}
		return dm, nil
	}
	return nil, fmt.Errorf("data source %s is not supported", dbname)
}

// Delete removes data from the txn table in Postgres DB
func (p postgresql) Delete(wg *sync.WaitGroup) {

	// set up a 24-hour ticker
	ticker := time.NewTicker(day)

	// close resources
	defer func() {
		ticker.Stop()
		p.db.Close()
		wg.Done()
	}()

	exec := make(chan bool)
	done := make(chan bool)

	// execute delete
	go func() {
		for {
			select {
			case <-ticker.C:
				exec <- true
			case <-exec:
				p.logger.Info("start data pruning")
				err := p.deleteTransactions()
				if err != nil {
					p.logger.Warnf("exec: data pruning err: %v", err)
				}
				ticker.Reset(day)
				done <- true
			}
		}
	}()

	// exec pruning job base on configured interval
	if p.config.Frequency == once {
		exec <- true
		<-done
		return
	} else if p.config.Frequency == daily {
		// query last pruned time
		var prunedms map[string]time.Time
		query := "SELECT k from metastate WHERE k='pruned'"
		err := p.db.QueryRow(p.ctx, query).Scan(&prunedms)
		if err != nil && err != pgx.ErrNoRows {
			p.logger.Warnf(" Delete() metastate: %v", err)
			return
		}

		// prune data
		exec <- true
		<-p.ctx.Done()
	} else {
		p.logger.Warnf("%s pruning interval is not supported", p.config.Frequency)
	}
}

// Closed indicates whether the process has exited Delete() and closed DB connection.
func (p postgresql) Closed() bool {
	return p.db.Stat().TotalConns() == 0
}

func (p postgresql) deleteTransactions() error {
	sec := timeout
	// allow any timeout value for testing
	if p.test || p.config.Timeout > 0 {
		sec = p.config.Timeout
	}

	ctx, cancel := context.WithTimeout(p.ctx, time.Duration(sec)*time.Second)
	defer cancel()

	// get latest txn round
	var latestRound uint64
	query := "SELECT round FROM txn ORDER BY round DESC LIMIT 1"
	err := p.db.QueryRow(ctx, query).Scan(&latestRound)
	if err != nil {
		return fmt.Errorf("deleteTransactions: %v", err)
	}
	p.logger.Infof("last round in database %d", latestRound)
	// latest round < desired number of rounds to keep
	if latestRound < p.config.Rounds {
		// no data to remove
		return nil
	}

	// oldest round to keep
	keep := latestRound - p.config.Rounds + 1

	// atomic txn: delete old transactions and update metastate
	deleteTxns := func() error {
		// start a transaction
		tx, err2 := p.db.BeginTx(ctx, pgx.TxOptions{})
		if err2 != nil {
			return fmt.Errorf("delete transactions: %w", err2)
		}
		defer tx.Rollback(ctx)

		p.logger.Infof("keeping round %d and later", keep)
		// delete query
		query = "DELETE FROM txn WHERE round < $1"
		cmd, err2 := tx.Exec(ctx, query, keep)
		if err2 != nil {
			return fmt.Errorf("transaction delete err %w", err2)
		}
		t := time.Now()
		// update last_pruned in metastate
		// format time, "2006-01-02T15:04:05Z07:00"
		ft := t.Format(time.RFC3339)
		metastate := fmt.Sprintf("{last_pruned: %s}", ft)
		encoded, err2 := json.Marshal(metastate)
		if err2 != nil {
			return fmt.Errorf("transaction delete err %w", err2)
		}
		query = "INSERT INTO metastate (k,v) VALUES('prune',$1) ON CONFLICT(k) DO UPDATE SET v=EXCLUDED.v"
		_, err2 = tx.Exec(ctx, query, string(encoded))
		if err2 != nil {
			return fmt.Errorf("metastate update err %w", err2)
		}
		// Commit the transaction.
		if err = tx.Commit(ctx); err2 != nil {
			return fmt.Errorf("delete transactions: %w", err2)
		}
		p.logger.Infof("%d transactions deleted, last pruned at %s", cmd.RowsAffected(), ft)
		return nil
	}
	// retry
	for i := 1; i <= 3; i++ {
		err = deleteTxns()
		if err == nil {
			return nil
		}
		p.logger.Infof("data pruning retry %d", i)
	}
	return err
}
