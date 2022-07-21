package blockprocessor

import (
	"fmt"
	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/exporters/noop"
	"github.com/algorand/indexer/processor"
	"github.com/algorand/indexer/util/test"
	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var logger, _ = test2.NewNullLogger()
var nc = noop.Constructor{}

type errExp struct {
	exporters.Exporter
}

func (exp *errExp) Receive(_ data.BlockData) error {
	return fmt.Errorf("foobar")
}

func processTxnForRound(t *testing.T, round uint64, prevHeader bookkeeping.BlockHeader, pr processor.Processor) {
	txn := test.MakePaymentTxn(0, uint64(round), 0, 1, 1, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err := test.MakeBlockForTxns(prevHeader, &txn)
	assert.NoError(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = pr.Process(&rawBlock)
	assert.NoError(t, err)
}

func TestAddGenesisBlockNoop(t *testing.T) {
	proc := &ledgerExporterProcessor{
		exporter: nil,
		ledger:   nil,
		logger:   nil,
	}
	assert.NoError(t, proc.AddGenesisBlock())
}

func TestAddGenesisBlockSuccess(t *testing.T) {
	exp := nc.New()
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: exp,
		ledger:   l,
		logger:   logger,
	}

	err = proc.AddGenesisBlock()
	assert.NoError(t, err)
}

func TestAddGenesisBlockExporterFailure(t *testing.T) {
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: &errExp{},
		ledger:   l,
		logger:   logger,
	}

	err = proc.AddGenesisBlock()
	assert.Errorf(t, err, "foobar")
}

func TestNextRoundToProcess(t *testing.T) {
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: nc.New(),
		ledger:   l,
		logger:   logger,
	}

	assert.Equal(t, uint64(1), proc.NextRoundToProcess())
}

func TestProcessRound(t *testing.T) {
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: nc.New(),
		ledger:   l,
		logger:   logger,
	}

	genesisBlock, err := l.Block(basics.Round(0))
	assert.NoError(t, err)
	prevHeader := genesisBlock.BlockHeader
	processTxnForRound(t, uint64(1), prevHeader, proc)
	assert.Equal(t, uint64(2), proc.NextRoundToProcess())
}

func TestProcessNilBlock(t *testing.T) {
	proc := &ledgerExporterProcessor{
		exporter: nc.New(),
		ledger:   nil,
		logger:   logger,
	}
	err := proc.Process(nil)
	assert.Errorf(t, err, "Process(): cannot process a nil block")
}

func TestProcessInvalidRound(t *testing.T) {
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: nc.New(),
		ledger:   l,
		logger:   logger,
	}

	rawBlock := rpcs.EncodedBlockCert{Block: bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 2,
		},
	}, Certificate: agreement.Certificate{}}
	err = proc.Process(&rawBlock)
	assert.Errorf(t, err, "Process() invalid round blockCert.Block.Round(): %d nextRoundToProcess: %d", 2, 1)
}

func TestProcessInvalidConsensus(t *testing.T) {
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: nc.New(),
		ledger:   l,
		logger:   logger,
	}

	rawBlock := rpcs.EncodedBlockCert{Block: bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 1,
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: protocol.ConsensusVersion("foobar_not_real"),
			},
		},
	}, Certificate: agreement.Certificate{}}
	err = proc.Process(&rawBlock)
	assert.Errorf(t, err, "Process() cannot find proto version %s", "foobar_not_real")
}

func TestProcessEvalAssetDoesNotExistError(t *testing.T) {
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: nc.New(),
		ledger:   l,
		logger:   logger,
	}
	genesisBlock, err := l.Block(basics.Round(0))
	assert.NoError(t, err)
	prevHeader := genesisBlock.BlockHeader
	addr, _ := basics.UnmarshalChecksumAddress("J5YDZLPOHWB5O6MVRHNFGY4JXIQAYYM6NUJWPBSYBBIXH5ENQ4Z5LTJELU")
	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				AssetConfigTxnFields: transactions.AssetConfigTxnFields{
					ConfigAsset: 99999,
				},
				Header: transactions.Header{
					Sender:      addr,
					GenesisHash: test.GenesisHash,
				},
				Type: protocol.AssetConfigTx,
			},
		},
		ApplyData: transactions.ApplyData{
			ConfigAsset: basics.AssetIndex(99999999),
		},
	}
	block, err := test.MakeBlockForTxns(prevHeader, &txn)
	assert.NoError(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}

	err = proc.Process(&rawBlock)
	assert.Errorf(t, err, "Process() eval err: ProcessBlockForIndexer() err: transaction BXIPBW544ZQ7YHBQGR3VAFKJFQZ6W67YQIDABJZU3EB4OCK376LA: asset 99999 does not exist or has been deleted")
}

func TestProcessProtoChanged(t *testing.T) {
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: nc.New(),
		ledger:   l,
		logger:   logger,
	}

	rawBlock := rpcs.EncodedBlockCert{Block: bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			GenesisHash: test.GenesisHash,
			Round:       1,
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: protocol.ConsensusV24,
			},
		},
		Payset: []transactions.SignedTxnInBlock{},
	}, Certificate: agreement.Certificate{}}
	err = proc.Process(&rawBlock)
	assert.NoError(t, err)
}

func TestProcessExporterError(t *testing.T) {
	l, err := test.MakeTestLedger(logger)
	defer l.Close()
	require.NoError(t, err)
	proc := &ledgerExporterProcessor{
		exporter: &errExp{},
		ledger:   l,
		logger:   logger,
	}

	rawBlock := rpcs.EncodedBlockCert{Block: bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			GenesisHash: test.GenesisHash,
			Round:       1,
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: protocol.ConsensusV24,
			},
		},
		Payset: []transactions.SignedTxnInBlock{},
	}, Certificate: agreement.Certificate{}}
	err = proc.Process(&rawBlock)
	assert.Errorf(t, err, "Process() exporter err: foobar")
}
