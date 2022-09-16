package util

import (
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
)

type Interval int

const (
	once Interval = iota
	daily
)

// PruneConfigurations a data type
type PruneConfigurations struct {
	Interval Interval `yaml:"interval"`
	Rounds   uint64   `yaml:"rounds"`
}

type DataManager interface {
	Delete(<-chan uint64)
}

type postgresDB struct {
	configs *PruneConfigurations
	db      idb.IndexerDb
	logger  *logrus.Logger
}

func MakeDataManager(pc *plugins.PluginConfig, logger *logrus.Logger) DataManager {
	db := postgresDB{
		configs: nil,
		db:      nil,
		logger:  nil,
	}
	return db
}
func (p postgresDB) Delete(rnd <-chan uint64) {}
