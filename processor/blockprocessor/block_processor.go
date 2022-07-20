package blockprocessor

import (
	"context"
	"fmt"
	"github.com/algorand/indexer/exporters"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/processor"
	indexerledger "github.com/algorand/indexer/processor/eval"
	"github.com/algorand/indexer/util"
)

func MakeProcessorWithLedger(logger *log.Logger, l *ledger.Ledger, genericHandler interface{}) (processor.Processor, error) {
	if l == nil {
		return nil, fmt.Errorf("MakeProcessorWithLedger() err: local ledger not initialized")
	}
	var proc processor.Processor
	if exp, ok := genericHandler.(exporters.Exporter); ok {
		proc = &ledgerExporterProcessor{logger: logger, ledger: l, exporter: exp}
	} else if handler, ok := genericHandler.(func(block *ledgercore.ValidatedBlock) error); ok {
		proc = &blockProcessor{logger: logger, ledger: l, handler: handler}
	} else if genericHandler == nil {
		proc = &blockProcessor{logger: logger, ledger: l, handler: handler}
	} else {
		return nil, fmt.Errorf("MakeProcessorWithLedger was unable to determine the type of block handler: %v", genericHandler)
	}
	err := proc.AddGenesisBlock()
	if err != nil {
		return nil, fmt.Errorf("MakeProcessorWithLedger() err: %w", err)
	}
	return proc, nil
}

// MakeProcessorWithLedgerInit creates a block processor and initializes the ledger.
func MakeProcessorWithLedgerInit(ctx context.Context, logger *log.Logger, catchpoint string, genesis *bookkeeping.Genesis, nextDBRound uint64, opts idb.IndexerDbOptions, genericHandler interface{}) (processor.Processor, error) {
	err := InitializeLedger(ctx, logger, catchpoint, nextDBRound, *genesis, &opts)
	if err != nil {
		return nil, fmt.Errorf("MakeProcessorWithLedgerInit() err: %w", err)
	}
	return MakeProcessor(logger, genesis, nextDBRound, opts.IndexerDatadir, genericHandler)
}

// MakeProcessor creates a block processor
func MakeProcessor(logger *log.Logger, genesis *bookkeeping.Genesis, dbRound uint64, datadir string, genericHandler interface{}) (processor.Processor, error) {
	l, err := util.MakeLedger(logger, false, genesis, datadir)
	if err != nil {
		return nil, fmt.Errorf("MakeProcessor() err: %w", err)
	}
	if uint64(l.Latest()) > dbRound {
		logger.Fatalf("ledger round: %v, db round: %v", l.Latest(), dbRound)
		return nil, fmt.Errorf("MakeProcessor() err: the ledger cache is ahead of the required round and must be re-initialized")
	}
	return MakeProcessorWithLedger(logger, l, genericHandler)
}

// Preload all resources (account data, account resources, asset/app creators) for the
// evaluator.
func prepareEvalResources(l *indexerledger.LedgerForEvaluator, block *bookkeeping.Block) (ledger.EvalForIndexerResources, error) {
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
func prepareCreators(l *indexerledger.LedgerForEvaluator, payset transactions.Payset) (map[basics.AssetIndex]ledger.FoundAddress, map[basics.AppIndex]ledger.FoundAddress, error) {
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
func prepareAccountsResources(l *indexerledger.LedgerForEvaluator, payset transactions.Payset, assetCreators map[basics.AssetIndex]ledger.FoundAddress, appCreators map[basics.AppIndex]ledger.FoundAddress) (map[basics.Address]*ledgercore.AccountData, map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource, error) {
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
