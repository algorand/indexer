package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/pipeline"
	_ "github.com/algorand/indexer/conduit/plugins/exporters/postgresql"
	_ "github.com/algorand/indexer/conduit/plugins/importers/algod"
	_ "github.com/algorand/indexer/conduit/plugins/processors/blockprocessor"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	iutil "github.com/algorand/indexer/util"
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
	maxBoxesLimit             uint32
	defaultBoxesLimit         uint32
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
				fmt.Fprintf(os.Stderr, "Exiting with error: %s\n", err.Error())
				os.Exit(1)
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
	cfg.flags.Uint32VarP(&cfg.maxBoxesLimit, "max-boxes-limit", "", 10000, "set the maximum allowed Limit parameter for searching an app's boxes")
	cfg.flags.Uint32VarP(&cfg.defaultBoxesLimit, "default-boxes-limit", "", 1000, "set the default allowed Limit parameter for searching an app's boxes")

	cfg.flags.StringVarP(&cfg.indexerDataDir, "data-dir", "i", "", "path to indexer data dir, or $INDEXER_DATA")
	cfg.flags.BoolVar(&cfg.initLedger, "init-ledger", true, "initialize local ledger using sequential mode")
	cfg.flags.StringVarP(&cfg.catchpoint, "catchpoint", "", "", "initialize local ledger using fast catchup")

	cfg.flags.StringVarP(&cfg.cpuProfile, "cpuprofile", "", "", "file to record cpu profile to")
	cfg.flags.StringVarP(&cfg.pidFilePath, "pidfile", "", "", "file to write daemon's process id to")
	cfg.flags.StringVarP(&cfg.configFile, "configfile", "c", "", "file path to configuration file (indexer.yml)")
	viper.RegisterAlias("algod", "algod-data-dir")
	viper.RegisterAlias("algod-net", "algod-address")
	viper.RegisterAlias("server", "server-address")
	viper.RegisterAlias("token", "api-token")
	return daemonCmd
}

func configureIndexerDataDir(indexerDataDir string) error {
	var err error
	if indexerDataDir == "" {
		return nil
	}
	if _, err = os.Stat(indexerDataDir); os.IsNotExist(err) {
		err = os.Mkdir(indexerDataDir, 0755)
		if err != nil {
			return fmt.Errorf("indexer data directory error, %v", err)
		}
	}
	return err
}

func resolveConfigFile(indexerDataDir string, configFile string) (string, error) {
	var err error
	potentialIndexerConfigPath, err := iutil.GetConfigFromDataDir(indexerDataDir, autoLoadIndexerConfigFileName, config.FileTypes[:])
	if err != nil {
		return "", err
	}
	indexerConfigFound := potentialIndexerConfigPath != ""

	if indexerConfigFound {
		//autoload
		if configFile != "" {
			err = fmt.Errorf("indexer configuration was found in data directory (%s) as well as supplied via command line.  Only provide one",
				potentialIndexerConfigPath)
			return "", err
		}
		return potentialIndexerConfigPath, nil
	} else if configFile != "" {
		// user specified
		return configFile, nil
	}
	// neither autoload nor user specified
	return "", nil
}

// loadIndexerConfig opens the file and calls viper.ReadConfig
func loadIndexerConfig(configFile string) error {
	if configFile == "" {
		return nil
	}

	configs, err := os.Open(configFile)
	if err != nil {
		return fmt.Errorf("config file does not exist: %w", err)
	}
	defer configs.Close()
	err = viper.ReadConfig(configs)
	if err != nil {
		return fmt.Errorf("invalid config file (%s): %w", configFile, err)
	}
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
	potentialParamConfigPath, err := iutil.GetConfigFromDataDir(cfg.indexerDataDir, autoLoadParameterConfigFileName, config.FileTypes[:])
	if err != nil {
		logger.Error(err)
		return err
	}
	paramConfigFound := potentialParamConfigPath != ""
	// If we auto-loaded configs but a user supplied them as well, we have an error
	if paramConfigFound {
		if cfg.suppliedAPIConfigFile != "" {
			err = fmt.Errorf("api parameter configuration was found in data directory (%s) as well as supplied via command line.  Only provide one",
				potentialParamConfigPath)
			logger.WithError(err).Errorf("indexer parameter config error: %v", err)
			return err
		}
		cfg.suppliedAPIConfigFile = potentialParamConfigPath
		logger.Infof("Auto-loading parameter configuration file: %s", suppliedAPIConfigFile)
	}
	return err
}

