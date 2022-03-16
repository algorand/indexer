package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/viper"

	bg "github.com/algorand/indexer/cmd/block-generator/core"
	iv "github.com/algorand/indexer/cmd/import-validator/core"
	v "github.com/algorand/indexer/cmd/validator/core"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/dummy"
	_ "github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/util/metrics"
	"github.com/algorand/indexer/version"
)

func maybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	logger.WithError(err).Errorf(errfmt, params...)
	os.Exit(1)
}

var rootCmd = &cobra.Command{
	Use:   "indexer",
	Short: "Algorand Indexer",
	Long:  `Indexer imports blocks from an algod node into an SQL database for querying. It is a daemon that can serve queries from that database.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		//If no arguments passed, we should fallback to help
		cmd.HelpFunc()(cmd, args)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if doVersion {
			fmt.Printf("%s\n", version.LongVersion())
			os.Exit(0)
			return
		}
		if pidFilePath != "" {
			fout, err := os.Create(pidFilePath)
			maybeFail(err, "%s: could not create pid file, %v", pidFilePath, err)
			_, err = fmt.Fprintf(fout, "%d", os.Getpid())
			maybeFail(err, "%s: could not write pid file, %v", pidFilePath, err)
			err = fout.Close()
			maybeFail(err, "%s: could not close pid file, %v", pidFilePath, err)
		}
		if cpuProfile != "" {
			var err error
			profFile, err = os.Create(cpuProfile)
			maybeFail(err, "%s: create, %v", cpuProfile, err)
			err = pprof.StartCPUProfile(profFile)
			maybeFail(err, "%s: start pprof, %v", cpuProfile, err)
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
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if cpuProfile != "" {
			pprof.StopCPUProfile()
			profFile.Close()
		}
		if pidFilePath != "" {
			err := os.Remove(pidFilePath)
			if err != nil {
				logger.WithError(err).Errorf("%s: could not remove pid file", pidFilePath)
			}
		}
	},
}

var (
	postgresAddr   string
	dummyIndexerDb bool
	doVersion      bool
	cpuProfile     string
	pidFilePath    string
	profFile       io.WriteCloser
	logLevel       string
	logFile        string
	logger         *log.Logger
	configFile     string
)

func indexerDbFromFlags(opts idb.IndexerDbOptions) (idb.IndexerDb, chan struct{}) {
	if postgresAddr != "" {
		db, ch, err := idb.IndexerDbByName("postgres", postgresAddr, opts, logger)
		maybeFail(err, "could not init db, %v", err)
		return db, ch
	}
	if dummyIndexerDb {
		return dummy.IndexerDb(), nil
	}
	logger.Errorf("no import db set")
	os.Exit(1)
	return nil, nil
}

func init() {
	// Utilities subcommand for more convenient access to useful testing utilities.
	utilsCmd := &cobra.Command{
		Use:   "util",
		Short: "Utilities for testing Indexer operation and correctness.",
		Long:  "Utilities used for Indexer development. These are low level tools that may require low level knowledge of Indexer deployment and operation. They are included as part of this binary for ease of deployment and automation, and to publicize their existance to people who may find them useful. More detailed documention may be found on github in README files located the different 'cmd' directories.",
	}
	utilsCmd.AddCommand(iv.ImportValidatorCmd)
	utilsCmd.AddCommand(v.ValidatorCmd)
	utilsCmd.AddCommand(bg.BlockGenerator)
	rootCmd.AddCommand(utilsCmd)

	logger = log.New()
	logger.SetFormatter(&log.JSONFormatter{
		DisableHTMLEscape: true,
	})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(log.InfoLevel)

	rootCmd.AddCommand(importCmd)
	importCmd.Hidden = true
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(apiConfigCmd)

	// Not applied globally to avoid adding to utility commands.
	addFlags := func(cmd *cobra.Command) {
		cmd.Flags().StringVarP(&logLevel, "loglevel", "l", "info", "verbosity of logs: [error, warn, info, debug, trace]")
		cmd.Flags().StringVarP(&logFile, "logfile", "f", "", "file to write logs to, if unset logs are written to standard out")
		cmd.Flags().StringVarP(&postgresAddr, "postgres", "P", "", "connection string for postgres database")
		cmd.Flags().BoolVarP(&dummyIndexerDb, "dummydb", "n", false, "use dummy indexer db")
		cmd.Flags().StringVarP(&cpuProfile, "cpuprofile", "", "", "file to record cpu profile to")
		cmd.Flags().StringVarP(&pidFilePath, "pidfile", "", "", "file to write daemon's process id to")
		cmd.Flags().StringVarP(&configFile, "configfile", "c", "", "file path to configuration file (indexer.yml)")
		cmd.Flags().BoolVarP(&doVersion, "version", "v", false, "print version and exit")
	}
	addFlags(daemonCmd)
	addFlags(importCmd)

	viper.RegisterAlias("postgres", "postgres-connection-string")

	// Setup configuration file
	viper.SetConfigName(config.FileName)
	viper.SetConfigType(config.FileType)
	for _, k := range config.ConfigPaths {
		viper.AddConfigPath(k)
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, not an error since it may be set on the CLI.
		} else {
			fmt.Fprintf(os.Stderr, "invalid config file (%s): %v", viper.ConfigFileUsed(), err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("Using configuration file: %s\n", viper.ConfigFileUsed())
	}

	viper.SetEnvPrefix(config.EnvPrefix)
	viper.AutomaticEnv()

	// Register metrics with the global prometheus handler.
	metrics.RegisterPrometheusMetrics()
}

func configureLogger() error {
	if logLevel != "" {
		level, err := log.ParseLevel(logLevel)
		if err != nil {
			return err
		}
		logger.SetLevel(level)
	}

	if logFile == "-" {
		logger.SetOutput(os.Stdout)
	} else if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		logger.SetOutput(f)
	}

	return nil
}

func main() {

	// Hidden command to generate docs in a given directory
	// algorand-indexer generate-docs [path]
	if len(os.Args) == 3 && os.Args[1] == "generate-docs" {
		err := doc.GenMarkdownTree(rootCmd, os.Args[2])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := rootCmd.Execute(); err != nil {
		logger.WithError(err).Error("an error occurred running indexer")
		os.Exit(1)
	}
}
