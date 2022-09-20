package util

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/sirupsen/logrus"
)

type Interval string

const (
	once    Interval = "once"
	daily   Interval = "daily"
	timeout          = 5 * time.Second
)

// PruneConfigurations a data type
type PruneConfigurations struct {
	Interval Interval `yaml:"interval"`
	Rounds   uint64   `yaml:"rounds"`
}

type DataManager interface {
	Delete()
}

type Postgressql struct {
	Config *PruneConfigurations
	DB     *pgxpool.Pool
	Logger *logrus.Logger
}

func MakeDataManager(cfg *PruneConfigurations, dbname string, connection string, logger *logrus.Logger) (DataManager, error) {
	if dbname == "postgres" {
		postgresConfig, err := pgxpool.ParseConfig(connection)
		if err != nil {
			return nil, fmt.Errorf("Couldn't parse config: %v", err)
		}
		db, err := pgxpool.ConnectConfig(context.Background(), postgresConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to postgres: %w", err)
		}
		dm := Postgressql{
			Config: cfg,
			DB:     db,
			Logger: logger,
		}
		return dm, nil
	} else {
		return nil, fmt.Errorf("data source %s is not supported", dbname)
	}
}
func (p Postgressql) Delete() {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// set up delete task
	day := 24 * time.Hour
	ticker := time.NewTicker(day)
	run := make(chan bool)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				ticker.Stop()
				return
			case <-run:
				p.deleteTransactions(ctx)
				ticker.Reset(day)
			case <-ticker.C:
				p.deleteTransactions(ctx)
			}
		}
	}()

	// run pruning job base on configured interval
	if p.Config.Interval == once {
		run <- true
		done <- true
	} else if p.Config.Interval == daily {
		// query last pruned date
		var lastPruned time.Time
		query := "SELECT last_pruned from metastate"
		result, err := p.DB.Query(ctx, query)
		if err != nil {
			p.Logger.Warnf(" Delete(): %v", err)
			return
		}
		err = result.Scan(lastPruned)
		if err == nil {
			currentTime := time.Now()
			diff := currentTime.Sub(lastPruned)
			if diff.Hours() < 24 {
				// wait for next run
				p.Logger.Infof("next data pruning in %v hours", diff.Hours())
				time.Sleep(diff * time.Hour)
			}
		} else if err != nil && err != pgx.ErrNoRows {
			p.Logger.Warnf("Delete(): %v", err)
			return
		}

		// prune data
		run <- true

	} else {
		p.Logger.Warnf("%s pruning interval is not supported", p.Config.Interval)
	}

}

func (p Postgressql) deleteTransactions(ctx context.Context) {
	// get latest txn round
	query := "SELECT round FROM txn ORDER BY round DESC LIMIT 1"
	result, err := p.DB.Query(ctx, query)
	if err != nil {
		p.Logger.Warnf("Delete(): %v", err)
		return
	}
	var latestRound uint64
	err = result.Scan(&latestRound)
	if err != nil {
		p.Logger.Warnf("Delete(): %v", err)
		return
	}
	// latest round < desired number of rounds to keep
	if latestRound < p.Config.Rounds {
		// no data to remove
		return
	}
	// oldest round to keep
	keep := latestRound - p.Config.Rounds

	// atomic txn: delete old transactions and update metastate
	deleteTxns := func() {
		// start a transaction
		tx, err := p.DB.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			p.Logger.Warnf("delete transactions: %v", err)
			return
		}
		defer tx.Rollback(ctx)
		p.Logger.Infof("keeping round %d and later", keep)
		query = "DELETE FROM txn WHERE round < $1"
		cmd, err := tx.Exec(ctx, query, keep)
		t := time.Now()
		if err != nil {
			p.Logger.Warnf("transaction delete err %v", err)
			return
		} else {
			// format time, "2006-01-02T15:04:05Z07:00"
			ft := t.Format(time.RFC3339)
			p.Logger.Infof("%d transactions deleted, last pruned at %s", cmd.RowsAffected(), ft)
			// update last_pruned
			metastate := fmt.Sprintf("{last_pruned: %s}", ft)
			encoded, err := json.Marshal(metastate)
			if err != nil {
				p.Logger.Warnf("transaction delete err %v", err)
				return
			}
			query = "INSERT INTO metastate (k,v) VALUES('prune',$1) ON CONFLICT(k) DO UPDATE SET v=EXCLUDED.v"
			_, err = tx.Exec(ctx, query, string(encoded))
			if err != nil {
				p.Logger.Warnf("metastate update err %v", err)
				return
			}
		}
		// Commit the transaction.
		if err = tx.Commit(ctx); err != nil {
			p.Logger.Warnf("delete transactions: %v", err)
		}
	}
	deleteTxns()
}
