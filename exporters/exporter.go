package exporters

import "github.com/algorand/go-algorand/rpcs"

// ExporterConfig is a generic type providing serialization/deserialization for exporter config files
type ExporterConfig interface{}

// Exporter defines the interface for plugins
type Exporter interface {
	// Connect will be called during initialization, before block data starts going through the pipeline.
	// Typically used for things like initializing network connections.
	// The ExporterConfig passed to Connect will contain the Unmarhsalled config file specific to this plugin.
	// Should return an error if it fails--this will result in the Indexer process terminating.
	Connect(cfg ExporterConfig) error

	// Disconnect will be called during termination of the Indexer process.
	// There is no guarantee that plugin lifecycle hooks will be invoked in any specific order in relation to one another.
	// Returns an error if it fails which will be surfaced in the logs, but the process is already terminating.
	Disconnect() error

	// Receive is called for each block to be processed by the exporter.
	// Should return an error on failure--retries are configurable.
	Receive(blockData *rpcs.EncodedBlockCert) error

	// Round returns the next round not yet processed by the Exporter. Atomically updated when Receive successfully completes.
	Round() uint64
}
