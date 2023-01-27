package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/viper"

	v "github.com/algorand/indexer/cmd/validator/core"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/dummy"
	_ "github.com/algorand/indexer/idb/postgres"
	_ "github.com/algorand/indexer/util/disabledeadlock"
	"github.com/algorand/indexer/util/metrics"
	"github.com/algorand/indexer/version"
)

const autoLoadIndexerConfigFileName = config.FileName
const autoLoadParameterConfigFileName = "api_config"

// Calling os.Exit() directly will not honor any defer'd statements.
// Instead, we will create an exit type and handler so that we may panic
// and handle any exit specific errors
type exit struct {
	RC int // The exit code
}

// exitHandler will handle a panic with type of exit (see above)
func exitHandler() {
	if err := recover(); err != nil {
		if exit, ok := err.(exit); ok {
			os.Exit(exit.RC)
		}

		// It's not actually an exit type, restore panic
		panic(err)
	}
}

// Requires that main (and every go-routine that this is used)
// have defer exitHandler() called first
func maybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	logger.WithError(err).Errorf(errfmt, params...)
	panic(exit{1})
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
		}
	},
}

var (
	postgresAddr   string
	dummyIndexerDb bool
	doVersion      bool
	profFile       io.WriteCloser
	logLevel       string
	logFile        string
	logger         *log.Logger
)

func indexerDbFromFlags(opts idb.IndexerDbOptions) (idb.IndexerDb, chan struct{}, error) {
	if postgresAddr != "" {
		db, ch, err := idb.IndexerDbByName("postgres", postgresAddr, opts, logger)
		maybeFail(err, "could not init db, %v", err)
		return db, ch, nil
	}
	if dummyIndexerDb {
		return dummy.IndexerDb(), nil, nil
	}
	err := fmt.Errorf("no import db set")
	logger.WithError(err)
	return nil, nil, err
}

func init() {
	// Utilities subcommand for more convenient access to useful testing utilities.
	utilsCmd := &cobra.Command{
		Use:   "util",
		Short: "Utilities for testing Indexer operation and correctness.",
		Long:  "Utilities used for Indexer development. These are low level tools that may require low level knowledge of Indexer deployment and operation. They are included as part of this binary for ease of deployment and automation, and to publicize their existance to people who may find them useful. More detailed documention may be found on github in README files located the different 'cmd' directories.",
	}
	utilsCmd.AddCommand(v.ValidatorCmd)
	rootCmd.AddCommand(utilsCmd)

	logger = log.New()
	logger.SetFormatter(&log.JSONFormatter{
		DisableHTMLEscape: true,
	})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(log.InfoLevel)

	rootCmd.AddCommand(importCmd)
	importCmd.Hidden = true
	daemonCmd := DaemonCmd()
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(apiConfigCmd)

	// Version should be available globally
	rootCmd.Flags().BoolVarP(&doVersion, "version", "v", false, "print version and exit")

	// Not applied globally to avoid adding to utility commands.
	addFlags := func(cmd *cobra.Command) {
		cmd.Flags().StringVarP(&logLevel, "loglevel", "l", "info", "verbosity of logs: [error, warn, info, debug, trace]")
		cmd.Flags().StringVarP(&logFile, "logfile", "f", "", "file to write logs to, if unset logs are written to standard out")
		cmd.Flags().StringVarP(&postgresAddr, "postgres", "P", "", "connection string for postgres database")
		cmd.Flags().BoolVarP(&dummyIndexerDb, "dummydb", "n", false, "use dummy indexer db")
		cmd.Flags().BoolVarP(&doVersion, "version", "v", false, "print version and exit")
	}
	addFlags(daemonCmd)
	addFlags(importCmd)

	viper.RegisterAlias("postgres", "postgres-connection-string")

	// Setup configuration file
	viper.SetConfigName(config.FileName)
	// just hard-code yaml since we support multiple yaml filetypes
	viper.SetConfigType("yaml")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.SetEnvPrefix(config.EnvPrefix)
	viper.AutomaticEnv()

	// Register metrics with the global prometheus handler.
	metrics.RegisterPrometheusMetrics("indexer_daemon")
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

	// Setup our exit handler for maybeFail() and other exit panics
	defer exitHandler()

	if err := rootCmd.Execute(); err != nil {
		logger.WithError(err).Error("an error occurred running indexer")
		os.Exit(1)
	}
}
