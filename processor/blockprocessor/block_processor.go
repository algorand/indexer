package blockprocessor

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/processor"
	indexerledger "github.com/algorand/indexer/processor/eval"
	"github.com/algorand/indexer/util"
)

type blockProcessor struct {
	handler func(block *ledgercore.ValidatedBlock) error
	ledger  *ledger.Ledger
}

// MakeProcessorWithLedger creates a block processor with a given ledger
func MakeProcessorWithLedger(l *ledger.Ledger, handler func(block *ledgercore.ValidatedBlock) error) (processor.Processor, error) {
	if l == nil {
		return nil, fmt.Errorf("MakeProcessorWithLedger() err: local ledger not initialized")
	}
	err := addGenesisBlock(l, handler)
	if err != nil {
		return nil, fmt.Errorf("MakeProcessorWithLedger() err: %w", err)
	}
	return &blockProcessor{ledger: l, handler: handler}, nil
}

// MakeProcessorWithLedgerInit creates a block processor and initializes the ledger.
func MakeProcessorWithLedgerInit(ctx context.Context, logger *log.Logger, catchpoint string, genesis *bookkeeping.Genesis, nextDBRound uint64, opts idb.IndexerDbOptions, handler func(block *ledgercore.ValidatedBlock) error) (processor.Processor, error) {
	if nextDBRound > 0 {
		if catchpoint != "" {
			round, _, err := ledgercore.ParseCatchpointLabel(catchpoint)
			if err != nil {
				return &blockProcessor{}, fmt.Errorf("MakeProcessorWithCatchup() label err: %w", err)
			}
			if uint64(round) >= nextDBRound {
				return &blockProcessor{}, fmt.Errorf("invalid catchpoint: catchpoint round %d should not be ahead of target round %d", uint64(round), nextDBRound-1)
			} else {
				err = InitializeLedgerFastCatchup(ctx, logger, catchpoint, opts.IndexerDatadir, *genesis)
				if err != nil {
					return &blockProcessor{}, fmt.Errorf("MakeProcessorWithCatchup() fast catchup err: %w", err)
				}
			}
		}
		err := InitializeLedgerSimple(ctx, logger, nextDBRound-1, &opts)
		if err != nil {
			return &blockProcessor{}, fmt.Errorf("MakeProcessorWithCatchup() slow catchup err: %w", err)
		}
	}
	return MakeProcessor(logger, genesis, nextDBRound, opts.IndexerDatadir, handler)
}

// MakeProcessor creates a block processor
func MakeProcessor(logger *log.Logger, genesis *bookkeeping.Genesis, dbRound uint64, datadir string, handler func(block *ledgercore.ValidatedBlock) error) (processor.Processor, error) {
	l, err := util.MakeLedger(logger, false, genesis, datadir)
	if err != nil {
		return nil, fmt.Errorf("MakeProcessor() err: %w", err)
	}
	if uint64(l.Latest()) > dbRound {
		return nil, fmt.Errorf("MakeProcessor() err: the ledger cache is ahead of the required round and must be re-initialized")
	}
	return MakeProcessorWithLedger(l, handler)
}

func addGenesisBlock(l *ledger.Ledger, handler func(block *ledgercore.ValidatedBlock) error) error {
	if handler != nil && uint64(l.Latest()) == 0 {
		blk, err := l.Block(0)
		if err != nil {
			return fmt.Errorf("addGenesisBlock() err: %w", err)
		}
		vb := ledgercore.MakeValidatedBlock(blk, ledgercore.StateDelta{})
		err = handler(&vb)
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
		panic(fmt.Errorf("Process() resources err: %w", err))
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

func (proc *blockProcessor) SetHandler(handler func(block *ledgercore.ValidatedBlock) error) {
	proc.handler = handler
}

func (proc *blockProcessor) NextRoundToProcess() uint64 {
	return uint64(proc.ledger.Latest()) + 1
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
