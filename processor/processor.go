package processor

import (
	"github.com/algorand/go-algorand/rpcs"
)

// Processor is the block processor interface
type Processor interface {
	Process(cert *rpcs.EncodedBlockCert) error
	NextRoundToProcess() uint64
	AddGenesisBlock() error
}
