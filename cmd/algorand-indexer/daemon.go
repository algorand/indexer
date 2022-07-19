package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/algorand/indexer/plugins"
	"gopkg.in/yaml.v3"
	"io/fs"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/go-algorand/util"

	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/exporters"
	_ "github.com/algorand/indexer/exporters/noop"
	"github.com/algorand/indexer/exporters/postgresql"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/processor"
	"github.com/algorand/indexer/processor/blockprocessor"
	iutil "github.com/algorand/indexer/util"
	"github.com/algorand/indexer/util/metrics"
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
	genesisJSONPath           string
	exporterPlugin            string
}

// DaemonCmd creates the main cobra command, initializes flags, and viper aliases
func DaemonCmd() *cobra.Command {
	cfg := &daemonConfig{}
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "run indexer daemon",
		Long:  "run indexer daemon. Serve api on HTTP.",
		//Args:
		Run: func(cmd *cobra.Command, args []string) {
			if err := runDaemon(cfg); err != nil {
				panic(exit{1})
			}
		},
	}
	cfg.flags = daemonCmd.Flags()
	cfg.flags.StringVarP(&cfg.algodDataDir, "algod", "d", "", "path to algod data dir, or $ALGORAND_DATA")
	cfg.flags.StringVarP(&cfg.algodAddr, "algod-net", "", "", "host:port of algod")
	cfg.flags.StringVarP(&cfg.algodToken, "algod-token", "", "", "api access token for algod")
	cfg.flags.StringVarP(&cfg.genesisJSONPath, "genesis", "g", "", "path to genesis.json (defaults to genesis.json in algod data dir if that was set)")
	cfg.flags.StringVarP(&cfg.daemonServerAddr, "server", "S", ":8980", "host:port to serve API on (default :8980)")
	cfg.flags.BoolVarP(&cfg.noAlgod, "no-algod", "", false, "disable connecting to algod for block following")
	cfg.flags.StringVarP(&cfg.tokenString, "token", "t", "", "an optional auth token, when set REST calls must use this token in a bearer format, or in a 'X-Indexer-API-Token' header")
	cfg.flags.BoolVarP(&cfg.developerMode, "dev-mode", "", false, "allow performance intensive operations like searching for accounts at a particular round")
	cfg.flags.BoolVarP(&cfg.allowMigration, "allow-migration", "", false, "allow migrations to happen even when no algod connected")
	cfg.flags.StringVarP(&cfg.metricsMode, "metrics-mode", "", "OFF", "configure the /metrics endpoint to [ON, OFF, VERBOSE]")
	cfg.flags.DurationVarP(&cfg.writeTimeout, "write-timeout", "", 30*time.Second, "set the maximum duration to wait before timing out writes to a http response, breaking connection")
	cfg.flags.DurationVarP(&cfg.readTimeout, "read-timeout", "", 5*time.Second, "set the maximum duration for reading the entire request")
	cfg.flags.Uint32VarP(&cfg.maxConn, "max-conn", "", 0, "set the maximum connections allowed in the connection pool, if the maximum is reached subsequent connections will wait until a connection becomes available, or timeout according to the read-timeout setting")

	cfg.flags.StringVar(&cfg.suppliedAPIConfigFile, "api-config-file", "", "supply an API config file to enable/disable parameters")
	cfg.flags.BoolVar(&cfg.enableAllParameters, "enable-all-parameters", false, "override default configuration and enable all parameters. Can't be used with --api-config-file")
	cfg.flags.Uint32VarP(&cfg.maxAPIResourcesPerAccount, "max-api-resources-per-account", "", 1000, "set the maximum total number of resources (created assets, created apps, asset holdings, and application local state) per account that will be allowed in REST API lookupAccountByID and searchForAccounts responses before returning a 400 Bad Request. Set zero for no limit")
	cfg.flags.Uint32VarP(&cfg.maxTransactionsLimit, "max-transactions-limit", "", 10000, "set the maximum allowed Limit parameter for querying transactions")
	cfg.flags.Uint32VarP(&cfg.defaultTransactionsLimit, "default-transactions-limit", "", 1000, "set the default Limit parameter for querying transactions, if none is provided")
	cfg.flags.Uint32VarP(&cfg.maxAccountsLimit, "max-accounts-limit", "", 1000, "set the maximum allowed Limit parameter for querying accounts")
	cfg.flags.Uint32VarP(&cfg.defaultAccountsLimit, "default-accounts-limit", "", 100, "set the default Limit parameter for querying accounts, if none is provided")
	cfg.flags.Uint32VarP(&cfg.maxAssetsLimit, "max-assets-limit", "", 1000, "set the maximum allowed Limit parameter for querying assets")
	cfg.flags.Uint32VarP(&cfg.defaultAssetsLimit, "default-assets-limit", "", 100, "set the default Limit parameter for querying assets, if none is provided")
	cfg.flags.Uint32VarP(&cfg.maxBalancesLimit, "max-balances-limit", "", 10000, "set the maximum allowed Limit parameter for querying balances")
	cfg.flags.Uint32VarP(&cfg.defaultBalancesLimit, "default-balances-limit", "", 1000, "set the default Limit parameter for querying balances, if none is provided")
	cfg.flags.Uint32VarP(&cfg.maxApplicationsLimit, "max-applications-limit", "", 1000, "set the maximum allowed Limit parameter for querying applications")
	cfg.flags.Uint32VarP(&cfg.defaultApplicationsLimit, "default-applications-limit", "", 100, "set the default Limit parameter for querying applications, if none is provided")

	cfg.flags.StringVarP(&cfg.indexerDataDir, "data-dir", "i", "", "path to indexer data dir, or $INDEXER_DATA")
	cfg.flags.BoolVar(&cfg.initLedger, "init-ledger", true, "initialize local ledger using sequential mode")
	cfg.flags.StringVarP(&cfg.catchpoint, "catchpoint", "", "", "initialize local ledger using fast catchup")

	cfg.flags.StringVarP(&cfg.cpuProfile, "cpuprofile", "", "", "file to record cpu profile to")
	cfg.flags.StringVarP(&cfg.pidFilePath, "pidfile", "", "", "file to write daemon's process id to")
	cfg.flags.StringVarP(&cfg.configFile, "configfile", "c", "", "file path to configuration file (indexer.yml)")
	cfg.flags.StringVarP(&cfg.exporterPlugin, "exporter", "", postgresql.ExporterName, "name of the exporter plugin to use")
	viper.RegisterAlias("algod", "algod-data-dir")
	viper.RegisterAlias("algod-net", "algod-address")
	viper.RegisterAlias("server", "server-address")
	viper.RegisterAlias("token", "api-token")
	viper.RegisterAlias("data-dir", "data")
	return daemonCmd
}

