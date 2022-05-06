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
	"github.com/algorand/indexer/util/test"
	"github.com/stretchr/testify/assert"
)

func TestProcess(t *testing.T) {
	//initialize local ledger
	genesis := test.MakeGenesis()
	genesisBlock := test.MakeGenesisBlock()
	initState, err := test.CreateInitState(&genesis, &genesisBlock)
	if err != nil {
		log.Panicf("test init err: %v", err)
	}
	logger := logging.NewLogger()
	l, err := ledger.OpenLedger(logger, "local_ledger", true, initState, config.GetDefaultLocal())
	if err != nil {
		log.Panicf("test init err: %v", err)
	}
	//create processor
	handler := func(vb *ledgercore.ValidatedBlock) error {
		return nil
	}
	pr := block_processor.MakeProcessor(l, handler)
	prevHeader := genesisBlock.BlockHeader

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
	//initialize local ledger
	genesis := test.MakeGenesis()
	genesisBlock := test.MakeGenesisBlock()
	initState, err := test.CreateInitState(&genesis, &genesisBlock)
	if err != nil {
		log.Panicf("test init err: %v", err)
	}
	logger := logging.NewLogger()
	l, err := ledger.OpenLedger(logger, "local_ledger2", true, initState, config.GetDefaultLocal())
	if err != nil {
		log.Panicf("test init err: %v", err)
	}
	// invalid processor
	pr := block_processor.MakeProcessor(nil, nil)
	err = pr.Process(&rpcs.EncodedBlockCert{})
	assert.Contains(t, err.Error(), "Process() err: local ledger not initialized")
	pr = block_processor.MakeProcessor(l, nil)
	err = pr.Process(nil)
	assert.Contains(t, err.Error(), "Process() err: cannot process a nil block")

	// incorrect round
	txn := test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA)
	block, err := test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	block.BlockHeader.Round = 10
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() err: block has invalid round")

	//	non-zero balance after close remainder to sender address
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA)
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() apply transaction group")

	//	stxn GenesisID not empty
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	block.Payset[0].Txn.GenesisID = "genesisID"
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() decode payset groups err")

	//	eval error: concensus protocol not supported
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
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
	pr = block_processor.MakeProcessor(l, handler)
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.Contains(t, err.Error(), "Process() handler err")
}
