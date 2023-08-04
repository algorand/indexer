package types

import (
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// orignal source: https://github.com/algorand/conduit/data/block_export_data.go

// BlockData is provided to the Exporter on each round.
type BlockData struct {

	// BlockHeader is the immutable header from the block
	BlockHeader sdk.BlockHeader `json:"block,omitempty"`

	// Payset is the set of data the block is carrying--can be modified as it is processed
	Payset []sdk.SignedTxnInBlock `json:"payset,omitempty"`

	// Delta contains a list of account changes resulting from the block. Processor plugins may have modify this data.
	Delta *sdk.LedgerStateDelta `json:"delta,omitempty"`

	// Certificate contains voting data that certifies the block. The certificate is non deterministic, a node stops collecting votes once the voting threshold is reached.
	Certificate *map[string]interface{} `json:"cert,omitempty"`
}

// Round returns the round to which the BlockData corresponds
func (blkData BlockData) Round() uint64 {
	return uint64(blkData.BlockHeader.Round)
}

// Empty returns whether the Block contains Txns. Assumes the Block is never nil
func (blkData BlockData) Empty() bool {
	return len(blkData.Payset) == 0
}
