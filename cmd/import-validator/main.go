// The import validator tool imports blocks into indexer database and algod's sqlite
// database in lockstep and checks that the modified accounts are the same in the two
// databases. It lets detect the first round where an accounting discrepancy occurs
// and it prints out what the difference is before crashing.
// There is a small limitation, however. The set of modified accounts is computed using
// the sqlite database. Thus, if indexer's accounting were to modify a superset of
// those accounts, this tool would not detect it. This, however, should be unlikely.

package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

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
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/util"
)

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
		err = db.AddBlock(genesisBlock)
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

	ledger, err := ledger.OpenLedger(
		logger, path.Join(ledgerPath, "ledger"), false, initState, config.GetDefaultLocal())
	if err != nil {
		return nil, fmt.Errorf("openLedger() open err: %w", err)
	}

	return ledger, nil
}

func getModifiedAccounts(l *ledger.Ledger, block *bookkeeping.Block) ([]basics.Address, error) {
	eval, err := l.StartEvaluator(block.BlockHeader, len(block.Payset), 0)
	if err != nil {
		return nil, fmt.Errorf("changedAccounts() start evaluator err: %w", err)
	}

	paysetgroups, err := block.DecodePaysetGroups()
	if err != nil {
		return nil, fmt.Errorf("changedAccounts() decode payset groups err: %w", err)
	}

	for _, group := range paysetgroups {
		err = eval.TransactionGroup(group)
		if err != nil {
			return nil, fmt.Errorf("changedAccounts() apply transaction group err: %w", err)
		}
	}

	vb, err := eval.GenerateBlock()
	if err != nil {
		return nil, fmt.Errorf("changedAccounts() generate block err: %w", err)
	}

	accountDeltas := vb.Delta().Accts
	return accountDeltas.ModifiedAccounts(), nil
}

func checkModifiedAccounts(db *postgres.IndexerDb, l *ledger.Ledger, block *bookkeeping.Block, addresses []basics.Address) error {
	var accountsIndexer map[basics.Address]basics.AccountData
	var err0 error
	var accountsAlgod map[basics.Address]basics.AccountData
	var err1 error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		accountsIndexer, err0 = db.GetAccountData(addresses)
		if err0 != nil {
			err0 = fmt.Errorf("checkModifiedAccounts() err0: %w", err0)
			return
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		accountsAlgod = make(map[basics.Address]basics.AccountData, len(addresses))
		for _, address := range addresses {
			var accountData basics.AccountData
			accountData, _, err1 = l.LookupWithoutRewards(block.Round(), address)
			if err1 != nil {
				err1 = fmt.Errorf("checkModifiedAccounts() lookup err1: %w", err1)
				return
			}

			// Indexer returns nil for these maps if they are empty. Unfortunately,
			// in go-algorand it's not well defined, and sometimes ledger returns empty
			// maps and sometimes nil maps. So we set those maps to nil if they are empty so
			// that comparison works.
			if len(accountData.AssetParams) == 0 {
				accountData.AssetParams = nil
			}
			if len(accountData.Assets) == 0 {
				accountData.Assets = nil
			}

			if accountData.AppParams != nil {
				// Make a copy of `AppParams` to avoid modifying ledger's storage.
				appParams :=
					make(map[basics.AppIndex]basics.AppParams, len(accountData.AppParams))
				for index, params := range accountData.AppParams {
					if len(params.GlobalState) == 0 {
						params.GlobalState = nil
					}
					appParams[index] = params
				}
				accountData.AppParams = appParams
			}

			if accountData.AppLocalStates != nil {
				// Make a copy of `AppLocalStates` to avoid modifying ledger's storage.
				appLocalStates :=
					make(map[basics.AppIndex]basics.AppLocalState, len(accountData.AppLocalStates))
				for index, state := range accountData.AppLocalStates {
					if len(state.KeyValue) == 0 {
						state.KeyValue = nil
					}
					appLocalStates[index] = state
				}
				accountData.AppLocalStates = appLocalStates
			}

			accountsAlgod[address] = accountData
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
			"checkModifiedAccounts() accounts differ,"+
				"\naccountsIndexer: %+v,\naccountsAlgod: %+v,\ndiff: %s",
			accountsIndexer, accountsAlgod, diff)
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
		var modifiedAccounts []basics.Address
		var err0 error
		var err1 error
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			modifiedAccounts, err0 = getModifiedAccounts(l, &block.Block)
			wg.Done()
		}()

		if nextRoundLedger >= nextRoundIndexer {
			wg.Add(1)
			go func() {
				start := time.Now()
				err1 = db.AddBlock(&block.Block)
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

		return checkModifiedAccounts(db, l, &block.Block, modifiedAccounts)
	}
	bot.SetBlockHandler(blockHandlerFunc)
	bot.SetNextRound(nextRoundLedger)
	err = bot.Run(context.Background())
	if err != nil {
		return fmt.Errorf("catchup err: %w", err)
	}

	return nil
}

func main() {
	var algodAddr string
	var algodToken string
	var algodLedger string
	var postgresConnStr string

	var rootCmd = &cobra.Command{
		Use:   "import-validator",
		Short: "Import validator",
		Run: func(cmd *cobra.Command, args []string) {
			logger := logrus.New()

			bot, err := fetcher.ForNetAndToken(algodAddr, algodToken, logger)
			if err != nil {
				fmt.Printf("error initializing fetcher err: %v", err)
				os.Exit(1)
			}

			genesis, err := getGenesis(bot.Algod())
			if err != nil {
				fmt.Printf("error getting genesis err: %v", err)
				os.Exit(1)
			}
			genesisBlock, err := getGenesisBlock(bot.Algod())
			if err != nil {
				fmt.Printf("error getting genesis block err: %v", err)
				os.Exit(1)
			}

			db, err := openIndexerDb(postgresConnStr, &genesis, &genesisBlock, logger)
			if err != nil {
				fmt.Printf("error opening indexer database err: %v", err)
				os.Exit(1)
			}
			l, err := openLedger(algodLedger, &genesis, &genesisBlock)
			if err != nil {
				fmt.Printf("error opening algod database err: %v", err)
				os.Exit(1)
			}

			err = catchup(db, l, bot, logger)
			if err != nil {
				fmt.Printf("error catching up err: %v", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.Flags().StringVar(&algodAddr, "algod-net", "", "host:port of algod")
	rootCmd.MarkFlagRequired("algod-net")

	rootCmd.Flags().StringVar(
		&algodToken, "algod-token", "", "api access token for algod")
	rootCmd.MarkFlagRequired("algod-token")

	rootCmd.Flags().StringVar(
		&algodLedger, "algod-ledger", "", "path to algod ledger directory")
	rootCmd.MarkFlagRequired("algod-ledger")

	rootCmd.Flags().StringVar(
		&postgresConnStr, "postgres", "", "connection string for postgres database")
	rootCmd.MarkFlagRequired("postgres")

	rootCmd.Execute()
}
