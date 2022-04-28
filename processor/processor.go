package processor

import (
	"fmt"
	"path"
	"sync"

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

var l *ledger.Ledger

type Processor interface {
	Process(cert *rpcs.EncodedBlockCert)
	SetHandler(handler func(block *ledgercore.ValidatedBlock) error)
	GetLastProcessedRound() uint64
}

type processorImpl struct {
	handler            func(block *ledgercore.ValidatedBlock) error
	lastProcessedRound uint64
}

func (processor *processorImpl) init() {
	//open ledger
}

func (processor *processorImpl) Process(cert *rpcs.EncodedBlockCert) {
	var modifiedAccounts map[basics.Address]struct{}
	var modifiedResources map[basics.Address]map[ledger.Creatable]struct{}
	var err0 error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		modifiedAccounts, modifiedResources, err0 = getModifiedState(l, &cert.Block)
		wg.Done()
	}()

	if cert.Block.Round() == basics.Round(0) {
		// Block 0 is special, we cannot run the evaluator on it
		l.AddBlock(cert.Block, agreement.Certificate{})
	} else {
		proto, _ := config.Consensus[cert.Block.BlockHeader.CurrentProtocol]
		//if !ok {
		//	return fmt.Errorf(
		//		"process() cannot find proto version %s", cert.Block.BlockHeader.CurrentProtocol)
		//}
		proto.EnableAssetCloseAmount = true

		ledgerForEval, _ := MakeLedgerForEvaluator(l, cert.Block.Round()-1)
		//if err != nil {
		//	return fmt.Errorf("AddBlock() err: %w", err)
		//}
		defer ledgerForEval.Close()
		resources, _ := prepareEvalResources(&ledgerForEval, &cert.Block)
		//if err != nil {
		//	return fmt.Errorf("AddBlock() eval err: %w", err)
		//}

		delta, _, _ :=
			ledger.EvalForIndexer(ledgerForEval, &cert.Block, proto, resources)
		//if err != nil {
		//	return fmt.Errorf("AddBlock() eval err: %w", err)
		//}
		//validated delta
		vb := ledgercore.MakeValidatedBlock(cert.Block, delta)
		//	write to ledger
		l.AddValidatedBlock(vb, agreement.Certificate{})
	}
	processor.lastProcessedRound = processor.lastProcessedRound + 1

}

func (processor *processorImpl) SetHandler(handler func(block *ledgercore.ValidatedBlock) error) {
	processor.handler = handler
}
func (processor *processorImpl) GetLastProcessedRound() uint64 {
	return processor.lastProcessedRound
}

func openLedger(ledgerPath string, genesis *bookkeeping.Genesis, genesisBlock *bookkeeping.Block) (*ledger.Ledger, error) {
	logger := logging.NewLogger()

	accounts := make(map[basics.Address]basics.AccountData)
	for _, alloc := range genesis.Allocation {
		address, err := basics.UnmarshalChecksumAddress(alloc.Address)
		if err != nil {
			return nil, fmt.Errorf("openLedger() decode address err: %w", err)
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
		return nil, fmt.Errorf("openLedger() open err: %w", err)
	}

	return ledger, nil
}

func getModifiedState(l *ledger.Ledger, block *bookkeeping.Block) (map[basics.Address]struct{}, map[basics.Address]map[ledger.Creatable]struct{}, error) {
	eval, err := l.StartEvaluator(block.BlockHeader, len(block.Payset), 0)
	if err != nil {
		return nil, nil, fmt.Errorf("getModifiedState() start evaluator err: %w", err)
	}

	paysetgroups, err := block.DecodePaysetGroups()
	if err != nil {
		return nil, nil, fmt.Errorf("getModifiedState() decode payset groups err: %w", err)
	}

	for _, group := range paysetgroups {
		err = eval.TransactionGroup(group)
		if err != nil {
			return nil, nil,
				fmt.Errorf("getModifiedState() apply transaction group err: %w", err)
		}
	}

	vb, err := eval.GenerateBlock()
	if err != nil {
		return nil, nil, fmt.Errorf("getModifiedState() generate block err: %w", err)
	}

	accountDeltas := vb.Delta().Accts

	modifiedAccounts := make(map[basics.Address]struct{})
	for _, address := range accountDeltas.ModifiedAccounts() {
		modifiedAccounts[address] = struct{}{}
	}

	modifiedResources := make(map[basics.Address]map[ledger.Creatable]struct{})
	for _, r := range accountDeltas.GetAllAssetResources() {
		c, ok := modifiedResources[r.Addr]
		if !ok {
			c = make(map[ledger.Creatable]struct{})
			modifiedResources[r.Addr] = c
		}
		creatable := ledger.Creatable{
			Index: basics.CreatableIndex(r.Aidx),
			Type:  basics.AssetCreatable,
		}
		c[creatable] = struct{}{}
	}
	for _, r := range accountDeltas.GetAllAppResources() {
		c, ok := modifiedResources[r.Addr]
		if !ok {
			c = make(map[ledger.Creatable]struct{})
			modifiedResources[r.Addr] = c
		}
		creatable := ledger.Creatable{
			Index: basics.CreatableIndex(r.Aidx),
			Type:  basics.AppCreatable,
		}
		c[creatable] = struct{}{}
	}

	return modifiedAccounts, modifiedResources, nil
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
