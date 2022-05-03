package processor

import (
	"os"
	"testing"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/util/test"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	genesis := test.MakeGenesis()
	genesisBlock := test.MakeGenesisBlock()
	dir := "/tmp/ledger"
	err := os.Mkdir(dir, 0755)
	defer os.RemoveAll(dir)
	assert.Nil(t, err)

	processor := processorImpl{}
	err = processor.Init(dir, &genesis, &genesisBlock)
	assert.Nil(t, err)
	assert.NotNil(t, processor.ledger)
}

func TestProcess(t *testing.T) {
	processor := processorImpl{}
	err := processor.Process(&rpcs.EncodedBlockCert{})
	assert.Contains(t, err.Error(), "local ledger not initialized")

	//initialize local ledger
	genesis := test.MakeGenesis()
	genesisBlock := test.MakeGenesisBlock()
	dir := "/tmp/ledger"
	err = os.Mkdir(dir, 0755)
	assert.Nil(t, err)
	err = processor.Init(dir, &genesis, &genesisBlock)
	defer os.RemoveAll(dir)
	assert.Nil(t, err)
	// create a few rounds
	for i := 0; i < 3; i++ {
		prevHeader, err := processor.ledger.BlockHdr(processor.ledger.Latest())
		assert.Nil(t, err)

		txn := test.MakePaymentTxn(0, uint64(i+1), 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
		block, err := test.MakeBlockForTxns(prevHeader, &txn)
		assert.Nil(t, err)
		block.BlockHeader.Round = basics.Round(i + 1)
		rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
		err = processor.Process(&rawBlock)
		assert.Nil(t, err)
		// check round
		assert.Equal(t, processor.ledger.Latest(), basics.Round(i+1))
		// check added block
		addedBlock, err := processor.ledger.Block(basics.Round(i + 1))
		assert.Nil(t, err)
		assert.NotNil(t, addedBlock)
		assert.Equal(t, len(addedBlock.Payset), 1)

	}
	// incorrect round
	txns := test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA)
	block, err := test.MakeBlockForTxns(genesisBlock.BlockHeader, &txns)
	block.BlockHeader.Round = 10
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = processor.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() err: block has invalid round")
}
