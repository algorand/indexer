package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/go-algorand/util"
	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/util/metrics"

	goconfig "github.com/algorand/go-algorand/config"

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
	enableAllParameters       bool
	indexerDataDir            string
	cpuProfile                string
	pidFilePath               string
	configFile                string
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "run indexer daemon",
	Long:  "run indexer daemon. Serve api on HTTP.",
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		config.BindFlags(cmd)
		err = configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			panic(exit{1})
		}

		if indexerDataDir == "" {
			fmt.Fprint(os.Stderr, "indexer data directory was not provided")
			panic(exit{1})
		}

		// Create the directory if it doesn't exist
		if _, err := os.Stat(indexerDataDir); os.IsNotExist(err) {
			err := os.Mkdir(indexerDataDir, 0755)
			maybeFail(err, "failure while creating data directory: %v", err)
		}

		// Detect the various auto-loading configs from data directory
		indexerConfigFound := util.FileExists(filepath.Join(indexerDataDir, autoLoadIndexerConfigName))
		paramConfigFound := util.FileExists(filepath.Join(indexerDataDir, autoLoadParameterConfigName))
		consensusConfigFound := util.FileExists("/app/consensus.json")

		if consensusConfigFound {
			err = goconfig.LoadConfigurableConsensusProtocols("/app/consensus.json")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to load optional consensus protocols file: /app/consensus.json %v\n", err)
			}else{
				fmt.Printf("/app/consensus.json loaded\n");
			}
		}else{
			fmt.Printf("/app/consensus.json does not exists\n");
		}

		// If we auto-loaded configs but a user supplied them as well, we have an error
		if indexerConfigFound {
			if configFile != "" {
				logger.Errorf(
					"indexer configuration was found in data directory (%s) as well as supplied via command line.  Only provide one.",
					filepath.Join(indexerDataDir, autoLoadIndexerConfigName))
				panic(exit{1})
			}
			// No config file supplied via command line, auto-load it
			configs, err := os.Open(configFile)
			if err != nil {
				maybeFail(err, "%v", err)
			}
			defer configs.Close()
			err = viper.ReadConfig(configs)
			if err != nil {
				maybeFail(err, "invalid config file (%s): %v", viper.ConfigFileUsed(), err)
			}
		}

		if paramConfigFound {
			if suppliedAPIConfigFile != "" {
				logger.Errorf(
					"api parameter configuration was found in data directory (%s) as well as supplied via command line.  Only provide one.",
					filepath.Join(indexerDataDir, autoLoadParameterConfigName))
				panic(exit{1})
			}
			suppliedAPIConfigFile = filepath.Join(indexerDataDir, autoLoadParameterConfigName)
			fmt.Printf("Auto-loading parameter configuration file: %s", suppliedAPIConfigFile)

		}

		if pidFilePath != "" {
			fmt.Printf("Creating PID file at: %s\n", pidFilePath)
			fout, err := os.Create(pidFilePath)
			maybeFail(err, "%s: could not create pid file, %v", pidFilePath, err)
			_, err = fmt.Fprintf(fout, "%d", os.Getpid())
			maybeFail(err, "%s: could not write pid file, %v", pidFilePath, err)
			err = fout.Close()
			maybeFail(err, "%s: could not close pid file, %v", pidFilePath, err)
			defer func(name string) {
				err := os.Remove(name)
				if err != nil {
					logger.WithError(err).Errorf("%s: could not remove pid file", pidFilePath)
				}
			}(pidFilePath)
		}

		if cpuProfile != "" {
			var err error
			profFile, err = os.Create(cpuProfile)
			maybeFail(err, "%s: create, %v", cpuProfile, err)
			defer profFile.Close()
			err = pprof.StartCPUProfile(profFile)
			maybeFail(err, "%s: start pprof, %v", cpuProfile, err)
			defer pprof.StopCPUProfile()
		}

		if configFile != "" {
			configs, err := os.Open(configFile)
			if err != nil {
				maybeFail(err, "%v", err)
			}
			defer configs.Close()
			err = viper.ReadConfig(configs)
			if err != nil {
				maybeFail(err, "invalid config file (%s): %v", viper.ConfigFileUsed(), err)
			}
			fmt.Printf("Using configuration file: %s\n", configFile)
		}

		// If someone supplied a configuration file but also said to enable all parameters,
		// that's an error
		if suppliedAPIConfigFile != "" && enableAllParameters {
			fmt.Fprint(os.Stderr, "not allowed to supply an api config file and enable all parameters")
			panic(exit{1})
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
				// Need to redefine exitHandler() for every go-routine
				defer exitHandler()
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
			if indexerDataDir == "" {
				fmt.Fprint(os.Stderr, "missing indexer data directory")
				panic(exit{1})
			}
			wg.Add(1)
			go func() {
				// Need to redefine exitHandler() for every go-routine
				defer exitHandler()
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
						panic(exit{1})
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
	daemonCmd.Flags().BoolVar(&enableAllParameters, "enable-all-parameters", false, "override default configuration and enable all parameters. Can't be used with --api-config-file")
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

	daemonCmd.Flags().StringVarP(&indexerDataDir, "data-dir", "i", "", "path to indexer data dir, or $INDEXER_DATA")

	daemonCmd.Flags().StringVarP(&cpuProfile, "cpuprofile", "", "", "file to record cpu profile to")
	daemonCmd.Flags().StringVarP(&pidFilePath, "pidfile", "", "", "file to write daemon's process id to")
	daemonCmd.Flags().StringVarP(&configFile, "configfile", "c", "", "file path to configuration file (indexer.yml)")

	viper.RegisterAlias("algod", "algod-data-dir")
	viper.RegisterAlias("algod-net", "algod-address")
	viper.RegisterAlias("server", "server-address")
	viper.RegisterAlias("token", "api-token")
	viper.RegisterAlias("data-dir", "data")
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

	if enableAllParameters {
		options.DisabledMapConfig = api.MakeDisabledMapConfig()
	} else {
		options.DisabledMapConfig = api.GetDefaultDisabledMapConfigForPostgres()
	}

	if suppliedAPIConfigFile != "" {
		swag, err := generated.GetSwagger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get swagger: %v", err)
			panic(exit{1})
		}

		logger.Infof("supplied api configuration file located at: %s", suppliedAPIConfigFile)
		potentialDisabledMapConfig, err := api.MakeDisabledMapConfigFromFile(swag, suppliedAPIConfigFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to created disabled map config from file: %v", err)
			panic(exit{1})
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
