package exporters

import (
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/loggers"
	"github.com/algorand/indexer/plugins"
)

// Exporter defines the interface for plugins
type Exporter interface {
	// Metadata associated with each Exporter.
	Metadata() ExporterMetadata

	// Init will be called during initialization, before block data starts going through the pipeline.
	// Typically used for things like initializing network connections.
	// The ExporterConfig passed to Connect will contain the Unmarhsalled config file specific to this plugin.
	// Should return an error if it fails--this will result in the Indexer process terminating.
	Init(cfg plugins.PluginConfig, logger *loggers.MT) error

	// Config returns the configuration options used to create an Exporter.
	// Initialized during Connect, it should return nil until the Exporter has been Connected.
	Config() plugins.PluginConfig

	// Close will be called during termination of the Indexer process.
	// There is no guarantee that plugin lifecycle hooks will be invoked in any specific order in relation to one another.
	// Returns an error if it fails which will be surfaced in the logs, but the process is already terminating.
	Close() error

	// Receive is called for each block to be processed by the exporter.
	// Should return an error on failure--retries are configurable.
	Receive(exportData data.BlockData) error

	// HandleGenesis is an Exporter's opportunity to do initial validation and handling of the Genesis block.
	// If validation (such as a check to ensure `genesis` matches a previously stored genesis block) or handling fails,
	// it returns an error.
	HandleGenesis(genesis bookkeeping.Genesis) error

	// Round returns the next round not yet processed by the Exporter. Atomically updated when Receive successfully completes.
	Round() uint64
}
