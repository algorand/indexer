package importer

import (
	"fmt"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/protocol"
	"github.com/algorand/indexer/protocol/config"

	"github.com/algorand/go-algorand/ledger/ledgercore"
)

// Importer is used to import blocks into an idb.IndexerDb object.
type Importer interface {
	ImportBlock(vb *ledgercore.ValidatedBlock) error
}

type importerImpl struct {
	db idb.IndexerDb
}

// ImportBlock processes a block and adds it to the IndexerDb
func (imp *importerImpl) ImportBlock(vb *ledgercore.ValidatedBlock) error {
	block := vb.Block()

	_, ok := config.Consensus[protocol.ConsensusVersion(block.CurrentProtocol)]
	if !ok {
		return fmt.Errorf("protocol %s not found", block.CurrentProtocol)
	}
	return imp.db.AddBlock(vb)
}

// NewImporter creates a new importer object.
func NewImporter(db idb.IndexerDb) Importer {
	return &importerImpl{db: db}
}
