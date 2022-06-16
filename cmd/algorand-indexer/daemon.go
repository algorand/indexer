package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/algorand/indexer/config"
	"github.com/spf13/pflag"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/go-algorand/util"
	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	localledger "github.com/algorand/indexer/migrations/local_ledger"
	"github.com/algorand/indexer/processor"
	"github.com/algorand/indexer/processor/blockprocessor"
	"github.com/algorand/indexer/util/metrics"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type daemonConfig struct {
	flags                     *pflag.FlagSet
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
	initLedger                bool
	catchpoint                string
	cpuProfile                string
	pidFilePath               string
	configFile                string
	suppliedAPIConfigFile     string
}

func newDaemonConfig() *daemonConfig {
	return &daemonConfig{}
}

func newDaemonCmd(config *daemonConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "run indexer daemon",
		Long:  "run indexer daemon. Serve api on HTTP.",
		//Args:
		Run: func(cmd *cobra.Command, args []string) {
			config.flags = cmd.Flags()
			if err := runDaemon(config); err != nil {
				panic(exit{1})
			}
		},
	}
	config.setFlags(cmd.Flags())
	return cmd
}

func runDaemon(daemonConfig *daemonConfig) error {
	var err error
	config.BindFlagSet(daemonConfig.flags)
	err = configureLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
		return err
	}

	if daemonConfig.indexerDataDir == "" {
		fmt.Fprint(os.Stderr, "indexer data directory was not provided")
		if err != nil {
			logger.WithError(err).Errorf("indexer data directory error, %v", err)
			return err
		}
	}
	if _, err := os.Stat(daemonConfig.indexerDataDir); os.IsNotExist(err) {
		err := os.Mkdir(daemonConfig.indexerDataDir, 0755)
		if err != nil {
			logger.WithError(err).Errorf("indexer data directory error, %v", err)
			return err
		}
	}

	// Detect the various auto-loading configs from data directory
	indexerConfigFound := util.FileExists(filepath.Join(daemonConfig.indexerDataDir, autoLoadIndexerConfigName))
	paramConfigFound := util.FileExists(filepath.Join(daemonConfig.indexerDataDir, autoLoadParameterConfigName))

	// If we auto-loaded configs but a user supplied them as well, we have an error
	if indexerConfigFound {
		if daemonConfig.configFile != "" {
			err = fmt.Errorf("indexer configuration was found in data directory (%s) as well as supplied via command line.  Only provide one",
				filepath.Join(daemonConfig.indexerDataDir, autoLoadIndexerConfigName))
			logger.WithError(err)
			return err
		}
		// No config file supplied via command line, auto-load it
		configs, err := os.Open(daemonConfig.configFile)
		if err != nil {
			logger.WithError(err).Errorf("%v", err)
			return err
		}
		defer configs.Close()
		err = viper.ReadConfig(configs)
		if err != nil {
			logger.WithError(err).Errorf("invalid config file (%s): %v", viper.ConfigFileUsed(), err)
			return err
		}
	}
	if daemonConfig.configFile != "" {
		configs, err := os.Open(daemonConfig.configFile)
		if err != nil {
			logger.WithError(err).Errorf("File Does Not Exist Error: %v", err)
			return err
		}
		defer configs.Close()
		err = viper.ReadConfig(configs)
		if err != nil {
			logger.WithError(err).Errorf("invalid config file (%s): %v", viper.ConfigFileUsed(), err)
			return err
		}
		fmt.Printf("Using configuration file: %s\n", daemonConfig.configFile)
	}

	if paramConfigFound {
		if suppliedAPIConfigFile != "" {
			logger.Errorf(
				"api parameter configuration was found in data directory (%s) as well as supplied via command line.  Only provide one.",
				filepath.Join(daemonConfig.indexerDataDir, autoLoadParameterConfigName))
			panic(exit{1})
		}
		suppliedAPIConfigFile = filepath.Join(daemonConfig.indexerDataDir, autoLoadParameterConfigName)
		fmt.Printf("Auto-loading parameter configuration file: %s", suppliedAPIConfigFile)

	}

	if daemonConfig.pidFilePath != "" {
		fmt.Printf("Creating PID file at: %s\n", daemonConfig.pidFilePath)
		fout, err := os.Create(daemonConfig.pidFilePath)
		if err != nil {
			logger.WithError(err).Errorf("%s: could not create pid file, %v", daemonConfig.pidFilePath, err)
			return err
		}
		_, err = fmt.Fprintf(fout, "%d", os.Getpid())
		if err != nil {
			logger.WithError(err).Errorf("%s: could not write pid file, %v", daemonConfig.pidFilePath, err)
			return err
		}
		err = fout.Close()
		if err != nil {
			logger.WithError(err).Errorf("%s: could not close pid file, %v", daemonConfig.pidFilePath, err)
			return err
		}
		defer func(name string) {
			err := os.Remove(name)
			if err != nil {
				logger.WithError(err).Errorf("%s: could not remove pid file", daemonConfig.pidFilePath)
			}
		}(daemonConfig.pidFilePath)
	}

	if daemonConfig.cpuProfile != "" {
		var err error
		profFile, err = os.Create(daemonConfig.cpuProfile)
		if err != nil {
			logger.WithError(err).Errorf("%s: create, %v", daemonConfig.cpuProfile, err)
			return err
		}
		defer profFile.Close()
		err = pprof.StartCPUProfile(profFile)
		if err != nil {
			logger.WithError(err).Errorf("%s: start pprof, %v", daemonConfig.cpuProfile, err)
			return err
		}
		defer pprof.StopCPUProfile()
	}

	// If someone supplied a configuration file but also said to enable all parameters,
	// that's an error
	if daemonConfig.suppliedAPIConfigFile != "" && daemonConfig.enableAllParameters {
		err = errors.New("not allowed to supply an api config file and enable all parameters")
		logger.WithError(err).Errorf("API Parameter Error: %v", err)
		return err
	}

	if daemonConfig.algodDataDir == "" {
		daemonConfig.algodDataDir = os.Getenv("ALGORAND_DATA")
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
	if daemonConfig.noAlgod {
		logger.Info("algod block following disabled")
	} else if daemonConfig.algodAddr != "" && daemonConfig.algodToken != "" {
		bot, err = fetcher.ForNetAndToken(daemonConfig.algodAddr, daemonConfig.algodToken, logger)
		maybeFail(err, "fetcher setup, %v", err)
	} else if daemonConfig.algodDataDir != "" {
		bot, err = fetcher.ForDataDir(daemonConfig.algodDataDir, logger)
		maybeFail(err, "fetcher setup, %v", err)
	} else {
		// no algod was found
		daemonConfig.noAlgod = true
	}
	opts := idb.IndexerDbOptions{}
	if daemonConfig.noAlgod && !daemonConfig.allowMigration {
		opts.ReadOnly = true
	}

	opts.MaxConn = daemonConfig.maxConn
	opts.IndexerDatadir = daemonConfig.indexerDataDir
	opts.AlgodDataDir = daemonConfig.algodDataDir
	opts.AlgodToken = daemonConfig.algodToken
	opts.AlgodAddr = daemonConfig.algodAddr

	db, availableCh := indexerDbFromFlags(opts)
	defer db.Close()
	var wg sync.WaitGroup
	if bot != nil {
		if daemonConfig.indexerDataDir == "" {
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

			// sync local ledger
			nextDBRound, err := db.GetNextRoundToAccount()
			maybeFail(err, "Error getting DB round")
			if nextDBRound > 0 {
				if daemonConfig.catchpoint != "" {
					err = localledger.RunMigrationFastCatchup(logging.NewLogger(), daemonConfig.catchpoint, &opts)
					maybeFail(err, "Error running ledger migration in fast catchup mode")
				}
				err = localledger.RunMigrationSimple(nextDBRound-1, &opts)
				maybeFail(err, "Error running ledger migration")
			}

			logger.Info("Initializing block import handler.")
			imp := importer.NewImporter(db)

			logger.Info("Initializing local ledger.")
			genesisReader = importer.GetGenesisFile(genesisJSONPath, bot.Algod(), logger)
			genesis, err := readGenesis(genesisReader)
			maybeFail(err, "Error reading genesis file")

			proc, err := blockprocessor.MakeProcessor(&genesis, nextDBRound, daemonConfig.indexerDataDir, imp.ImportBlock)
			if err != nil {
				maybeFail(err, "blockprocessor.MakeProcessor() err %v", err)
			}

			bot.SetNextRound(proc.NextRoundToProcess())
			handler := blockHandler(proc, 1*time.Second)
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

	fmt.Printf("serving on %s\n", daemonConfig.daemonServerAddr)
	logger.Infof("serving on %s", daemonConfig.daemonServerAddr)

	options := makeOptions(daemonConfig)

	api.Serve(ctx, daemonConfig.daemonServerAddr, db, bot, logger, options)
	wg.Wait()
	return err
}

func (cfg *daemonConfig) setFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&cfg.algodDataDir, "algod", "d", "", "path to algod data dir, or $ALGORAND_DATA")
	flags.StringVarP(&cfg.algodAddr, "algod-net", "", "", "host:port of algod")
	flags.StringVarP(&cfg.algodToken, "algod-token", "", "", "api access token for algod")
	flags.StringVarP(&genesisJSONPath, "genesis", "g", "", "path to genesis.json (defaults to genesis.json in algod data dir if that was set)")
	flags.StringVarP(&cfg.daemonServerAddr, "server", "S", ":8980", "host:port to serve API on (default :8980)")
	flags.BoolVarP(&cfg.noAlgod, "no-algod", "", false, "disable connecting to algod for block following")
	flags.StringVarP(&cfg.tokenString, "token", "t", "", "an optional auth token, when set REST calls must use this token in a bearer format, or in a 'X-Indexer-API-Token' header")
	flags.BoolVarP(&cfg.developerMode, "dev-mode", "", false, "allow performance intensive operations like searching for accounts at a particular round")
	flags.BoolVarP(&cfg.allowMigration, "allow-migration", "", false, "allow migrations to happen even when no algod connected")
	flags.StringVarP(&cfg.metricsMode, "metrics-mode", "", "OFF", "configure the /metrics endpoint to [ON, OFF, VERBOSE]")
	flags.DurationVarP(&cfg.writeTimeout, "write-timeout", "", 30*time.Second, "set the maximum duration to wait before timing out writes to a http response, breaking connection")
	flags.DurationVarP(&cfg.readTimeout, "read-timeout", "", 5*time.Second, "set the maximum duration for reading the entire request")
	flags.Uint32VarP(&cfg.maxConn, "max-conn", "", 0, "set the maximum connections allowed in the connection pool, if the maximum is reached subsequent connections will wait until a connection becomes available, or timeout according to the read-timeout setting")

	flags.StringVar(&cfg.suppliedAPIConfigFile, "api-config-file", "", "supply an API config file to enable/disable parameters")
	flags.BoolVar(&cfg.enableAllParameters, "enable-all-parameters", false, "override default configuration and enable all parameters. Can't be used with --api-config-file")
	flags.Uint32VarP(&cfg.maxAPIResourcesPerAccount, "max-api-resources-per-account", "", 1000, "set the maximum total number of resources (created assets, created apps, asset holdings, and application local state) per account that will be allowed in REST API lookupAccountByID and searchForAccounts responses before returning a 400 Bad Request. Set zero for no limit")
	flags.Uint32VarP(&cfg.maxTransactionsLimit, "max-transactions-limit", "", 10000, "set the maximum allowed Limit parameter for querying transactions")
	flags.Uint32VarP(&cfg.defaultTransactionsLimit, "default-transactions-limit", "", 1000, "set the default Limit parameter for querying transactions, if none is provided")
	flags.Uint32VarP(&cfg.maxAccountsLimit, "max-accounts-limit", "", 1000, "set the maximum allowed Limit parameter for querying accounts")
	flags.Uint32VarP(&cfg.defaultAccountsLimit, "default-accounts-limit", "", 100, "set the default Limit parameter for querying accounts, if none is provided")
	flags.Uint32VarP(&cfg.maxAssetsLimit, "max-assets-limit", "", 1000, "set the maximum allowed Limit parameter for querying assets")
	flags.Uint32VarP(&cfg.defaultAssetsLimit, "default-assets-limit", "", 100, "set the default Limit parameter for querying assets, if none is provided")
	flags.Uint32VarP(&cfg.maxBalancesLimit, "max-balances-limit", "", 10000, "set the maximum allowed Limit parameter for querying balances")
	flags.Uint32VarP(&cfg.defaultBalancesLimit, "default-balances-limit", "", 1000, "set the default Limit parameter for querying balances, if none is provided")
	flags.Uint32VarP(&cfg.maxApplicationsLimit, "max-applications-limit", "", 1000, "set the maximum allowed Limit parameter for querying applications")
	flags.Uint32VarP(&cfg.defaultApplicationsLimit, "default-applications-limit", "", 100, "set the default Limit parameter for querying applications, if none is provided")

	flags.StringVarP(&cfg.indexerDataDir, "data-dir", "i", "", "path to indexer data dir, or $INDEXER_DATA")
	flags.BoolVar(&cfg.initLedger, "init-ledger", true, "initialize local ledger using sequential mode")
	flags.StringVarP(&cfg.catchpoint, "catchpoint", "", "", "initialize local ledger using fast catchup")

	flags.StringVarP(&cfg.cpuProfile, "cpuprofile", "", "", "file to record cpu profile to")
	flags.StringVarP(&cfg.pidFilePath, "pidfile", "", "", "file to write daemon's process id to")
	flags.StringVarP(&cfg.configFile, "configfile", "c", "", "file path to configuration file (indexer.yml)")
}

