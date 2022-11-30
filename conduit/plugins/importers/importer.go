package importers

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/data"

	"github.com/algorand/go-algorand/data/bookkeeping"
)

// Importer defines the interface for importer plugins
type Importer interface {
	// PluginMetadata implement this interface.
	conduit.PluginMetadata

	// Init will initialize each importer with a given config. This config will contain the Unmarhsalled config file specific to this plugin.
	// It is called during initialization of an importer plugin such as setting up network connections, file buffers etc.
	// Importers will also be responsible for returning a valid Genesis object pointer
	Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*bookkeeping.Genesis, error)

	// Config returns the configuration options used to create an Importer. Initialized during Init.
	Config() plugins.PluginConfig

	// Close function is used for closing network connections, files, flushing buffers etc.
	Close() error

	// GetBlock given any round number-rnd fetches the block at that round
	// It returns an object of type BlockData defined in data
	GetBlock(rnd uint64) (data.BlockData, error)
}
