package processor

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/accounting"
)

type Processor interface {
	Process(cert *rpcs.EncodedBlockCert)
	SetHandler(handler func(block *ledgercore.ValidatedBlock) error)
	GetLastProcessedRound() uint64
	Start(path string, client *algod.Client)
}

type ProcessorImpl struct {
	handler            func(block *ledgercore.ValidatedBlock) error
	lastProcessedRound uint64
	ledger             *ledger.Ledger
	aclient            *algod.Client
	ledgerPath         string
}

func (processor *ProcessorImpl) Start() {
	//open ledger
	genesis, err := getGenesis(processor.aclient)
	if err != nil {
		fmt.Printf("error getting genesis err: %v", err)
		os.Exit(1)
	}
	genesisBlock, err := getGenesisBlock(processor.aclient)
	if err != nil {
		fmt.Printf("error getting genesis block err: %v", err)
		os.Exit(1)
	}
	openLedger(processor.ledgerPath, &genesis, &genesisBlock)
}

// Process a raw algod block
func (processor *ProcessorImpl) Process(cert *rpcs.EncodedBlockCert) {
	if processor.ledger == nil {
		panic(fmt.Errorf("local ledger not initialized"))
	}
	if cert.Block.Round() == basics.Round(0) {
		// Block 0 is special, we cannot run the evaluator on it
		err := processor.ledger.AddBlock(cert.Block, agreement.Certificate{})
		if err != nil {
			panic(fmt.Errorf("error adding round  %v to local ledger", err.Error()))
		}
		processor.lastProcessedRound = processor.lastProcessedRound + 1
	} else {
		proto, ok := config.Consensus[cert.Block.BlockHeader.CurrentProtocol]
		if !ok {
			panic(fmt.Errorf(
				"process() cannot find proto version %s", cert.Block.BlockHeader.CurrentProtocol))
		}
		proto.EnableAssetCloseAmount = true

		ledgerForEval, err := MakeLedgerForEvaluator(processor.ledger)
		if err != nil {
			panic(fmt.Errorf("AddBlock() err: %w", err))
		}
		defer ledgerForEval.Close()
		resources, _ := prepareEvalResources(&ledgerForEval, &cert.Block)
		if err != nil {
			panic(fmt.Errorf("AddBlock() eval err: %w", err))
		}

		delta, _, err :=
			ledger.EvalForIndexer(ledgerForEval, &cert.Block, proto, resources)
		if err != nil {
			panic(fmt.Errorf("AddBlock() eval err: %w", err))
		}
		//validated block
		vb := ledgercore.MakeValidatedBlock(cert.Block, delta)
		//	write to ledger
		err = processor.ledger.AddValidatedBlock(vb, agreement.Certificate{})
		if err != nil {
			panic(fmt.Errorf("AddBlock() eval err: %w", err))
		}
		processor.lastProcessedRound = processor.lastProcessedRound + 1
		if processor.handler != nil {
			processor.handler(&vb)
		}
	}
}

func (processor *ProcessorImpl) SetHandler(handler func(block *ledgercore.ValidatedBlock) error) {
	processor.handler = handler
}
func (processor *ProcessorImpl) GetLastProcessedRound() uint64 {
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

func getGenesisBlock(client *algod.Client) (bookkeeping.Block, error) {
	data, err := client.BlockRaw(0).Do(context.Background())
	if err != nil {
		return bookkeeping.Block{}, fmt.Errorf("getGenesisBlock() client err: %w", err)
	}

	var block rpcs.EncodedBlockCert
	err = protocol.Decode(data, &block)
	if err != nil {
		return bookkeeping.Block{}, fmt.Errorf("getGenesisBlock() decode err: %w", err)
	}

	return block.Block, nil
}

func getGenesis(client *algod.Client) (bookkeeping.Genesis, error) {
	data, err := client.GetGenesis().Do(context.Background())
	if err != nil {
		return bookkeeping.Genesis{}, fmt.Errorf("getGenesis() client err: %w", err)
	}

	var res bookkeeping.Genesis
	err = protocol.DecodeJSON([]byte(data), &res)
	if err != nil {
		return bookkeeping.Genesis{}, fmt.Errorf("getGenesis() decode err: %w", err)
	}

	return res, nil
}
