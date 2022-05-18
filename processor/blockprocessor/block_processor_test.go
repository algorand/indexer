package blockprocessor_test

import (
	"fmt"
	"log"
	"testing"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/rpcs"
	block_processor "github.com/algorand/indexer/processor/blockprocessor"
	"github.com/algorand/indexer/util"
	"github.com/algorand/indexer/util/test"
	"github.com/stretchr/testify/assert"
)

func TestProcess(t *testing.T) {
	l := makeTestLedger(t, "local_ledger")
	genesisBlock, err := l.Block(basics.Round(0))
	assert.Nil(t, err)
	// create processor
	handler := func(vb *ledgercore.ValidatedBlock) error {
		return nil
	}
	pr, _ := block_processor.MakeProcessor(l, handler)
	prevHeader := genesisBlock.BlockHeader

	// create a few rounds
	for i := 1; i <= 3; i++ {
		txn := test.MakePaymentTxn(0, uint64(i), 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{}, 0)
		block, err := test.MakeBlockForTxns(prevHeader, &txn)
		assert.Nil(t, err)
		rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
		err = pr.Process(&rawBlock)
		assert.Nil(t, err)
		// check round
		assert.Equal(t, basics.Round(i), l.Latest())
		assert.Equal(t, uint64(basics.Round(i+1)), pr.NextRoundToProcess())
		// check added block
		addedBlock, err := l.Block(l.Latest())
		assert.Nil(t, err)
		assert.NotNil(t, addedBlock)
		assert.Equal(t, 1, len(addedBlock.Payset))
		prevHeader = addedBlock.BlockHeader
	}
}

func TestFailedProcess(t *testing.T) {
	l := makeTestLedger(t, "local_ledger2")
	// invalid processor
	pr, err := block_processor.MakeProcessor(nil, nil)
	assert.Contains(t, err.Error(), "MakeProcessor(): local ledger not initialized")
	pr, err = block_processor.MakeProcessor(l, nil)
	assert.Nil(t, err)
	err = pr.Process(nil)
	assert.Contains(t, err.Error(), "Process(): cannot process a nil block")

	genesisBlock, err := l.Block(basics.Round(0))
	assert.Nil(t, err)
	// incorrect round
	txn := test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA, 0)
	block, err := test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	block.BlockHeader.Round = 10
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() invalid round blockCert.Block.Round()")

	// non-zero balance after close remainder to sender address
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA, 0)
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "ProcessBlockForIndexer() err")

	// stxn GenesisID not empty
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{}, 0)
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	block.Payset[0].Txn.GenesisID = "genesisID"
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "ProcessBlockForIndexer() err")

	// eval error: concensus protocol not supported
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{}, 0)
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	block.BlockHeader.CurrentProtocol = "testing"
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() block eval err")

	// handler error
	handler := func(vb *ledgercore.ValidatedBlock) error {
		return fmt.Errorf("handler error")
	}
	_, err = block_processor.MakeProcessor(l, handler)
	assert.Contains(t, err.Error(), "MakeProcessor() handler err")
	pr, _ = block_processor.MakeProcessor(l, nil)
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{}, 0)
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	pr.SetHandler(handler)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() handler err")
}

func makeTestLedger(t *testing.T, prefix string) *ledger.Ledger {
	// initialize local ledger
	genesis := test.MakeGenesis()
	genesisBlock := test.MakeGenesisBlock()
	initState, err := util.CreateInitState(&genesis, &genesisBlock)
	if err != nil {
		log.Panicf("test init err: %v", err)
	}
	logger := logging.NewLogger()
	l, err := ledger.OpenLedger(logger, prefix, true, initState, config.GetDefaultLocal())
	if err != nil {
		log.Panicf("test init err: %v", err)
	}
	return l
}
