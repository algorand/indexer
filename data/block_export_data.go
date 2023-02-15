package data

import (
	"github.com/algorand/indexer/helpers"
	"github.com/algorand/indexer/types"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/rpcs"
)

// RoundProvider is the interface which all data types sent to Exporters should implement
type RoundProvider interface {
	Round() uint64
	Empty() bool
}

// InitProvider is the interface that can be used when initializing to get common algod related
// variables
type InitProvider interface {
	GetGenesis() *sdk.Genesis
	NextDBRound() sdk.Round
}

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

// MakeBlockDataFromValidatedBlock makes BlockData from agreement.ValidatedBlock
func MakeBlockDataFromValidatedBlock(input types.ValidatedBlock) BlockData {
	blockData := BlockData{}
	blockData.UpdateFromValidatedBlock(input)
	return blockData
}

// UpdateFromValidatedBlock updates BlockData from ValidatedBlock input
func (blkData *BlockData) UpdateFromValidatedBlock(input types.ValidatedBlock) {
	blkData.BlockHeader = input.Block.BlockHeader
	blkData.Payset = input.Block.Payset
	delta := input.Delta
	blkData.Delta = &delta
}

// UpdateFromEncodedBlockCertificate updates BlockData from EncodedBlockCert info
func (blkData *BlockData) UpdateFromEncodedBlockCertificate(input *types.EncodedBlockCert) {
	if input == nil {
		return
	}

	blkData.BlockHeader = input.Block.BlockHeader
	blkData.Payset = input.Block.Payset

	cert := input.Certificate
	blkData.Certificate = &cert
}

// MakeBlockDataFromEncodedBlockCertificate makes BlockData from rpcs.EncodedBlockCert
func MakeBlockDataFromEncodedBlockCertificate(input *rpcs.EncodedBlockCert) BlockData {
	blkData := BlockData{}
	iBlockCert, _ := helpers.UnonvertEncodedBlockCert(*input)
	blkData.UpdateFromEncodedBlockCertificate(&iBlockCert)
	return blkData
}

// ValidatedBlock returns a validated block from the BlockData object
func (blkData BlockData) ValidatedBlock() types.ValidatedBlock {
	tmpBlock := sdk.Block{
		BlockHeader: blkData.BlockHeader,
		Payset:      blkData.Payset,
	}

	tmpDelta := sdk.LedgerStateDelta{}
	if blkData.Delta != nil {
		tmpDelta = *blkData.Delta
	}
	vb := types.ValidatedBlock{
		Block: tmpBlock,
		Delta: tmpDelta,
	}
	return vb
}

// EncodedBlockCertificate returns an encoded block certificate from the BlockData object
func (blkData BlockData) EncodedBlockCertificate() types.EncodedBlockCert {

	tmpBlock := sdk.Block{
		BlockHeader: blkData.BlockHeader,
		Payset:      blkData.Payset,
	}

	tmpCert := make(map[string]interface{})
	if blkData.Certificate != nil {
		tmpCert = *blkData.Certificate
	}
	return types.EncodedBlockCert{
		Block:       tmpBlock,
		Certificate: tmpCert,
	}
}

// Round returns the round to which the BlockData corresponds
func (blkData BlockData) Round() uint64 {
	return uint64(blkData.BlockHeader.Round)
}

// Empty returns whether the Block contains Txns. Assumes the Block is never nil
func (blkData BlockData) Empty() bool {
	return len(blkData.Payset) == 0
}
