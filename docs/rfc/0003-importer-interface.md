# Importer Interface RFC

- Contribution Name: Importer Interface/Plugin Framework
- Implementation Owner: Ganesh Vanahalli

## Problem Statement

Users would like to choose how to fetch blocks, either by downloading from an algod rest endpoint or directly from the gossip network or read blocks from a file which was previously written by the exporter.

The current implementation of indexer only allows downloading blocks sequentially from an algod rest endpoint starting at the round number of recently written block to postgres db and users cannot choose a specific block to download.

## Proposal

Importer interface allows users to fetch any particular block either from algod rest endpoint (or directly from the network or from a file written to by the exporter plugin).

### Plugin Interface
Importer plugins that are native to the Indexer (maintained within the Indexer repository) will each implementation the importer interface:

```GO
package importers

// Importer defines the interface for importer plugins
type Importer interface {
	// Metadata associated with each Importer.
	Metadata() ImporterMetadata

	// Init will initialize each importer with a given config. This config will contain the Unmarhsalled config file specific to this plugin.
	// It is called during initialization of an importer plugin such as setting up network connections, file buffers etc.
	Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) error

	// Config returns the configuration options used to create an Importer. Initialized during Init.
	Config() plugins.PluginConfig

	// Close function is used for closing network connections, files, flushing buffers etc.
	Close() error

	// GetBlock, given any round number-rnd fetches the block at that round
	// It returns an object of type BlockData defined in data
	GetBlock(rnd uint64) (data.BlockData, error)
}
```

### Interface Description

#### Metadata
This function returns the metadata associated with an importer plugin that implements the above importer interface

#### Init
This function is used to initialize the importer plugin such as establishing network connections, file buffers, context etc.

#### Config
This function returns the configuration options used to create an Importer. Initialized during Init.

#### Close
This function is used to close/end resources used by the importer plugin, such as closing of open network connections, file buffers, context etc.

#### GetBlock
Every importer plugin implements GetBlock function that fetches a block at given round number and returns this data in a standardized format (data.BlockData).
