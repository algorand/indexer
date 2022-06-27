package blockprocessor_test

import (
	"fmt"
	"testing"

	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
	block_processor "github.com/algorand/indexer/processor/blockprocessor"
	"github.com/algorand/indexer/util/test"
	"github.com/stretchr/testify/assert"
)

func TestProcess(t *testing.T) {
	log, _ := test2.NewNullLogger()
	l, err := test.MakeTestLedger(log)
	require.NoError(t, err)
	defer l.Close()
	genesisBlock, err := l.Block(basics.Round(0))
	assert.Nil(t, err)
	// create processor
	handler := func(vb *ledgercore.ValidatedBlock) error {
		return nil
	}
	pr, _ := block_processor.MakeProcessorWithLedger(l, handler)
	prevHeader := genesisBlock.BlockHeader
	assert.Equal(t, basics.Round(0), l.Latest())
	// create a few rounds
	for i := 1; i <= 3; i++ {
		txn := test.MakePaymentTxn(0, uint64(i), 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
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
	log, _ := test2.NewNullLogger()
	l, err := test.MakeTestLedger(log)
	require.NoError(t, err)
	defer l.Close()
	// invalid processor
	_, err := block_processor.MakeProcessorWithLedger(nil, nil)
	assert.Contains(t, err.Error(), "MakeProcessorWithLedger() err: local ledger not initialized")
	pr, err := block_processor.MakeProcessorWithLedger(l, nil)
	assert.Nil(t, err)
	err = pr.Process(nil)
	assert.Contains(t, err.Error(), "Process(): cannot process a nil block")

	genesisBlock, err := l.Block(basics.Round(0))
	assert.Nil(t, err)
	// incorrect round
	txn := test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA)
	block, err := test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	block.BlockHeader.Round = 10
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() invalid round blockCert.Block.Round()")

	// non-zero balance after close remainder to sender address
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA)
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "ProcessBlockForIndexer() err")

	// stxn GenesisID not empty
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	block.Payset[0].Txn.GenesisID = "genesisID"
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "ProcessBlockForIndexer() err")

	// eval error: concensus protocol not supported
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	block.BlockHeader.CurrentProtocol = "testing"
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() cannot find proto version testing")

	// handler error
	handler := func(vb *ledgercore.ValidatedBlock) error {
		return fmt.Errorf("handler error")
	}
	_, err = block_processor.MakeProcessorWithLedger(l, handler)
	assert.Contains(t, err.Error(), "handler error")
	pr, _ = block_processor.MakeProcessorWithLedger(l, nil)
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	pr.SetHandler(handler)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() handler err")
}
