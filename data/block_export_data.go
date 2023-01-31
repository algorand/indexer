package data

import (
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
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
	NextDBRound() basics.Round
}

// BlockData is provided to the Exporter on each round.
type BlockData struct {

	// BlockHeader is the immutable header from the block
	BlockHeader bookkeeping.BlockHeader `json:"block,omitempty"`

	// Payset is the set of data the block is carrying--can be modified as it is processed
	Payset []transactions.SignedTxnInBlock `json:"payset,omitempty"`

	// Delta contains a list of account changes resulting from the block. Processor plugins may have modify this data.
	Delta *ledgercore.StateDelta `json:"delta,omitempty"`

	// Certificate contains voting data that certifies the block. The certificate is non deterministic, a node stops collecting votes once the voting threshold is reached.
	Certificate *agreement.Certificate `json:"cert,omitempty"`
}

// MakeBlockDataFromValidatedBlock makes BlockData from agreement.ValidatedBlock
func MakeBlockDataFromValidatedBlock(input ledgercore.ValidatedBlock) BlockData {
	blockData := BlockData{}
	blockData.UpdateFromValidatedBlock(input)
	return blockData
}

// UpdateFromValidatedBlock updates BlockData from ValidatedBlock input
func (blkData *BlockData) UpdateFromValidatedBlock(input ledgercore.ValidatedBlock) {
	blkData.BlockHeader = input.Block().BlockHeader
	blkData.Payset = input.Block().Payset
	delta := input.Delta()
	blkData.Delta = &delta
}

// UpdateFromEncodedBlockCertificate updates BlockData from EncodedBlockCert info
func (blkData *BlockData) UpdateFromEncodedBlockCertificate(input *rpcs.EncodedBlockCert) {
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
	blkData.UpdateFromEncodedBlockCertificate(input)
	return blkData
}

// ValidatedBlock returns a validated block from the BlockData object
func (blkData BlockData) ValidatedBlock() ledgercore.ValidatedBlock {
	tmpBlock := bookkeeping.Block{
		BlockHeader: blkData.BlockHeader,
		Payset:      blkData.Payset,
	}

	tmpDelta := ledgercore.StateDelta{}
	if blkData.Delta != nil {
		tmpDelta = *blkData.Delta
	}
	return ledgercore.MakeValidatedBlock(tmpBlock, tmpDelta)
}

// EncodedBlockCertificate returns an encoded block certificate from the BlockData object
func (blkData BlockData) EncodedBlockCertificate() rpcs.EncodedBlockCert {

	tmpBlock := bookkeeping.Block{
		BlockHeader: blkData.BlockHeader,
		Payset:      blkData.Payset,
	}

	tmpCert := agreement.Certificate{}
	if blkData.Certificate != nil {
		tmpCert = *blkData.Certificate
	}
	return rpcs.EncodedBlockCert{
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
