package exporters

import (
	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

// ExporterConfig is a generic string which can be deserialized by each individual Exporter.
type ExporterConfig string
type ExportData interface {
	Round() uint64
}

// BlockExportData is provided to the Exporter on each round.
type BlockExportData struct {
	// Block is the block data written to the blockchain.
	Block bookkeeping.Block

	// Delta contains a list of account changes resulting from the block. Processor plugins may have modify this data.
	Delta ledgercore.StateDelta

	// Certificate contains voting data that certifies the block. The certificate is non deterministic, a node stops collecting votes once the voting threshold is reached.
	Certificate agreement.Certificate
}

func (blkData *BlockExportData) Round() uint64 {
	return uint64(blkData.Block.Round())
}

// Exporter defines the interface for plugins
type Exporter interface {
	// Name is a UID for each Exporter.
	Name() string

	// Connect will be called during initialization, before block data starts going through the pipeline.
	// Typically used for things like initializing network connections.
	// The ExporterConfig passed to Connect will contain the Unmarhsalled config file specific to this plugin.
	// Should return an error if it fails--this will result in the Indexer process terminating.
	Connect(cfg ExporterConfig) error

	// Config returns the configuration options used to create an Exporter.
	// Initialized during Connect, it should return nil until the Exporter has been Connected.
	Config() ExporterConfig

	// Disconnect will be called during termination of the Indexer process.
	// There is no guarantee that plugin lifecycle hooks will be invoked in any specific order in relation to one another.
	// Returns an error if it fails which will be surfaced in the logs, but the process is already terminating.
	Disconnect() error

	// Receive is called for each block to be processed by the exporter.
	// Should return an error on failure--retries are configurable.
	Receive(exportData ExportData) error

	// HandleGenesis is an Exporter's opportunity to do initial validation and handling of the Genesis block.
	// If validation (such as a check to ensure `genesis` matches a previously stored genesis block) or handling fails,
	// it returns an error.
	HandleGenesis(genesis bookkeeping.Genesis) error

	// Round returns the next round not yet processed by the Exporter. Atomically updated when Receive successfully completes.
	Round() uint64
}
