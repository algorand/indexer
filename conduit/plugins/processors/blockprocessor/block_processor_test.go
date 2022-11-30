package blockprocessor_test

import (
	"context"
	"fmt"
	"testing"

	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/conduit/plugins/processors/blockprocessor"
	"github.com/algorand/indexer/util/test"
)

var noopHandler = func(block *ledgercore.ValidatedBlock) error {
	return nil
}

func TestProcess(t *testing.T) {
	logger, _ := test2.NewNullLogger()
	l, err := test.MakeTestLedger(logger)
	require.NoError(t, err)
	defer l.Close()
	genesisBlock, err := l.Block(basics.Round(0))
	assert.Nil(t, err)
	// create processor
	pr, _ := blockprocessor.MakeBlockProcessorWithLedger(logger, l, noopHandler)
	proc := blockprocessor.MakeBlockProcessorHandlerAdapter(&pr, noopHandler)
	prevHeader := genesisBlock.BlockHeader
	assert.Equal(t, basics.Round(0), l.Latest())
	// create a few rounds
	for i := 1; i <= 3; i++ {
		txn := test.MakePaymentTxn(0, uint64(i), 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
		block, err := test.MakeBlockForTxns(prevHeader, &txn)
		assert.Nil(t, err)
		rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
		err = proc(&rawBlock)
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
	logger, _ := test2.NewNullLogger()
	l, err := test.MakeTestLedger(logger)
	require.NoError(t, err)
	defer l.Close()
	// invalid processor
	pr, err := blockprocessor.MakeBlockProcessorWithLedger(logger, nil, nil)
	assert.Contains(t, err.Error(), "local ledger not initialized")
	pr, err = blockprocessor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := blockprocessor.MakeBlockProcessorHandlerAdapter(&pr, nil)
	assert.Nil(t, err)
	err = proc(nil)
	assert.Nil(t, err)

	genesisBlock, err := l.Block(basics.Round(0))
	assert.Nil(t, err)
	// incorrect round
	txn := test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA)
	block, err := test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	block.BlockHeader.Round = 10
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Contains(t, err.Error(), "invalid round blockCert.Block.Round()")

	// non-zero balance after close remainder to sender address
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, test.AccountA, test.AccountA)
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Contains(t, err.Error(), "ProcessBlockForIndexer() err")

	// stxn GenesisID not empty
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	block.Payset[0].Txn.GenesisID = "genesisID"
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Contains(t, err.Error(), "ProcessBlockForIndexer() err")

	// eval error: concensus protocol not supported
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	block.BlockHeader.CurrentProtocol = "testing"
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Contains(t, err.Error(), "cannot find proto version testing")

	// handler error, errors out if this is set to true
	throwError := true
	handler := func(vb *ledgercore.ValidatedBlock) error {
		if throwError {
			return fmt.Errorf("handler error")
		}
		return nil
	}
	_, err = blockprocessor.MakeBlockProcessorWithLedger(logger, l, handler)
	assert.Contains(t, err.Error(), "handler error")
	// We don't want it to throw an error when we create the ledger but after
	throwError = false
	pr, err = blockprocessor.MakeBlockProcessorWithLedger(logger, l, handler)
	proc = blockprocessor.MakeBlockProcessorHandlerAdapter(&pr, handler)
	assert.NotNil(t, pr)
	assert.NoError(t, err)
	// enable this so it will throw an error when we process the block
	throwError = true
	txn = test.MakePaymentTxn(0, 10, 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err = test.MakeBlockForTxns(genesisBlock.BlockHeader, &txn)
	assert.Nil(t, err)
	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Contains(t, err.Error(), "handler error")
}

// TestMakeProcessorWithLedgerInit_CatchpointErrors verifies that the catchpoint error handling works properly.
func TestMakeProcessorWithLedgerInit_CatchpointErrors(t *testing.T) {
	logger, _ := test2.NewNullLogger()
	var genesis bookkeeping.Genesis

	testCases := []struct {
		name       string
		catchpoint string
		round      uint64
		errMsg     string
	}{
		{
			name:       "invalid catchpoint string",
			catchpoint: "asdlgkjasldgkjsadg",
			round:      1,
			errMsg:     "catchpoint parsing failed",
		},
		{
			name:       "catchpoint too recent",
			catchpoint: "21890000#IQ4BQTCNVEDIBNRPNCKWRBQLJ7ILXIJBYKJHF67TLUOYRUGHW7ZA",
			round:      21889999,
			errMsg:     "invalid catchpoint: catchpoint round 21890000 should not be ahead of target round 21889998",
		},
		{
			name:       "get past catchpoint check",
			catchpoint: "21890000#IQ4BQTCNVEDIBNRPNCKWRBQLJ7ILXIJBYKJHF67TLUOYRUGHW7ZA",
			round:      21890001,
			errMsg:     "indexer data directory missing",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := blockprocessor.Config{Catchpoint: tc.catchpoint}
			_, err := blockprocessor.MakeBlockProcessorWithLedgerInit(
				context.Background(),
				logger,
				tc.round,
				&genesis,
				config,
				noopHandler)
			require.ErrorContains(t, err, tc.errMsg)
		})
	}
}
