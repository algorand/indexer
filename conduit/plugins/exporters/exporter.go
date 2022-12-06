package exporters

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/data"
)

// Exporter defines the interface for plugins
type Exporter interface {
	// PluginMetadata implement this interface.
	conduit.PluginMetadata

	// Init will be called during initialization, before block data starts going through the pipeline.
	// Typically used for things like initializing network connections.
	// The ExporterConfig passed to Connect will contain the Unmarhsalled config file specific to this plugin.
	// Should return an error if it fails--this will result in the Indexer process terminating.
	Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error

	// Config returns the configuration options used to create an Exporter.
	// Initialized during Connect, it should return nil until the Exporter has been Connected.
	Config() string

	// Close will be called during termination of the Indexer process.
	// There is no guarantee that plugin lifecycle hooks will be invoked in any specific order in relation to one another.
	// Returns an error if it fails which will be surfaced in the logs, but the process is already terminating.
	Close() error

	// Receive is called for each block to be processed by the exporter.
	// Should return an error on failure--retries are configurable.
	Receive(exportData data.BlockData) error
}
