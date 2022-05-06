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
func MakeProcessor(ledger *ledger.Ledger, handler func(block *ledgercore.ValidatedBlock) error) processor.Processor {
	return &blockProcessor{ledger: ledger, nextRoundToProcess: 1, handler: handler}
}

// Process a raw algod block
func (processor *blockProcessor) Process(cert *rpcs.EncodedBlockCert) error {
	if processor.ledger == nil {
		return fmt.Errorf("Process() err: local ledger not initialized")
	}
	if cert == nil {
		return fmt.Errorf("Process() err: cannot process a nil block")
	}
	if uint64(cert.Block.Round()) != processor.nextRoundToProcess {
		return fmt.Errorf("Process() err: block has invalid round")
	}

	blkeval, err := processor.ledger.StartEvaluator(cert.Block.BlockHeader, len(cert.Block.Payset), 0)
	if err != nil {
		return fmt.Errorf("Process() block eval err: %w", err)
	}

	paysetgroups, err := cert.Block.DecodePaysetGroups()
	if err != nil {
		return fmt.Errorf("Process() decode payset groups err: %w", err)
	}

	for _, group := range paysetgroups {
		err = blkeval.TransactionGroup(group)
		if err != nil {
			return fmt.Errorf("Process() apply transaction group err: %w", err)
		}
	}

	//validated block
	vb, err := blkeval.GenerateBlock()
	if err != nil {
		return fmt.Errorf("Process() validated block err: %w", err)
	}
	if processor.handler != nil {
		err = processor.handler(vb)
		if err != nil {
			return fmt.Errorf("Process() handler err: %w", err)
		}
	}
	//	write to ledger
	err = processor.ledger.AddValidatedBlock(*vb, cert.Certificate)
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
