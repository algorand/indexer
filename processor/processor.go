package processor

import (
	"fmt"
	"path"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/accounting"
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
	//open ledger
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
		panic(fmt.Errorf("Process() err: local ledger not initialized"))
	}
	if uint64(cert.Block.Round()) != processor.lastProcessedRound+1 {
		return fmt.Errorf("Process() err: block has invalid round")
	}
	if cert.Block.Round() == basics.Round(0) {
		// Block 0 is special, we cannot run the evaluator on it
		err := processor.ledger.AddBlock(cert.Block, agreement.Certificate{})
		if err != nil {
			return fmt.Errorf("error adding round  %v to local ledger", err.Error())
		}
		processor.lastProcessedRound = uint64(cert.Block.Round())
	} else {
		proto, ok := config.Consensus[cert.Block.BlockHeader.CurrentProtocol]
		if !ok {
			return fmt.Errorf(
				"Process() cannot find proto version %s", cert.Block.BlockHeader.CurrentProtocol)
		}
		proto.EnableAssetCloseAmount = true

		ledgerForEval, err := MakeLedgerForEvaluator(processor.ledger)
		if err != nil {
			return fmt.Errorf("Process() err: %w", err)
		}
		defer ledgerForEval.Close()
		resources, _ := prepareEvalResources(&ledgerForEval, &cert.Block)
		if err != nil {
			panic(fmt.Errorf("Process() eval err: %w", err))
		}

		delta, _, err :=
			ledger.EvalForIndexer(ledgerForEval, &cert.Block, proto, resources)
		if err != nil {
			return fmt.Errorf("Process() eval err: %w", err)
		}
		//validated block
		vb := ledgercore.MakeValidatedBlock(cert.Block, delta)
		//	write to ledger
		err = processor.ledger.AddValidatedBlock(vb, agreement.Certificate{})
		if err != nil {
			return fmt.Errorf("Process() add validated block err: %w", err)
		}
		if processor.handler != nil {
			err = processor.handler(&vb)
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

// Preload all resources (account data, account resources, asset/app creators) for the
// evaluator.
func prepareEvalResources(l *LedgerForEvaluator, block *bookkeeping.Block) (ledger.EvalForIndexerResources, error) {
	assetCreators, appCreators, err := prepareCreators(l, block.Payset)
	if err != nil {
		return ledger.EvalForIndexerResources{},
			fmt.Errorf("prepareEvalResources() err: %w", err)
	}

	res := ledger.EvalForIndexerResources{
		Accounts:  nil,
		Resources: nil,
		Creators:  make(map[ledger.Creatable]ledger.FoundAddress),
	}

	for index, foundAddress := range assetCreators {
		creatable := ledger.Creatable{
			Index: basics.CreatableIndex(index),
			Type:  basics.AssetCreatable,
		}
		res.Creators[creatable] = foundAddress
	}
	for index, foundAddress := range appCreators {
		creatable := ledger.Creatable{
			Index: basics.CreatableIndex(index),
			Type:  basics.AppCreatable,
		}
		res.Creators[creatable] = foundAddress
	}

	res.Accounts, res.Resources, err = prepareAccountsResources(l, block.Payset, assetCreators, appCreators)
	if err != nil {
		return ledger.EvalForIndexerResources{},
			fmt.Errorf("prepareEvalResources() err: %w", err)
	}

	return res, nil
}

// Preload asset and app creators.
func prepareCreators(l *LedgerForEvaluator, payset transactions.Payset) (map[basics.AssetIndex]ledger.FoundAddress, map[basics.AppIndex]ledger.FoundAddress, error) {
	assetsReq, appsReq := accounting.MakePreloadCreatorsRequest(payset)

	assets, err := l.GetAssetCreator(assetsReq)
	if err != nil {
		return nil, nil, fmt.Errorf("prepareCreators() err: %w", err)
	}
	apps, err := l.GetAppCreator(appsReq)
	if err != nil {
		return nil, nil, fmt.Errorf("prepareCreators() err: %w", err)
	}

	return assets, apps, nil
}

// Preload account data and account resources.
func prepareAccountsResources(l *LedgerForEvaluator, payset transactions.Payset, assetCreators map[basics.AssetIndex]ledger.FoundAddress, appCreators map[basics.AppIndex]ledger.FoundAddress) (map[basics.Address]*ledgercore.AccountData, map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource, error) {
	addressesReq, resourcesReq :=
		accounting.MakePreloadAccountsResourcesRequest(payset, assetCreators, appCreators)

	accounts, err := l.LookupWithoutRewards(addressesReq)
	if err != nil {
		return nil, nil, fmt.Errorf("prepareAccountsResources() err: %w", err)
	}
	resources, err := l.LookupResources(resourcesReq)
	if err != nil {
		return nil, nil, fmt.Errorf("prepareAccountsResources() err: %w", err)
	}

	return accounts, resources, nil
}