func init() {
	viper.RegisterAlias("algod", "algod-data-dir")
	viper.RegisterAlias("algod-net", "algod-address")
	viper.RegisterAlias("server", "server-address")
	viper.RegisterAlias("token", "api-token")
	viper.RegisterAlias("data-dir", "data")
}

// makeOptions converts CLI options to server options
func makeOptions(daemonConfig *daemonConfig) (options api.ExtraOptions) {
	options.DeveloperMode = daemonConfig.developerMode
	if daemonConfig.tokenString != "" {
		options.Tokens = append(options.Tokens, daemonConfig.tokenString)
	}
	switch strings.ToUpper(daemonConfig.metricsMode) {
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
	options.WriteTimeout = daemonConfig.writeTimeout
	options.ReadTimeout = daemonConfig.readTimeout

	options.MaxAPIResourcesPerAccount = uint64(daemonConfig.maxAPIResourcesPerAccount)
	options.MaxTransactionsLimit = uint64(daemonConfig.maxTransactionsLimit)
	options.DefaultTransactionsLimit = uint64(daemonConfig.defaultTransactionsLimit)
	options.MaxAccountsLimit = uint64(daemonConfig.maxAccountsLimit)
	options.DefaultAccountsLimit = uint64(daemonConfig.defaultAccountsLimit)
	options.MaxAssetsLimit = uint64(daemonConfig.maxAssetsLimit)
	options.DefaultAssetsLimit = uint64(daemonConfig.defaultAssetsLimit)
	options.MaxBalancesLimit = uint64(daemonConfig.maxBalancesLimit)
	options.DefaultBalancesLimit = uint64(daemonConfig.defaultBalancesLimit)
	options.MaxApplicationsLimit = uint64(daemonConfig.maxApplicationsLimit)
	options.DefaultApplicationsLimit = uint64(daemonConfig.defaultApplicationsLimit)

	if daemonConfig.enableAllParameters {
		options.DisabledMapConfig = api.MakeDisabledMapConfig()
	} else {
		options.DisabledMapConfig = api.GetDefaultDisabledMapConfigForPostgres()
	}

	if daemonConfig.suppliedAPIConfigFile != "" {
		swag, err := generated.GetSwagger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get swagger: %v", err)
			panic(exit{1})
		}

		logger.Infof("supplied api configuration file located at: %s", daemonConfig.suppliedAPIConfigFile)
		potentialDisabledMapConfig, err := api.MakeDisabledMapConfigFromFile(swag, daemonConfig.suppliedAPIConfigFile)
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
func blockHandler(proc processor.Processor, retryDelay time.Duration) func(context.Context, *rpcs.EncodedBlockCert) error {
	return func(ctx context.Context, block *rpcs.EncodedBlockCert) error {
		for {
			err := handleBlock(block, proc)
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

func handleBlock(block *rpcs.EncodedBlockCert, proc processor.Processor) error {
	start := time.Now()
	err := proc.Process(block)
	if err != nil {
		logger.WithError(err).Errorf(
			"block %d import failed", block.Block.Round())
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

func readGenesis(reader io.Reader) (bookkeeping.Genesis, error) {
	var genesis bookkeeping.Genesis
	if reader == nil {
		return bookkeeping.Genesis{}, fmt.Errorf("readGenesis() err: reader is nil")
	}
	gbytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return bookkeeping.Genesis{}, fmt.Errorf("readGenesis() err: %w", err)
	}
	err = protocol.DecodeJSON(gbytes, &genesis)
	if err != nil {
		return bookkeeping.Genesis{}, fmt.Errorf("readGenesis() err: %w", err)
	}
	return genesis, nil
}