func runDaemon(daemonConfig *daemonConfig) error {
	var err error

	// check for config environment variables
	if daemonConfig.indexerDataDir == "" {
		daemonConfig.indexerDataDir = os.Getenv("INDEXER_DATA")
	}
	if daemonConfig.configFile == "" {
		daemonConfig.configFile = os.Getenv("INDEXER_CONFIGFILE")
	}
	if daemonConfig.algodDataDir == "" {
		daemonConfig.algodDataDir = os.Getenv("ALGORAND_DATA")
	}

	// Create the data directory if necessary/possible
	if err = configureIndexerDataDir(daemonConfig.indexerDataDir); err != nil {
		return err
	}

	// Detect the various auto-loading configs from data directory
	var configFile string
	if configFile, err = resolveConfigFile(daemonConfig.indexerDataDir, daemonConfig.configFile); err != nil {
		return err
	}

	if err = loadIndexerConfig(configFile); err != nil {
		return err
	}
	// We need to re-run this because loading the config file could change these
	config.BindFlagSet(daemonConfig.flags)

	if !daemonConfig.noAlgod && daemonConfig.indexerDataDir == "" {
		return fmt.Errorf("indexer data directory was not provided")
	}

	// Configure the logger as soon as we're able so that it can be used.
	err = configureLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
		return err
	}

	if configFile != "" {
		logger.Infof("Using configuration file: %s", configFile)
	}

	// Load the Parameter config
	if err = loadIndexerParamConfig(daemonConfig); err != nil {
		return err
	}

	if daemonConfig.pidFilePath != "" {
		err = iutil.CreateIndexerPidFile(logger, daemonConfig.pidFilePath)
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

	if daemonConfig.algodDataDir != "" {
		daemonConfig.algodAddr, daemonConfig.algodToken, _, err = fetcher.AlgodArgsForDataDir(daemonConfig.algodDataDir)
		if err != nil {
			return fmt.Errorf("algod data dir err, %v", err)
		}
	} else if daemonConfig.algodAddr == "" || daemonConfig.algodToken == "" {
		// no algod was found
		logger.Info("no algod was found, provide either --algod OR --algod-net and --algod-token to enable")
		daemonConfig.noAlgod = true
	}
	if daemonConfig.noAlgod {
		logger.Info("algod block following disabled")
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

	db, availableCh, err := indexerDbFromFlags(opts)
	if err != nil {
		return err
	}
	defer db.Close()
	var dataError func() error
	if daemonConfig.noAlgod != true {
		// Wait until the database is available.
		<-availableCh
		var nextRound uint64
		nextRound, err = db.GetNextRoundToAccount()
		if err == idb.ErrorNotInitialized {
			nextRound = 0
		} else if err != nil {
			return err
		}
		pipeline := runConduitPipeline(ctx, nextRound, daemonConfig)
		if pipeline != nil {
			dataError = pipeline.Error
			defer pipeline.Stop()
		}
	} else {
		logger.Info("No block importer configured.")
	}

	fmt.Printf("serving on %s\n", daemonConfig.daemonServerAddr)
	logger.Infof("serving on %s", daemonConfig.daemonServerAddr)

	options := makeOptions(daemonConfig)

	api.Serve(ctx, daemonConfig.daemonServerAddr, db, dataError, logger, options)
	return err
}

func makeConduitConfig(dCfg *daemonConfig, nextRound uint64) pipeline.Config {
	return pipeline.Config{
		RetryCount: 10,
		RetryDelay: 1 * time.Second,
		ConduitArgs: &conduit.Args{
			ConduitDataDir:    dCfg.indexerDataDir,
			NextRoundOverride: nextRound,
		},
		HideBanner:       true,
		PipelineLogLevel: logger.GetLevel().String(),
		Importer: pipeline.NameConfigPair{
			Name: "algod",
			Config: map[string]interface{}{
				"netaddr": dCfg.algodAddr,
				"token":   dCfg.algodToken,
			},
		},
		Processors: []pipeline.NameConfigPair{
			{
				Name: "block_evaluator",
				Config: map[string]interface{}{
					"catchpoint":     dCfg.catchpoint,
					"data-dir":       dCfg.indexerDataDir,
					"algod-data-dir": dCfg.algodDataDir,
					"algod-token":    dCfg.algodToken,
					"algod-addr":     dCfg.algodAddr,
				},
			},
		},
		Exporter: pipeline.NameConfigPair{
			Name: "postgresql",
			Config: map[string]interface{}{
				"connection-string": postgresAddr,
				"max-conn":          dCfg.maxConn,
				"test":              dummyIndexerDb,
			},
		},
	}

}

func runConduitPipeline(ctx context.Context, nextRound uint64, dCfg *daemonConfig) pipeline.Pipeline {
	// Need to redefine exitHandler() for every go-routine
	defer exitHandler()

	var conduit pipeline.Pipeline
	var err error
	pcfg := makeConduitConfig(dCfg, nextRound)
	if conduit, err = pipeline.MakePipeline(ctx, &pcfg, logger); err != nil {
		logger.Errorf("%v", err)
		panic(exit{1})
	}
	err = conduit.Init()
	if err != nil {
		logger.Errorf("%v", err)
		panic(exit{1})
	}
	conduit.Start()
	return conduit
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
	options.MaxBoxesLimit = uint64(daemonConfig.maxBoxesLimit)
	options.DefaultBoxesLimit = uint64(daemonConfig.defaultBoxesLimit)

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