func configureIndexerDataDir(indexerDataDir string) error {
	var err error
	if indexerDataDir == "" {
		err = fmt.Errorf("indexer data directory was not provided")
		logger.WithError(err).Errorf("indexer data directory error, %v", err)
		return err
	}
	if _, err = os.Stat(indexerDataDir); os.IsNotExist(err) {
		err = os.Mkdir(indexerDataDir, 0755)
		if err != nil {
			logger.WithError(err).Errorf("indexer data directory error, %v", err)
			return err
		}
	}
	return err
}

func loadIndexerConfig(indexerDataDir string, configFile string) error {
	var err error
	var resolvedConfigPath string
	indexerConfigAutoLoadPath := filepath.Join(indexerDataDir, autoLoadIndexerConfigName)
	indexerConfigFound := util.FileExists(indexerConfigAutoLoadPath)
	if indexerConfigFound {
		//autoload
		if configFile != "" {
			err = fmt.Errorf("indexer configuration was found in data directory (%s) as well as supplied via command line.  Only provide one",
				indexerConfigAutoLoadPath)
			logger.Error(err)
			return err
		}
		resolvedConfigPath = indexerConfigAutoLoadPath
	} else if configFile != "" {
		// user specified
		resolvedConfigPath = configFile
	} else {
		// neither autoload nor user specified
		return err
	}
	configs, err := os.Open(resolvedConfigPath)
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
	logger.Infof("Using configuration file: %s\n", resolvedConfigPath)
	return err
}

func loadIndexerParamConfig(cfg *daemonConfig) error {
	var err error
	// If someone supplied a configuration file but also said to enable all parameters,
	// that's an error
	if cfg.suppliedAPIConfigFile != "" && cfg.enableAllParameters {
		err = errors.New("not allowed to supply an api config file and enable all parameters")
		logger.WithError(err).Errorf("API Parameter Error: %v", err)
		return err
	}
	autoloadParamConfigPath := filepath.Join(cfg.indexerDataDir, autoLoadParameterConfigName)
	paramConfigFound := util.FileExists(autoloadParamConfigPath)
	// If we auto-loaded configs but a user supplied them as well, we have an error
	if paramConfigFound {
		if cfg.suppliedAPIConfigFile != "" {
			err = fmt.Errorf("api parameter configuration was found in data directory (%s) as well as supplied via command line.  Only provide one",
				filepath.Join(cfg.indexerDataDir, autoLoadParameterConfigName))
			logger.WithError(err).Errorf("indexer parameter config error: %v", err)
			return err
		}
		cfg.suppliedAPIConfigFile = autoloadParamConfigPath
		logger.Infof("Auto-loading parameter configuration file: %s", suppliedAPIConfigFile)
	}
	return err
}

