package blockprocessor

import (
	"context"
	_ "embed" // used to embed config
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/processors"
	indexerledger "github.com/algorand/indexer/conduit/plugins/processors/eval"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/util"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
)

const implementationName = "block_evaluator"

// BlockProcessor is the block processors interface
type BlockProcessor interface {
	NextRoundToProcess() uint64

	processors.Processor
	conduit.Completed
}

// package-wide init function
func init() {
	processors.Register(implementationName, processors.ProcessorConstructorFunc(func() processors.Processor {
		return &blockProcessor{}
	}))
}

type blockProcessor struct {
	handler func(block *ledgercore.ValidatedBlock) error
	ledger  *ledger.Ledger
	logger  *log.Logger

	cfg plugins.PluginConfig
	ctx context.Context

	// lastValidatedBlock is the last validated block that was made via the Process() function
	// to be used with the OnComplete() function
	lastValidatedBlock ledgercore.ValidatedBlock
	// lastValidatedBlockRound is the round at which to add the last validated block
	lastValidatedBlockRound       basics.Round
	lastValidatedBlockCertificate agreement.Certificate
}

//go:embed sample.yaml
var sampleConfig string

func (proc *blockProcessor) Metadata() conduit.Metadata {
	return conduit.Metadata{
		Name:         implementationName,
		Description:  "Local Ledger Block Processor",
		Deprecated:   false,
		SampleConfig: sampleConfig,
	}
}

func (proc *blockProcessor) Config() plugins.PluginConfig {
	return proc.cfg
}

func (proc *blockProcessor) Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *log.Logger) error {
	proc.ctx = ctx
	proc.logger = logger

	// First get the configuration from the string
	var pCfg Config
	err := yaml.Unmarshal([]byte(cfg), &pCfg)
	if err != nil {
		return fmt.Errorf("blockprocessor init error: %w", err)
	}
	proc.cfg = cfg

	genesis := initProvider.GetGenesis()
	round := uint64(initProvider.NextDBRound())

	err = InitializeLedger(ctx, proc.logger, round, *genesis, &pCfg)
	if err != nil {
		return fmt.Errorf("could not initialize ledger: %w", err)
	}

	l, err := util.MakeLedger(proc.logger, false, genesis, pCfg.IndexerDatadir)
	if err != nil {
		return fmt.Errorf("could not make ledger: %w", err)
	}
	if l == nil {
		return fmt.Errorf("ledger was created with nil pointer")
	}
	proc.ledger = l

	if uint64(l.Latest()) > round {
		return fmt.Errorf("the ledger cache is ahead of the required round (%d > %d) and must be re-initialized", l.Latest(), round)
	}

	return nil
}

func (proc *blockProcessor) extractValidatedBlockAndPayset(blockCert *rpcs.EncodedBlockCert) (ledgercore.ValidatedBlock, transactions.Payset, error) {
	var vb ledgercore.ValidatedBlock
	var payset transactions.Payset
	if blockCert == nil {
		return vb, payset, fmt.Errorf("cannot process a nil block")
	}
	if blockCert.Block.Round() == 0 && proc.ledger.Latest() == 0 {
		vb = ledgercore.MakeValidatedBlock(blockCert.Block, ledgercore.StateDelta{})
		return vb, blockCert.Block.Payset, nil
	}
	if blockCert.Block.Round() != (proc.ledger.Latest() + 1) {
		return vb, payset, fmt.Errorf("invalid round blockCert.Block.Round(): %d nextRoundToProcess: %d", blockCert.Block.Round(), uint64(proc.ledger.Latest())+1)
	}

	// Make sure "AssetCloseAmount" is enabled. If it isn't, override the
	// protocol and update the blocks to include transactions with modified
	// apply data.
	proto, ok := config.Consensus[blockCert.Block.BlockHeader.CurrentProtocol]
	if !ok {
		return vb, payset, fmt.Errorf(
			"cannot find proto version %s", blockCert.Block.BlockHeader.CurrentProtocol)
	}
	protoChanged := !proto.EnableAssetCloseAmount
	proto.EnableAssetCloseAmount = true

	ledgerForEval := indexerledger.MakeLedgerForEvaluator(proc.ledger)

	resources, err := prepareEvalResources(&ledgerForEval, &blockCert.Block)
	if err != nil {
		proc.logger.Panicf("ProcessBlockCert() resources err: %v", err)
	}

	start := time.Now()
	delta, payset, err := ledger.EvalForIndexer(ledgerForEval, &blockCert.Block, proto, resources)
	if err != nil {
		return vb, transactions.Payset{}, fmt.Errorf("eval err: %w", err)
	}
	EvalTimeSeconds.Observe(time.Since(start).Seconds())

	// validated block
	if protoChanged {
		block := bookkeeping.Block{
			BlockHeader: blockCert.Block.BlockHeader,
			Payset:      payset,
		}
		vb = ledgercore.MakeValidatedBlock(block, delta)
		return vb, payset, nil
	}
	vb = ledgercore.MakeValidatedBlock(blockCert.Block, delta)
	return vb, blockCert.Block.Payset, nil
}

