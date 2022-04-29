package processor

import (
	"crypto/rand"
	"os"
	"testing"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/util/test"
	"github.com/stretchr/testify/assert"
)

func TestStartProcessor(t *testing.T) {
	genesis := test.MakeGenesis()
	genesisBlock := test.MakeGenesisBlock()
	dir := "/tmp/ledger"
	err := os.Mkdir(dir, 0755)
	defer os.RemoveAll(dir)
	assert.Nil(t, err)

	processor := ProcessorImpl{}
	err = processor.Start(dir, &genesis, &genesisBlock)
	assert.Nil(t, err)
	assert.NotNil(t, processor.ledger)
	files, err := os.ReadDir(dir)
	assert.Nil(t, err)
	assert.Greater(t, len(files), 0)
}

func TestProcess(t *testing.T) {
	processor := ProcessorImpl{}
	err := processor.Process(&rpcs.EncodedBlockCert{})
	assert.Contains(t, err.Error(), "local ledger not initialized")
	//initialize local ledger
	genesis := test.MakeGenesis()
	genesisBlock := test.MakeGenesisBlock()
	dir := "/tmp/ledger"
	err = os.Mkdir(dir, 0755)
	assert.Nil(t, err)
	err = processor.Start(dir, &genesis, &genesisBlock)
	defer os.RemoveAll(dir)
	assert.Nil(t, err)
	// add a block
	var addr basics.Address
	_, err = rand.Read(addr[:])
	assert.Nil(t, err)
	txns := test.MakePaymentTxn(0, 10, 0, 1, 1, 0, addr, addr, addr, addr)
	block, err := test.MakeBlockForTxns(genesisBlock.BlockHeader, &txns)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = processor.Process(&rawBlock)
	assert.Nil(t, err)
}
