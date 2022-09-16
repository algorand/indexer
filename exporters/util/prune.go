package util

import (
	"fmt"
	"strings"

	"github.com/algorand/indexer/idb"
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
	Delete(<-chan uint64)
}

type Postgressql struct {
	Config *PruneConfigurations
	DB     idb.IndexerDb
	Logger *logrus.Logger
}

func MakeDataManager(cfg *PruneConfigurations, dbname string, dataconnection string, logger *logrus.Logger) (DataManager, error) {
	datasource := strings.Split(dataconnection, ":")[0]
	if datasource == "postgres" {
		conn, ready, err := idb.IndexerDbByName(dbname, dataconnection, idb.IndexerDbOptions{ReadOnly: false}, logger)
		if err != nil {
			return nil, err
		}
		<-ready
		dm := Postgressql{
			Config: cfg,
			DB:     conn,
			Logger: logger,
		}
		return dm, nil
	} else {
		return nil, fmt.Errorf("data source %s is not supported", datasource)
	}
}
func (p Postgressql) Delete(rnd <-chan uint64) {
	fmt.Println("TO BE IMPLEMENTED")
}
