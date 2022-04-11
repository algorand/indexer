package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/util/metrics"
)

var (
	algodDataDir              string
	algodAddr                 string
	algodToken                string
	daemonServerAddr          string
	noAlgod                   bool
	developerMode             bool
	allowMigration            bool
	metricsMode               string
	tokenString               string
	writeTimeout              time.Duration
	readTimeout               time.Duration
	maxConn                   uint32
	maxAPIResourcesPerAccount uint32
	maxTransactionsLimit      uint32
	defaultTransactionsLimit  uint32
	maxAccountsLimit          uint32
	defaultAccountsLimit      uint32
	maxAssetsLimit            uint32
	defaultAssetsLimit        uint32
	maxBalancesLimit          uint32
	defaultBalancesLimit      uint32
	maxApplicationsLimit      uint32
	defaultApplicationsLimit  uint32
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "run indexer daemon",
	Long:  "run indexer daemon. Serve api on HTTP.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		config.BindFlags(cmd)
		err = configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			os.Exit(1)
		}

		if algodDataDir == "" {
			algodDataDir = os.Getenv("ALGORAND_DATA")
		}

		ctx, cf := context.WithCancel(context.Background())
		defer cf()
		{
			cancelCh := make(chan os.Signal, 1)
			signal.Notify(cancelCh, syscall.SIGTERM, syscall.SIGINT)
			go func() {
				<-cancelCh
				logger.Println("Stopping Indexer.")
				cf()
			}()
		}

		var bot fetcher.Fetcher
		if noAlgod {
			logger.Info("algod block following disabled")
		} else if algodAddr != "" && algodToken != "" {
			bot, err = fetcher.ForNetAndToken(algodAddr, algodToken, logger)
			maybeFail(err, "fetcher setup, %v", err)
		} else if algodDataDir != "" {
			bot, err = fetcher.ForDataDir(algodDataDir, logger)
			maybeFail(err, "fetcher setup, %v", err)
		} else {
			// no algod was found
			noAlgod = true
		}
		opts := idb.IndexerDbOptions{}
		if noAlgod && !allowMigration {
			opts.ReadOnly = true
		}

		opts.MaxConn = maxConn

		db, availableCh := indexerDbFromFlags(opts)
		defer db.Close()
		var wg sync.WaitGroup
		if bot != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()

				// Wait until the database is available.
				<-availableCh

				// Initial import if needed.
				genesisReader := importer.GetGenesisFile(genesisJSONPath, bot.Algod(), logger)
				_, err := importer.EnsureInitialImport(db, genesisReader, logger)
				maybeFail(err, "importer.EnsureInitialImport() error")
				logger.Info("Initializing block import handler.")

				nextRound, err := db.GetNextRoundToAccount()
				maybeFail(err, "failed to get next round, %v", err)
				bot.SetNextRound(nextRound)

				imp := importer.NewImporter(db)
				handler := blockHandler(imp, 1*time.Second)
				bot.SetBlockHandler(handler)

				logger.Info("Starting block importer.")
				err = bot.Run(ctx)
				if err != nil {
					// If context is not expired.
					if ctx.Err() == nil {
						logger.WithError(err).Errorf("fetcher exited with error")
						os.Exit(1)
					}
				}
			}()
		} else {
			logger.Info("No block importer configured.")
		}

		fmt.Printf("serving on %s\n", daemonServerAddr)
		logger.Infof("serving on %s", daemonServerAddr)

		options := makeOptions()

		api.Serve(ctx, daemonServerAddr, db, bot, logger, options)
		wg.Wait()
	},
}

