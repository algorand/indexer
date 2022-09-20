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

type Frequency string

const (
	once    Frequency = "once"
	daily   Frequency = "daily"
	timeout uint64    = 15
	day               = 24 * time.Hour
)

// PruneConfigurations a data type
type PruneConfigurations struct {
	Frequency Frequency `yaml:"frequency"`
	Rounds    uint64    `yaml:"rounds"`
	Timeout   uint64    `yaml:"timeout"`
}

type DataManager interface {
	Delete()
	Closed() bool
}

type Postgressql struct {
	Config *PruneConfigurations
	DB     *pgxpool.Pool
	Logger *logrus.Logger
	Ctx    context.Context
}

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

		sec := timeout
		if cfg.Timeout > 0 {
			sec = cfg.Timeout
		}
		c, _ := context.WithTimeout(ctx, time.Duration(sec)*time.Second)
		dm := Postgressql{
			Config: cfg,
			DB:     db,
			Logger: logger,
			Ctx:    c,
		}
		return dm, nil
	} else {
		return nil, fmt.Errorf("data source %s is not supported", dbname)
	}
}
func (p Postgressql) Delete() {
	// close db connection
	defer p.DB.Close()

	// set up delete task
	ticker := time.NewTicker(day)
	exec := make(chan bool)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-ticker.C:
				exec <- true
			case <-exec:
				p.Logger.Info("start data pruning")
				err := p.deleteTransactions(p.Ctx)
				if err != nil {
					p.Logger.Warnf("exec: data pruning err: %v", err)
				}
				ticker.Reset(day)
				done <- true
			}
		}
	}()

	// exec pruning job base on configured interval
	if p.Config.Frequency == once {
		exec <- true
		<-done
		ticker.Stop()
		return
	} else if p.Config.Frequency == daily {
		// query last pruned time
		var prunedms map[string]time.Time
		query := "SELECT k from metastate WHERE k='pruned'"
		err := p.DB.QueryRow(p.Ctx, query).Scan(&prunedms)
		if err != nil && err != pgx.ErrNoRows {
			p.Logger.Warnf(" Delete() metastate: %v", err)
			return
		}

		// prune data
		exec <- true
		<-p.Ctx.Done()
	} else {
		p.Logger.Warnf("%s pruning interval is not supported", p.Config.Frequency)
	}
}
func (p Postgressql) Closed() bool {
	return p.DB.Stat().TotalConns() == 0
}
func (p Postgressql) deleteTransactions(ctx context.Context) error {
	// get latest txn round
	var latestRound uint64
	query := "SELECT round FROM txn ORDER BY round DESC LIMIT 1"
	err := p.DB.QueryRow(ctx, query).Scan(&latestRound)
	if err != nil {
		fmt.Errorf("deleteTransactions: %v", err)
		return err
	}
	p.Logger.Infof("last round in database %d", latestRound)
	// latest round < desired number of rounds to keep
	if latestRound < p.Config.Rounds {
		// no data to remove
		return nil
	}
	// oldest round to keep
	keep := latestRound - p.Config.Rounds + 1

	// atomic txn: delete old transactions and update metastate
	deleteTxns := func() error {
		// start a transaction
		tx, err2 := p.DB.BeginTx(ctx, pgx.TxOptions{})
		if err2 != nil {
			fmt.Errorf("delete transactions: %w", err)
			return err2
		}
		defer tx.Rollback(ctx)

		p.Logger.Infof("keeping round %d and later", keep)
		// delete query
		query = "DELETE FROM txn WHERE round < $1"
		cmd, err2 := tx.Exec(ctx, query, keep)
		if err2 != nil {
			fmt.Errorf("transaction delete err %w", err2)
			return err2
		}
		t := time.Now()
		// update last_pruned in metastate
		// format time, "2006-01-02T15:04:05Z07:00"
		ft := t.Format(time.RFC3339)
		metastate := fmt.Sprintf("{last_pruned: %s}", ft)
		encoded, err2 := json.Marshal(metastate)
		if err2 != nil {
			fmt.Errorf("transaction delete err %w", err2)
			return err2
		}
		query = "INSERT INTO metastate (k,v) VALUES('prune',$1) ON CONFLICT(k) DO UPDATE SET v=EXCLUDED.v"
		_, err2 = tx.Exec(ctx, query, string(encoded))
		if err2 != nil {
			fmt.Errorf("metastate update err %w", err2)
			return err2
		}
		// Commit the transaction.
		if err = tx.Commit(ctx); err2 != nil {
			fmt.Errorf("delete transactions: %w", err2)
		}
		p.Logger.Infof("%d transactions deleted, last pruned at %s", cmd.RowsAffected(), ft)
		return nil
	}
	// retry
	for i := 1; i < 3; i++ {
		err = deleteTxns()
		if err == nil {
			return nil
		}
	}
	return err
}
