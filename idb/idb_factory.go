package idb

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// IndexerDbFactory is used to install an IndexerDb implementation.
type IndexerDbFactory interface {
	Name() string
	Build(arg string, opts IndexerDbOptions, log *log.Logger) (IndexerDb, chan struct{}, error)
}

// This layer of indirection allows for different db integrations to be compiled in or compiled out by `go build --tags ...`
var indexerFactories map[string]IndexerDbFactory

// RegisterFactory is used by IndexerDb implementations to register their implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// driver's are configured and used.
func RegisterFactory(name string, factory IndexerDbFactory) {
	indexerFactories[name] = factory
}

// IndexerDbByName is used to construct an IndexerDb object by name.
// Returns an IndexerDb object, an availability channel that closes when the database
// becomes available, and an error object.
func IndexerDbByName(name, arg string, opts IndexerDbOptions, log *log.Logger) (IndexerDb, chan struct{}, error) {
	if val, ok := indexerFactories[name]; ok {
		return val.Build(arg, opts, log)
	}
	return nil, nil, fmt.Errorf("no IndexerDb factory for %s", name)
}

func init() {
	indexerFactories = make(map[string]IndexerDbFactory)
}
