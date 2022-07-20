package importerplugin

import (
	"github.com/algorand/indexer/exporters"
)

// Importer defines the interface for importer plugins
type Importer interface {
	// GetBlock given any round number rnd fetches the block at that round
	// It returns an object of type BlockExportData defined in exporters plugin
	GetBlock(rnd uint64) (*exporters.BlockExportData, error)
}
