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
	GetLastProcessedRound() uint64
	Start(path string, genesis *bookkeeping.Genesis, genesisBlock *bookkeeping.Block) error
}

type ProcessorImpl struct {
	handler            func(block *ledgercore.ValidatedBlock) error
	lastProcessedRound uint64
	ledger             *ledger.Ledger
}

// Start block processor. Create ledger if it wasn't initialized before
func (processor *ProcessorImpl) Start(ledgerPath string, genesis *bookkeeping.Genesis, genesisBlock *bookkeeping.Block) error {
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
	return nil
}

// Process a raw algod block
func (processor *ProcessorImpl) Process(cert *rpcs.EncodedBlockCert) error {
	if processor.ledger == nil {
		return fmt.Errorf("Process() err: local ledger not initialized")
	}
	if cert.Block.Round() == basics.Round(0) {
		// Block 0 is special, we cannot run the evaluator on it
		err := processor.ledger.AddBlock(cert.Block, agreement.Certificate{})
		if err != nil {
			return fmt.Errorf("error adding round  %v to local ledger", err.Error())
		}
		processor.lastProcessedRound = uint64(cert.Block.Round())
	} else {
		if uint64(cert.Block.Round()) != processor.lastProcessedRound+1 {
			return fmt.Errorf("Process() err: block has invalid round")
		}
		blkeval, err := processor.ledger.StartEvaluator(cert.Block.BlockHeader, len(cert.Block.Payset), 0)
		//validated block
		vb, err := blkeval.GenerateBlock()
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
		processor.lastProcessedRound = uint64(cert.Block.Round())
	}
	return nil
}

func (processor *ProcessorImpl) SetHandler(handler func(block *ledgercore.ValidatedBlock) error) {
	processor.handler = handler
}
func (processor *ProcessorImpl) GetLastProcessedRound() uint64 {
	return processor.lastProcessedRound
}
