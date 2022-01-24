package dummy

import (
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/idb"
)

type dummyFactory struct {
}

// Name is part of the IndexerFactory interface.
func (df dummyFactory) Name() string {
	return "dummy"
}

// Build is part of the IndexerFactory interface.
func (df dummyFactory) Build(config *pgxpool.Config, opts idb.IndexerDbOptions, log *log.Logger) (idb.IndexerDb, chan struct{}, error) {
	return &dummyIndexerDb{log: log}, nil, nil
}

func init() {
	idb.RegisterFactory("dummy", &dummyFactory{})
}