func createIndexerPidFile(pidFilePath string) error {
	var err error
	logger.Infof("Creating PID file at: %s\n", pidFilePath)
	fout, err := os.Create(pidFilePath)
	if err != nil {
		err = fmt.Errorf("%s: could not create pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}
	_, err = fmt.Fprintf(fout, "%d", os.Getpid())
	if err != nil {
		err = fmt.Errorf("%s: could not write pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}
	err = fout.Close()
	if err != nil {
		err = fmt.Errorf("%s: could not close pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}
	return err
}

func runDaemon(daemonConfig *daemonConfig) error {
	var err error
	config.BindFlagSet(daemonConfig.flags)
	err = configureLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
		return err
	}

	// Create the data directory if necessary/possible
	if err = configureIndexerDataDir(daemonConfig.indexerDataDir); err != nil {
		return err
	}

	// Detect the various auto-loading configs from data directory
	if err = loadIndexerConfig(daemonConfig.indexerDataDir, daemonConfig.configFile); err != nil {
		return err
	}
	// We need to re-run this because loading the config file could change these
	config.BindFlagSet(daemonConfig.flags)

	// Load the Parameter config
	if err = loadIndexerParamConfig(daemonConfig); err != nil {
		return err
	}

	if daemonConfig.pidFilePath != "" {
		err = createIndexerPidFile(daemonConfig.pidFilePath)
		if err != nil {
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
	// if daemonConfig.noAlgod && !daemonConfig.allowMigration {
	opts.ReadOnly = true
	// }

	opts.MaxConn = daemonConfig.maxConn
	opts.IndexerDatadir = daemonConfig.indexerDataDir
	opts.AlgodDataDir = daemonConfig.algodDataDir
	opts.AlgodToken = daemonConfig.algodToken
	opts.AlgodAddr = daemonConfig.algodAddr

	var wg sync.WaitGroup
	if bot != nil {
		wg.Add(1)
		logger.Info("Initializing exporter.")
		// Need to update config file w/ options provided via CLI
		writeCLIToPlugin(daemonConfig)
		exp, err := exporters.ExporterByName(daemonConfig.exporterPlugin, daemonConfig.indexerDataDir, logger)
		if err != nil {
			maybeFail(err, "unable to construct requested Exporter: %v", err)
		}
		go runBlockImporter(ctx, daemonConfig, &wg, bot, opts, exp)
	} else {
		logger.Info("No block importer configured.")
	}

	// Only serve the API for the postgres exporter
	if daemonConfig.exporterPlugin == postgresql.ExporterName {
		fmt.Printf("serving on %s\n", daemonConfig.daemonServerAddr)
		logger.Infof("serving on %s", daemonConfig.daemonServerAddr)
		db, availableCh := indexerDbFromFlags(opts)
		defer db.Close()
		options := makeOptions(daemonConfig)
		// Wait for db to become available
		<-availableCh
		api.Serve(ctx, daemonConfig.daemonServerAddr, db, bot, logger, options)
	}
	wg.Wait()
	return err
}

// Bridges functionality between setting config via CLI and the new plugin configs
func writeCLIToPlugin(cfg *daemonConfig) {
	switch cfg.exporterPlugin {
	case postgresql.Metadata.Name():
		pluginCfg := plugins.LoadConfig(logger, cfg.indexerDataDir, postgresql.Metadata)
		var expCfg postgresql.ExporterConfig
		err := yaml.Unmarshal([]byte(pluginCfg), &expCfg)
		if err != nil {
			maybeFail(err, "unable to load postgresql config: %v", err)
		}
		expCfg.Test = dummyIndexerDb
		expCfg.MaxConn = cfg.maxConn
		expCfg.ConnectionString = postgresAddr
		data, err := yaml.Marshal(expCfg)
		if err != nil {
			maybeFail(err, "failure setting postgresql config data: %v", err)
		}
		err = ioutil.WriteFile(plugins.PluginConfigPath(cfg.indexerDataDir, postgresql.Metadata), []byte(data), fs.ModePerm)
		if err != nil {
			maybeFail(err, "failure writing updated options to postgresql exporter config file: %v", err)
		}
	default:
	}
}

func runBlockImporter(ctx context.Context, cfg *daemonConfig, wg *sync.WaitGroup, bot fetcher.Fetcher, opts idb.IndexerDbOptions, exp exporters.Exporter) {
	// Need to redefine exitHandler() for every go-routine
	defer exitHandler()
	defer wg.Done()

	// Initial import if needed.
	genesisReader := importer.GetGenesisFile(cfg.genesisJSONPath, bot.Algod(), logger)
	genesis, err := iutil.ReadGenesis(genesisReader)
	maybeFail(err, "Error reading genesis file")

	err = exp.HandleGenesis(genesis)
	maybeFail(err, "exporter.HandleGenesis() error")

	logger.Info("Initializing local ledger.")
	proc, err := blockprocessor.MakeProcessorWithLedgerInit(ctx, logger, cfg.catchpoint, &genesis, exp.Round(), opts, exp)
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

		if len((*potentialDisabledMapConfig).Data) == 0 {
			logger.Warnf("All parameters are enabled since the provided parameter configuration file (%s) is empty.", suppliedAPIConfigFile)
		}

		options.DisabledMapConfig = potentialDisabledMapConfig
	} else {
		logger.Infof("Enable all parameters flag is set to: %v", daemonConfig.enableAllParameters)
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
