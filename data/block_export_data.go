package data

import (
	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

// RoundProvider is the interface which all data types sent to Exporters should implement
type RoundProvider interface {
	Round() uint64
}

// BlockData is provided to the Exporter on each round.
type BlockData struct {
	// Block is the block data written to the blockchain.
	Block *bookkeeping.Block

	// Delta contains a list of account changes resulting from the block. Processor plugins may have modify this data.
	Delta *ledgercore.StateDelta

	// Certificate contains voting data that certifies the block. The certificate is non deterministic, a node stops collecting votes once the voting threshold is reached.
	Certificate *agreement.Certificate
}

// Round returns the round to which the BlockData corresponds
func (blkData BlockData) Round() uint64 {
	return uint64(blkData.Block.Round())
}
