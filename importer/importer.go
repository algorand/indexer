package importer

import (
	"fmt"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/idb"
)

// Importer is used to import blocks into an idb.IndexerDb object.
type Importer interface {
	ImportBlock(blockContainer *rpcs.EncodedBlockCert) error
	ImportValidatedBlock(vb *ledgercore.ValidatedBlock) error
}

type importerImpl struct {
	db idb.IndexerDb
}

// ImportBlock processes a block and adds it to the IndexerDb
func (imp *importerImpl) ImportBlock(blockContainer *rpcs.EncodedBlockCert) error {
	block := &blockContainer.Block

	_, ok := config.Consensus[block.CurrentProtocol]
	if !ok {
		return fmt.Errorf("protocol %s not found", block.CurrentProtocol)
	}
	return imp.db.AddBlock(&blockContainer.Block)
}

// ImportValidatedBlock processes a validated block and adds it to the IndexerDb
func (imp *importerImpl) ImportValidatedBlock(vb *ledgercore.ValidatedBlock) error {
	block := vb.Block()

	_, ok := config.Consensus[block.CurrentProtocol]
	if !ok {
		return fmt.Errorf("protocol %s not found", block.CurrentProtocol)
	}
	return imp.db.AddBlock(&block)
}

// NewImporter creates a new importer object.
func NewImporter(db idb.IndexerDb) Importer {
	return &importerImpl{db: db}
}
