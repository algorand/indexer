package util

import (
	"context"
	"fmt"
	"strings"
	"time"

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

func MakeDataManager(cfg *PruneConfigurations, connection string, logger *logrus.Logger) (DataManager, error) {
	datasource := strings.Split(connection, ":")[0]
	if datasource == "postgres" {
		postgresConfig, err := pgxpool.ParseConfig(connection)
		if err != nil {
			return nil, fmt.Errorf("Couldn't parse config: %v", err)
		}
		db, err := pgxpool.ConnectConfig(context.Background(), postgresConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to postgres: %v", err)
		}
		dm := Postgressql{
			Config: cfg,
			DB:     db,
			Logger: logger,
		}
		return dm, nil
	} else {
		return nil, fmt.Errorf("data source %s is not supported", datasource)
	}
}
func (p Postgressql) Delete(ctx context.Context, rnd <-chan uint64) {
	select {
	case round := <-rnd:
		p.Logger.Infof("received round %d", round)
		p.deleteTransactions(round)
	case <-ctx.Done():
		p.Logger.Info("exporter closed")
		p.DB.Close()
	}
}

func (p Postgressql) deleteTransactions(round uint64) {
	keep := round - p.Config.Rounds
	prune := true
	dbctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if p.Config.Interval != once {
		currentTime := time.Now()
		query := "SELECT last_pruned from metastate"
		result, err := p.DB.Query(dbctx, query)
		if err != nil {
			p.Logger.Warnf("err %v", err)
		}
		var last_pruned time.Time
		err = result.Scan(last_pruned)
		if err != nil {
			p.Logger.Warnf("err %v", err)
		}
		diff := currentTime.Sub(last_pruned)
		if diff.Hours() < 24 {
			prune = false
		}
	}
	if keep > 0 && prune {
		//	sql: DELETE FROM txn WHERE round < keep;
		query := "DELETE FROM txn WHERE round < keep"
		cmd, err := p.DB.Exec(dbctx, query)
		if err != nil {
			p.Logger.Warnf("err %v", err)
		} else {
			p.Logger.Infof("%d transactions deleted", cmd.RowsAffected())
			// sql: update last_pruned
			metastate := fmt.Sprintf("{last_pruned: %s}", time.Now())
			query = "INSERT INTO metastate (k,v) VALUES('prune',$1) ON CONFLICT(k) DO UPDATE SET v=EXCLUDED.v"
			_, err = p.DB.Exec(dbctx, query, metastate)
			if err != nil {
				p.Logger.Warnf("err %v", err)
			}
		}
	}
}
