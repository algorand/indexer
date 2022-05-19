package blockprocessor

import (
	"fmt"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/accounting"
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
	if handler != nil && ledger.Latest() == 0 {
		blk, err := ledger.Block(0)
		if err != nil {
			return nil, fmt.Errorf("MakeProcessor() err: %w", err)
		}
		vb := ledgercore.MakeValidatedBlock(blk, ledgercore.StateDelta{})
		err = handler(&vb)
		if err != nil {
			return nil, fmt.Errorf("MakeProcessor() handler err: %w", err)
		}
	}
	return &blockProcessor{ledger: ledger, nextRoundToProcess: uint64(ledger.Latest() + 1), handler: handler}, nil
}

// Process a raw algod block
func (proc *blockProcessor) Process(blockCert *rpcs.EncodedBlockCert) error {
	if blockCert == nil {
		return fmt.Errorf("Process(): cannot process a nil block")
	}
	if uint64(blockCert.Block.Round()) != proc.nextRoundToProcess {
		return fmt.Errorf("Process() invalid round blockCert.Block.Round(): %d proc.nextRoundToProcess: %d", blockCert.Block.Round(), proc.nextRoundToProcess)
	}

	proto, ok := config.Consensus[blockCert.Block.BlockHeader.CurrentProtocol]
	if !ok {
		return fmt.Errorf(
			"Process() cannot find proto version %s", blockCert.Block.BlockHeader.CurrentProtocol)
	}
	proto.EnableAssetCloseAmount = true

	ledgerForEval, err := processor.MakeLedgerForEvaluator(proc.ledger)
	if err != nil {
		return fmt.Errorf("Process() err: %w", err)
	}
	resources, _ := prepareEvalResources(&ledgerForEval, &blockCert.Block)
	if err != nil {
		panic(fmt.Errorf("Process() eval err: %w", err))
	}

	delta, _, err :=
		ledger.EvalForIndexer(ledgerForEval, &blockCert.Block, proto, resources)
	if err != nil {
		return fmt.Errorf("Process() eval err: %w", err)
	}
	// validated block
	vb := ledgercore.MakeValidatedBlock(blockCert.Block, delta)
	if err != nil {
		return fmt.Errorf("Process() block eval err: %w", err)
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
	proc.nextRoundToProcess = uint64(proc.ledger.Latest()) + 1
	return nil
}

func (proc *blockProcessor) SetHandler(handler func(block *ledgercore.ValidatedBlock) error) {
	proc.handler = handler
}

func (proc *blockProcessor) NextRoundToProcess() uint64 {
	return proc.nextRoundToProcess
}

// Preload all resources (account data, account resources, asset/app creators) for the
// evaluator.
func prepareEvalResources(l *processor.LedgerForEvaluator, block *bookkeeping.Block) (ledger.EvalForIndexerResources, error) {
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
func prepareCreators(l *processor.LedgerForEvaluator, payset transactions.Payset) (map[basics.AssetIndex]ledger.FoundAddress, map[basics.AppIndex]ledger.FoundAddress, error) {
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
func prepareAccountsResources(l *processor.LedgerForEvaluator, payset transactions.Payset, assetCreators map[basics.AssetIndex]ledger.FoundAddress, appCreators map[basics.AppIndex]ledger.FoundAddress) (map[basics.Address]*ledgercore.AccountData, map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource, error) {
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
