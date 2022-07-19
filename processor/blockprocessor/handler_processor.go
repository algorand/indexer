package blockprocessor

import (
	"fmt"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
	indexerledger "github.com/algorand/indexer/processor/eval"
	"github.com/sirupsen/logrus"
)

type blockProcessor struct {
	handler func(block *ledgercore.ValidatedBlock) error
	ledger  *ledger.Ledger
	logger  *logrus.Logger
}

func (proc *blockProcessor) AddGenesisBlock() error {
	if proc.handler != nil && uint64(proc.ledger.Latest()) == 0 {
		blk, err := proc.ledger.Block(0)
		if err != nil {
			return fmt.Errorf("addGenesisBlock() err: %w", err)
		}
		vb := ledgercore.MakeValidatedBlock(blk, ledgercore.StateDelta{})
		err = proc.handler(&vb)
		if err != nil {
			return fmt.Errorf("addGenesisBlock() handler err: %w", err)
		}
	}
	return nil
}

// Process a raw algod block
func (proc *blockProcessor) Process(blockCert *rpcs.EncodedBlockCert) error {

	if blockCert == nil {
		return fmt.Errorf("Process(): cannot process a nil block")
	}
	if uint64(blockCert.Block.Round()) != uint64(proc.ledger.Latest())+1 {
		return fmt.Errorf("Process() invalid round blockCert.Block.Round(): %d nextRoundToProcess: %d", blockCert.Block.Round(), uint64(proc.ledger.Latest())+1)
	}

	proto, ok := config.Consensus[blockCert.Block.BlockHeader.CurrentProtocol]
	if !ok {
		return fmt.Errorf(
			"Process() cannot find proto version %s", blockCert.Block.BlockHeader.CurrentProtocol)
	}
	protoChanged := !proto.EnableAssetCloseAmount
	proto.EnableAssetCloseAmount = true

	ledgerForEval := indexerledger.MakeLedgerForEvaluator(proc.ledger)

	resources, err := prepareEvalResources(&ledgerForEval, &blockCert.Block)
	if err != nil {
		proc.logger.Panicf("Process() resources err: %v", err)
	}

	delta, modifiedTxns, err :=
		ledger.EvalForIndexer(ledgerForEval, &blockCert.Block, proto, resources)
	if err != nil {
		return fmt.Errorf("Process() eval err: %w", err)
	}
	// validated block
	var vb ledgercore.ValidatedBlock
	vb = ledgercore.MakeValidatedBlock(blockCert.Block, delta)
	if protoChanged {
		block := bookkeeping.Block{
			BlockHeader: blockCert.Block.BlockHeader,
			Payset:      modifiedTxns,
		}
		vb = ledgercore.MakeValidatedBlock(block, delta)
	}

	// execute handler before writing to local ledger
	if proc.handler != nil {
		err = proc.handler(&vb)
		if err != nil {
			return fmt.Errorf("Process() handler err: %w", err)
		}
	}
	// write to ledger
	err = proc.ledger.AddValidatedBlock(vb, blockCert.Certificate)
	if err != nil {
		return fmt.Errorf("Process() add validated block err: %w", err)
	}
	// wait for commit to disk
	proc.ledger.WaitForCommit(blockCert.Block.Round())
	return nil
}

func (proc *blockProcessor) NextRoundToProcess() uint64 {
	return uint64(proc.ledger.Latest()) + 1
}
