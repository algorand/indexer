package blockprocessor

import (
	"fmt"

	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/processor"
)

type blockProcessor struct {
	handler            func(block *ledgercore.ValidatedBlock) error
	nextRoundToProcess uint64
	ledger             *ledger.Ledger
}

// MakeProcessor creates a block processor
func MakeProcessor(ledger *ledger.Ledger, handler func(block *ledgercore.ValidatedBlock) error) (processor.Processor, error) {
	if ledger == nil {
		return nil, fmt.Errorf("MakeProcessor(): local ledger not initialized")
	}
	return &blockProcessor{ledger: ledger, nextRoundToProcess: uint64(ledger.Latest() + 1), handler: handler}, nil
}

// Process a raw algod block
func (processor *blockProcessor) Process(blockCert *rpcs.EncodedBlockCert) error {
	if blockCert == nil {
		return fmt.Errorf("Process(): cannot process a nil block")
	}
	if uint64(blockCert.Block.Round()) != processor.nextRoundToProcess {
		return fmt.Errorf("Process() invalid round blockCert.Block.Round(): %d processor.nextRoundToProcess: %d", blockCert.Block.Round(), processor.nextRoundToProcess)
	}

	blkeval, err := processor.ledger.StartEvaluator(blockCert.Block.BlockHeader, len(blockCert.Block.Payset), 0)
	if err != nil {
		return fmt.Errorf("Process() block eval err: %w", err)
	}

	paysetgroups, err := blockCert.Block.DecodePaysetGroups()
	if err != nil {
		return fmt.Errorf("Process() decode payset groups err: %w", err)
	}

	for _, group := range paysetgroups {
		err = blkeval.TransactionGroup(group)
		if err != nil {
			return fmt.Errorf("Process() apply transaction group err: %w", err)
		}
	}

	// validated block
	vb, err := blkeval.GenerateBlock()
	if err != nil {
		return fmt.Errorf("Process() validated block err: %w", err)
	}
	// execute handler before writing to local ledger
	if processor.handler != nil {
		err = processor.handler(vb)
		if err != nil {
			return fmt.Errorf("Process() handler err: %w", err)
		}
	}
	// write to ledger
	err = processor.ledger.AddValidatedBlock(*vb, blockCert.Certificate)
	if err != nil {
		return fmt.Errorf("Process() add validated block err: %w", err)
	}
	processor.nextRoundToProcess = uint64(processor.ledger.Latest()) + 1
	return nil
}

func (processor *blockProcessor) SetHandler(handler func(block *ledgercore.ValidatedBlock) error) {
	processor.handler = handler
}

func (processor *blockProcessor) NextRoundToProcess() uint64 {
	return processor.nextRoundToProcess
}
