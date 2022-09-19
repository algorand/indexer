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
	once  Interval = "once"
	daily Interval = "daily"
)

// PruneConfigurations a data type
type PruneConfigurations struct {
	Interval Interval `yaml:"interval"`
	Rounds   uint64   `yaml:"rounds"`
}

type DataManager interface {
	Delete(context.Context, <-chan uint64)
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
func (p Postgressql) Delete(ctx context.Context, rnd <-chan uint64) {
	select {
	case round := <-rnd:
		p.Logger.Infof("received exporter round %d", round)
		p.deleteTransactions(round)
	case <-ctx.Done():
		p.Logger.Info("exporter closed")
		p.DB.Close()
	}
}

func (p Postgressql) deleteTransactions(round uint64) {
	// current exporter round < desired number of rounds to keep
	if round < p.Config.Rounds {
		// no data to remove
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// oldest round to keep
	keep := round - p.Config.Rounds

	// delete old txns and update metastate
	deleteTxns := func() {
		// start a transaction
		tx, err := p.DB.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			p.Logger.Warnf("delete transactions: %v", err)
			return
		}
		defer tx.Rollback(ctx)
		p.Logger.Infof("keeping round %d and later", keep)
		query := "DELETE FROM txn WHERE round < $1"
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

	if p.Config.Interval == once {
		deleteTxns()
	} else if p.Config.Interval == daily {
		prune := false
		var lastPruned time.Time
		query := "SELECT last_pruned from metastate"
		result, err := p.DB.Query(ctx, query)
		if err != nil {
			p.Logger.Warnf("err %v", err)
		}
		err = result.Scan(lastPruned)
		if err == pgx.ErrNoRows {
			// first prune
			prune = true
		} else if err != nil {
			p.Logger.Warnf("err %v", err)
		} else {
			currentTime := time.Now()
			diff := currentTime.Sub(lastPruned)
			if diff.Hours() >= 24 {
				prune = true
			}
		}
		if prune {
			deleteTxns()
		}
	} else {
		p.Logger.Warnf("%s pruning interval is not supported", p.Config.Interval)
	}

}
