package core

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/processor/blockprocessor"
	"github.com/algorand/indexer/util"
)

// ImportValidatorArgs required parameters for the import validator servies.
type ImportValidatorArgs struct {
	AlgodAddr       string
	AlgodToken      string
	AlgodLedger     string
	PostgresConnStr string
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

func openIndexerDb(postgresConnStr string, genesis *bookkeeping.Genesis, genesisBlock *bookkeeping.Block, logger *logrus.Logger) (*postgres.IndexerDb, error) {
	db, availableCh, err :=
		postgres.OpenPostgres(postgresConnStr, idb.IndexerDbOptions{}, logger)
	if err != nil {
		return nil, fmt.Errorf("openIndexerDb() err: %w", err)
	}
	<-availableCh

	_, err = db.GetNextRoundToAccount()
	if err != idb.ErrorNotInitialized {
		if err != nil {
			return nil, fmt.Errorf("openIndexerDb() err: %w", err)
		}
	} else {
		err = db.LoadGenesis(*genesis)
		if err != nil {
			return nil, fmt.Errorf("openIndexerDb() err: %w", err)
		}
	}

	nextRound, err := db.GetNextRoundToAccount()
	if err != nil {
		return nil, fmt.Errorf("openIndexerDb() err: %w", err)
	}

	if nextRound == 0 {
		vb := ledgercore.MakeValidatedBlock(*genesisBlock, ledgercore.StateDelta{})
		err = db.AddBlock(&vb)
		if err != nil {
			return nil, fmt.Errorf("openIndexerDb() err: %w", err)
		}
	}

	return db, nil
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

	l, err := ledger.OpenLedger(
		logger, path.Join(ledgerPath, "ledger"), false, initState, config.GetDefaultLocal())
	if err != nil {
		return nil, fmt.Errorf("openLedger() open err: %w", err)
	}

	return l, nil
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

// Converts an `ledgercore.AppResource` to a `ledgercore.AccountResource`.
// Returns a copy with the `GlobalState` and `KeyValue` fields normalized if needed.
func convertAppResource(r ledgercore.AppResource) (ar ledgercore.AccountResource) {
	ar.AppLocalState = r.AppLocalState
	ar.AppParams = r.AppParams
	if (r.AppParams != nil) && (len(r.AppParams.GlobalState) == 0) {
		// Make a copy of `AppParams` to avoid modifying ledger's storage.
		appParams := new(basics.AppParams)
		*appParams = *r.AppParams
		appParams.GlobalState = nil
		ar.AppParams = appParams
	}
	if (r.AppLocalState != nil) && (len(r.AppLocalState.KeyValue) == 0) {
		// Make a copy of `AppLocalState` to avoid modifying ledger's storage.
		appLocalState := new(basics.AppLocalState)
		*appLocalState = *r.AppLocalState
		appLocalState.KeyValue = nil
		ar.AppLocalState = appLocalState
	}

	return ar
}

func checkModifiedState(db *postgres.IndexerDb, l *ledger.Ledger, block *bookkeeping.Block, addresses map[basics.Address]struct{}, resources map[basics.Address]map[ledger.Creatable]struct{}) error {
	var accountsIndexer map[basics.Address]ledgercore.AccountData
	var resourcesIndexer map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource
	var err0 error
	var accountsAlgod map[basics.Address]ledgercore.AccountData
	var resourcesAlgod map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource
	var err1 error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		var accounts map[basics.Address]*ledgercore.AccountData
		accounts, resourcesIndexer, err0 = db.GetAccountState(addresses, resources)
		if err0 != nil {
			err0 = fmt.Errorf("checkModifiedState() err0: %w", err0)
			return
		}

		accountsIndexer = make(map[basics.Address]ledgercore.AccountData, len(accounts))
		for address, accountData := range accounts {
			if accountData == nil {
				accountsIndexer[address] = ledgercore.AccountData{}
			} else {
				accountsIndexer[address] = *accountData
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		accountsAlgod = make(map[basics.Address]ledgercore.AccountData, len(addresses))
		for address := range addresses {
			accountsAlgod[address], _, err1 = l.LookupWithoutRewards(block.Round(), address)
			if err1 != nil {
				err1 = fmt.Errorf("checkModifiedState() lookup account err1: %w", err1)
				return
			}
		}
		resourcesAlgod =
			make(map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource)
		for address, creatables := range resources {
			resourcesForAddress := make(map[ledger.Creatable]ledgercore.AccountResource)
			resourcesAlgod[address] = resourcesForAddress
			for creatable := range creatables {
				switch creatable.Type {
				case basics.AssetCreatable:
					var assetResource ledgercore.AssetResource
					assetResource, err1 = l.LookupAsset(block.Round(), address, basics.AssetIndex(creatable.Index))
					if err1 != nil {
						err1 = fmt.Errorf("checkModifiedState() lookup asset resource err1: %w", err1)
						return
					}
					ar := ledgercore.AccountResource{
						AssetParams:  assetResource.AssetParams,
						AssetHolding: assetResource.AssetHolding,
					}
					resourcesForAddress[creatable] = ar
				case basics.AppCreatable:
					var appResource ledgercore.AppResource
					appResource, err1 = l.LookupApplication(block.Round(), address, basics.AppIndex(creatable.Index))
					if err1 != nil {
						err1 = fmt.Errorf("checkModifiedState() lookup application resource err1: %w", err1)
						return
					}
					resourcesForAddress[creatable] = convertAppResource(appResource)
				default:
					err1 = fmt.Errorf("checkModifiedState() unexpected creatable type %v", creatable.Type)
					return
				}
			}
		}
	}()

	wg.Wait()
	if err0 != nil {
		return err0
	}
	if err1 != nil {
		return err1
	}

	if !reflect.DeepEqual(accountsIndexer, accountsAlgod) {
		diff := util.Diff(accountsAlgod, accountsIndexer)
		return fmt.Errorf(
			"checkModifiedState() accounts differ,"+
				"\naccountsIndexer: %+v,\naccountsAlgod: %+v,\ndiff: %s",
			accountsIndexer, accountsAlgod, diff)
	}
	if !reflect.DeepEqual(resourcesIndexer, resourcesAlgod) {
		diff := util.Diff(resourcesAlgod, resourcesIndexer)
		return fmt.Errorf(
			"checkModifiedState() resources differ,"+
				"\nresourcesIndexer: %+v,\nresourcesAlgod: %+v,\ndiff: %s",
			resourcesIndexer, resourcesAlgod, diff)
	}

	return nil
}

func catchup(db *postgres.IndexerDb, l *ledger.Ledger, bot fetcher.Fetcher, logger *logrus.Logger) error {
	nextRoundIndexer, err := db.GetNextRoundToAccount()
	if err != nil {
		return fmt.Errorf("catchup err: %w", err)
	}
	nextRoundLedger := uint64(l.Latest()) + 1

	if nextRoundLedger > nextRoundIndexer {
		return fmt.Errorf(
			"catchup() ledger is ahead of indexer nextRoundIndexer: %d nextRoundLedger: %d",
			nextRoundIndexer, nextRoundLedger)
	}

	if nextRoundIndexer > nextRoundLedger+1 {
		return fmt.Errorf(
			"catchup() indexer is too ahead of ledger "+
				"nextRoundIndexer: %d nextRoundLedger: %d",
			nextRoundIndexer, nextRoundLedger)
	}

	blockHandlerFunc := func(ctx context.Context, block *rpcs.EncodedBlockCert) error {
		var modifiedAccounts map[basics.Address]struct{}
		var modifiedResources map[basics.Address]map[ledger.Creatable]struct{}
		var err0 error
		var err1 error
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			modifiedAccounts, modifiedResources, err0 = getModifiedState(l, &block.Block)
			wg.Done()
		}()

		if nextRoundLedger >= nextRoundIndexer {
			wg.Add(1)
			prc, err := blockprocessor.MakeProcessorWithLedger(l, db.AddBlock)
			if err != nil {
				return fmt.Errorf("catchup() err: %w", err)
			}
			blockCert := rpcs.EncodedBlockCert{Block: block.Block, Certificate: block.Certificate}
			go func() {
				start := time.Now()
				err1 = prc.Process(&blockCert)
				fmt.Printf(
					"%d transactions imported in %v\n",
					len(block.Block.Payset), time.Since(start))
				wg.Done()
			}()
		}

		wg.Wait()
		if err0 != nil {
			return fmt.Errorf("catchup() err0: %w", err0)
		}
		if nextRoundLedger >= nextRoundIndexer {
			if err1 != nil {
				return fmt.Errorf("catchup() err1: %w", err1)
			}
			nextRoundIndexer++
		}

		err0 = l.AddBlock(block.Block, agreement.Certificate{})
		if err0 != nil {
			return fmt.Errorf("catchup() err0: %w", err0)
		}
		nextRoundLedger++

		return checkModifiedState(
			db, l, &block.Block, modifiedAccounts, modifiedResources)
	}
	bot.SetBlockHandler(blockHandlerFunc)
	bot.SetNextRound(nextRoundLedger)
	err = bot.Run(context.Background())
	if err != nil {
		return fmt.Errorf("catchup err: %w", err)
	}

	return nil
}

// Run is a blocking call that runs the import validator service.
func Run(args ImportValidatorArgs) {
	logger := logrus.New()

	bot, err := fetcher.ForNetAndToken(args.AlgodAddr, args.AlgodToken, logger)
	if err != nil {
		fmt.Printf("error initializing fetcher err: %v", err)
		os.Exit(1)
	}

	genesis, err := getGenesis(bot.Algod())
	if err != nil {
		fmt.Printf("error getting genesis err: %v", err)
		os.Exit(1)
	}
	balances, err := genesis.Balances()
	if err != nil {
		fmt.Printf("error getting genesis balances err: %v", err)
		os.Exit(1)
	}
	genesisBlock, err := bookkeeping.MakeGenesisBlock(genesis.Proto, balances, genesis.ID(), genesis.Hash())
	if err != nil {
		fmt.Printf("error getting genesis block err: %v", err)
		os.Exit(1)
	}

	db, err := openIndexerDb(args.PostgresConnStr, &genesis, &genesisBlock, logger)
	if err != nil {
		fmt.Printf("error opening indexer database err: %v", err)
		os.Exit(1)
	}
	l, err := openLedger(args.AlgodLedger, &genesis, &genesisBlock)
	if err != nil {
		fmt.Printf("error opening algod database err: %v", err)
		os.Exit(1)
	}

	err = catchup(db, l, bot, logger)
	if err != nil {
		fmt.Printf("error catching up err: %v", err)
		os.Exit(1)
	}
}