func init() {
	daemonCmd.Flags().StringVarP(&algodDataDir, "algod", "d", "", "path to algod data dir, or $ALGORAND_DATA")
	daemonCmd.Flags().StringVarP(&algodAddr, "algod-net", "", "", "host:port of algod")
	daemonCmd.Flags().StringVarP(&algodToken, "algod-token", "", "", "api access token for algod")
	daemonCmd.Flags().StringVarP(&genesisJSONPath, "genesis", "g", "", "path to genesis.json (defaults to genesis.json in algod data dir if that was set)")
	daemonCmd.Flags().StringVarP(&daemonServerAddr, "server", "S", ":8980", "host:port to serve API on (default :8980)")
	daemonCmd.Flags().BoolVarP(&noAlgod, "no-algod", "", false, "disable connecting to algod for block following")
	daemonCmd.Flags().StringVarP(&tokenString, "token", "t", "", "an optional auth token, when set REST calls must use this token in a bearer format, or in a 'X-Indexer-API-Token' header")
	daemonCmd.Flags().BoolVarP(&developerMode, "dev-mode", "", false, "allow performance intensive operations like searching for accounts at a particular round")
	daemonCmd.Flags().BoolVarP(&allowMigration, "allow-migration", "", false, "allow migrations to happen even when no algod connected")
	daemonCmd.Flags().StringVarP(&metricsMode, "metrics-mode", "", "OFF", "configure the /metrics endpoint to [ON, OFF, VERBOSE]")
	daemonCmd.Flags().DurationVarP(&writeTimeout, "write-timeout", "", 30*time.Second, "set the maximum duration to wait before timing out writes to a http response, breaking connection")
	daemonCmd.Flags().DurationVarP(&readTimeout, "read-timeout", "", 5*time.Second, "set the maximum duration for reading the entire request")
	daemonCmd.Flags().Uint32VarP(&maxConn, "max-conn", "", 0, "set the maximum connections allowed in the connection pool, if the maximum is reached subsequent connections will wait until a connection becomes available, or timeout according to the read-timeout setting")

	daemonCmd.Flags().StringVar(&suppliedAPIConfigFile, "api-config-file", "", "supply an API config file to enable/disable parameters")
	daemonCmd.Flags().Uint32VarP(&maxAPIResourcesPerAccount, "max-api-resources-per-account", "", 1000, "set the maximum total number of resources (created assets, created apps, asset holdings, and application local state) per account that will be allowed in REST API lookupAccountByID and searchForAccounts responses before returning a 400 Bad Request. Set zero for no limit")
	daemonCmd.Flags().Uint32VarP(&maxTransactionsLimit, "max-transactions-limit", "", 10000, "set the maximum allowed Limit parameter for querying transactions")
	daemonCmd.Flags().Uint32VarP(&defaultTransactionsLimit, "default-transactions-limit", "", 1000, "set the default Limit parameter for querying transactions, if none is provided")
	daemonCmd.Flags().Uint32VarP(&maxAccountsLimit, "max-accounts-limit", "", 1000, "set the maximum allowed Limit parameter for querying accounts")
	daemonCmd.Flags().Uint32VarP(&defaultAccountsLimit, "default-accounts-limit", "", 100, "set the default Limit parameter for querying accounts, if none is provided")
	daemonCmd.Flags().Uint32VarP(&maxAssetsLimit, "max-assets-limit", "", 1000, "set the maximum allowed Limit parameter for querying assets")
	daemonCmd.Flags().Uint32VarP(&defaultAssetsLimit, "default-assets-limit", "", 100, "set the default Limit parameter for querying assets, if none is provided")
	daemonCmd.Flags().Uint32VarP(&maxBalancesLimit, "max-balances-limit", "", 10000, "set the maximum allowed Limit parameter for querying balances")
	daemonCmd.Flags().Uint32VarP(&defaultBalancesLimit, "default-balances-limit", "", 1000, "set the default Limit parameter for querying balances, if none is provided")
	daemonCmd.Flags().Uint32VarP(&maxApplicationsLimit, "max-applications-limit", "", 1000, "set the maximum allowed Limit parameter for querying applications")
	daemonCmd.Flags().Uint32VarP(&defaultApplicationsLimit, "default-applications-limit", "", 100, "set the default Limit parameter for querying applications, if none is provided")

	viper.RegisterAlias("algod", "algod-data-dir")
	viper.RegisterAlias("algod-net", "algod-address")
	viper.RegisterAlias("server", "server-address")
	viper.RegisterAlias("token", "api-token")
}

