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
package importerplugin

// Importer defines the interface for importer plugins
type Importer interface {
	// GetBlock given any round number rnd fetches the block at that round
	// It returns an object of type BlockExportData defined in exporters plugin
	GetBlock(rnd uint64) (*exporters.BlockExportData, error)
}
```

### Interface Description

#### GetBlock
Every importer plugin effectively implements only one function that fetches a block at given round number and returns this data in a standardized format (exporters.BlockExportData).