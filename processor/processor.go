package processor

import (
	"fmt"
	"path"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/rpcs"
)

type Processor interface {
	Process(cert *rpcs.EncodedBlockCert) error
	SetHandler(handler func(block *ledgercore.ValidatedBlock) error)
	NextRoundToProcess() uint64
	Init(path string, genesis *bookkeeping.Genesis, genesisBlock *bookkeeping.Block) error
}

type processorImpl struct {
	handler            func(block *ledgercore.ValidatedBlock) error
	nextRoundToProcess uint64
	ledger             *ledger.Ledger
}

// MakeProcessor creates a block processor
func MakeProcessor() Processor {
	return &processorImpl{}
}

// Init block processor. Create ledger if it wasn't initialized before
func (processor *processorImpl) Init(ledgerPath string, genesis *bookkeeping.Genesis, genesisBlock *bookkeeping.Block) error {
	logger := logging.NewLogger()

	accounts := make(map[basics.Address]basics.AccountData)
	for _, alloc := range genesis.Allocation {
		address, err := basics.UnmarshalChecksumAddress(alloc.Address)
		if err != nil {
			return fmt.Errorf("openLedger() decode address err: %w", err)
		}
		accounts[address] = alloc.State
	}
	initState := ledgercore.InitState{
		Block:       *genesisBlock,
		Accounts:    accounts,
		GenesisHash: genesisBlock.GenesisHash(),
	}
	ledger, err := ledger.OpenLedger(
		logger, path.Join(ledgerPath, "ledger"), false, initState, config.GetDefaultLocal())
	if err != nil {
		return fmt.Errorf("openLedger() open err: %w", err)
	}
	processor.ledger = ledger
	processor.nextRoundToProcess = 1
	return nil
}

// Process a raw algod block
func (processor *processorImpl) Process(cert *rpcs.EncodedBlockCert) error {
	if processor.ledger == nil {
		return fmt.Errorf("Process() err: local ledger not initialized")
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
	//	write to ledger
	err = processor.ledger.AddValidatedBlock(*vb, agreement.Certificate{})
	if err != nil {
		return fmt.Errorf("Process() add validated block err: %w", err)
	}
	if processor.handler != nil {
		err = processor.handler(vb)
		if err != nil {
			return fmt.Errorf("Process() handler err: %w", err)
		}
	}
	processor.nextRoundToProcess = uint64(cert.Block.Round()) + 1
	return nil
}

func (processor *processorImpl) SetHandler(handler func(block *ledgercore.ValidatedBlock) error) {
	processor.handler = handler
}
func (processor *processorImpl) NextRoundToProcess() uint64 {
	return processor.nextRoundToProcess
}