// makeOptions converts CLI options to server options
func makeOptions() (options api.ExtraOptions) {
	options.DeveloperMode = developerMode
	if tokenString != "" {
		options.Tokens = append(options.Tokens, tokenString)
	}
	switch strings.ToUpper(metricsMode) {
	case "OFF":
		options.MetricsEndpoint = false
		options.MetricsEndpointVerbose = false
	case "ON":
		options.MetricsEndpoint = true
		options.MetricsEndpointVerbose = false
	case "VERBOSE":
		options.MetricsEndpoint = true
		options.MetricsEndpointVerbose = true

	}
	options.WriteTimeout = writeTimeout
	options.ReadTimeout = readTimeout

	options.MaxAPIResourcesPerAccount = uint64(maxAPIResourcesPerAccount)
	options.MaxTransactionsLimit = uint64(maxTransactionsLimit)
	options.DefaultTransactionsLimit = uint64(defaultTransactionsLimit)
	options.MaxAccountsLimit = uint64(maxAccountsLimit)
	options.DefaultAccountsLimit = uint64(defaultAccountsLimit)
	options.MaxAssetsLimit = uint64(maxAssetsLimit)
	options.DefaultAssetsLimit = uint64(defaultAssetsLimit)
	options.MaxBalancesLimit = uint64(maxBalancesLimit)
	options.DefaultBalancesLimit = uint64(defaultBalancesLimit)
	options.MaxApplicationsLimit = uint64(maxApplicationsLimit)
	options.DefaultApplicationsLimit = uint64(defaultApplicationsLimit)

	options.DisabledMapConfig = api.MakeDisabledMapConfig()

	if suppliedAPIConfigFile != "" {
		swag, err := generated.GetSwagger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get swagger: %v", err)
			os.Exit(1)
		}

		logger.Infof("supplied api configuration file located at: %s", suppliedAPIConfigFile)
		potentialDisabledMapConfig, err := api.MakeDisabledMapConfigFromFile(swag, suppliedAPIConfigFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to created disabled map config from file: %v", err)
			os.Exit(1)
		}
		options.DisabledMapConfig = potentialDisabledMapConfig
	}

	return
}

// blockHandler creates a handler complying to the fetcher block handler interface. In case of a failure it keeps
// attempting to add the block until the fetcher shuts down.
func blockHandler(imp importer.Importer, retryDelay time.Duration) func(context.Context, *rpcs.EncodedBlockCert) error {
	return func(ctx context.Context, block *rpcs.EncodedBlockCert) error {
		for {
			err := handleBlock(block, imp)
			if err == nil {
				// return on success.
				return nil
			}

			// Delay or terminate before next attempt.
			select {
			case <-ctx.Done():
				return err
			case <-time.After(retryDelay):
				break
			}
		}
	}
}

func handleBlock(block *rpcs.EncodedBlockCert, imp importer.Importer) error {
	start := time.Now()
	err := imp.ImportBlock(block)
	if err != nil {
		logger.WithError(err).Errorf(
			"adding block %d to database failed", block.Block.Round())
		return fmt.Errorf("handleBlock() err: %w", err)
	}
	dt := time.Since(start)

	// Ignore round 0 (which is empty).
	if block.Block.Round() > 0 {
		metrics.BlockImportTimeSeconds.Observe(dt.Seconds())
		metrics.ImportedTxnsPerBlock.Observe(float64(len(block.Block.Payset)))
		metrics.ImportedRoundGauge.Set(float64(block.Block.Round()))
		txnCountByType := make(map[string]int)
		for _, txn := range block.Block.Payset {
			txnCountByType[string(txn.Txn.Type)]++
		}
		for k, v := range txnCountByType {
			metrics.ImportedTxns.WithLabelValues(k).Set(float64(v))
		}
	}

	logger.Infof("round r=%d (%d txn) imported in %s", block.Block.Round(), len(block.Block.Payset), dt.String())

	return nil
}
