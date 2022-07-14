package dummy

import (
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
func (df dummyFactory) Build(arg string, opts idb.IndexerDbOptions, log *log.Logger) (idb.IndexerDb, chan struct{}, error) {
	ch := make(chan struct{})
	close(ch)
	return &dummyIndexerDb{log: log}, ch, nil
}

func init() {
	idb.RegisterFactory("dummy", &dummyFactory{})
}
