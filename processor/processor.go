package processor

import (
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
)

// Processor is the block processor interface
type Processor interface {
	Process(cert *rpcs.EncodedBlockCert) error
	SetHandler(handler func(block *ledgercore.ValidatedBlock) error)
	NextRoundToProcess() uint64
}