func (proc *blockProcessor) saveLastValidatedInformation(lastValidatedBlock ledgercore.ValidatedBlock, lastValidatedBlockRound basics.Round, lastValidatedBlockCertificate agreement.Certificate) {

	// Set last validated block for later
	proc.lastValidatedBlock = lastValidatedBlock
	proc.lastValidatedBlockRound = lastValidatedBlockRound
	proc.lastValidatedBlockCertificate = lastValidatedBlockCertificate

}

func (proc *blockProcessor) Close() error {
	if proc.ledger != nil {
		proc.ledger.Close()
	}
	return nil
}

// MakeBlockProcessorWithLedger creates a block processorswith a given ledger
func MakeBlockProcessorWithLedger(logger *log.Logger, l *ledger.Ledger, handler func(block *ledgercore.ValidatedBlock) error) (BlockProcessor, error) {
	if l == nil {
		return nil, fmt.Errorf("MakeBlockProcessorWithLedger() err: local ledger not initialized")
	}
	err := addGenesisBlock(l, handler)
	if err != nil {
		return nil, fmt.Errorf("MakeBlockProcessorWithLedger() err: %w", err)
	}
	return &blockProcessor{logger: logger, ledger: l, handler: handler}, nil
}

// MakeBlockProcessorWithLedgerInit creates a block processor and initializes the ledger.
func MakeBlockProcessorWithLedgerInit(ctx context.Context, logger *log.Logger, nextDbRound uint64, genesis *bookkeeping.Genesis, config Config, handler func(block *ledgercore.ValidatedBlock) error) (BlockProcessor, error) {
	err := InitializeLedger(ctx, logger, nextDbRound, *genesis, &config)
	if err != nil {
		return nil, fmt.Errorf("MakeBlockProcessorWithLedgerInit() err: %w", err)
	}
	return MakeBlockProcessor(logger, genesis, nextDbRound, config.IndexerDatadir, handler)
}

// MakeBlockProcessor creates a block processor
func MakeBlockProcessor(logger *log.Logger, genesis *bookkeeping.Genesis, dbRound uint64, datadir string, handler func(block *ledgercore.ValidatedBlock) error) (BlockProcessor, error) {
	l, err := util.MakeLedger(logger, false, genesis, datadir)
	if err != nil {
		return nil, fmt.Errorf("MakeBlockProcessor() err: %w", err)
	}
	if uint64(l.Latest()) > dbRound {
		return nil, fmt.Errorf("MakeBlockProcessor() err: the ledger cache is ahead of the required round (%d > %d) and must be re-initialized", l.Latest(), dbRound)
	}
	return MakeBlockProcessorWithLedger(logger, l, handler)
}

func addGenesisBlock(l *ledger.Ledger, handler func(block *ledgercore.ValidatedBlock) error) error {
	if handler != nil && l != nil && l.Latest() == 0 {
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

func (proc *blockProcessor) Process(input data.BlockData) (data.BlockData, error) {
	start := time.Now()
	blockCert := input.EncodedBlockCertificate()

	vb, modifiedTxns, err := proc.extractValidatedBlockAndPayset(&blockCert)

	if err != nil {
		return data.BlockData{}, fmt.Errorf("processing error: %w", err)
	}

	// Set last validated block for later
	proc.saveLastValidatedInformation(vb, blockCert.Block.Round(), blockCert.Certificate)

	delta := vb.Delta()
	input.Payset = modifiedTxns
	input.Delta = &delta

	proc.logger.Debugf("Block processor: processed block %d (%s)", input.Round(), time.Since(start))

	return input, nil
}

func (proc *blockProcessor) OnComplete(_ data.BlockData) error {

	if proc.lastValidatedBlockRound == basics.Round(0) {
		return nil
	}
	// write to ledger
	err := proc.ledger.AddValidatedBlock(proc.lastValidatedBlock, proc.lastValidatedBlockCertificate)
	if err != nil {
		return fmt.Errorf("add validated block err: %w", err)
	}

	// wait for commit to disk
	proc.ledger.WaitForCommit(proc.lastValidatedBlockRound)
	return nil

}

func (proc *blockProcessor) NextRoundToProcess() uint64 {
	return uint64(proc.ledger.Latest()) + 1
}

func (proc *blockProcessor) ProvideMetrics() []prometheus.Collector {
	return []prometheus.Collector{
		EvalTimeSeconds,
	}
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

// MakeBlockProcessorHandlerAdapter makes an adapter function that emulates original behavior of block processor
func MakeBlockProcessorHandlerAdapter(proc *BlockProcessor, handler func(block *ledgercore.ValidatedBlock) error) func(cert *rpcs.EncodedBlockCert) error {
	return func(cert *rpcs.EncodedBlockCert) error {
		blockData, err := (*proc).Process(data.MakeBlockDataFromEncodedBlockCertificate(cert))
		if err != nil {
			return err
		}

		vb := blockData.ValidatedBlock()

		if handler != nil {
			err = handler(&vb)
			if err != nil {
				return err
			}
		}

		return (*proc).OnComplete(blockData)
	}
}
