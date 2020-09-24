package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/spf13/cobra"
	//"github.com/spf13/cobra/doc" // TODO: enable cobra doc generation
	"github.com/spf13/viper"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/version"
)

func maybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errfmt, params...)
	os.Exit(1)
}

var rootCmd = &cobra.Command{
	Use:   "indexer",
	Short: "Algorand Indexer",
	Long:  `indexer imports blocks from an algod node or from local files into an SQL database for querying. indexer is a daemon that can serve queries from that database.`,
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
			maybeFail(err, "%s: could not create pid file, %v\n", pidFilePath, err)
			_, err = fmt.Fprintf(fout, "%d", os.Getpid())
			maybeFail(err, "%s: could not write pid file, %v\n", pidFilePath, err)
			err = fout.Close()
			maybeFail(err, "%s: could not close pid file, %v\n", pidFilePath, err)
		}
		if cpuProfile != "" {
			var err error
			profFile, err = os.Create(cpuProfile)
			maybeFail(err, "%s: create, %v\n", cpuProfile, err)
			err = pprof.StartCPUProfile(profFile)
			maybeFail(err, "%s: start pprof, %v\n", cpuProfile, err)
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
				fmt.Fprintf(os.Stderr, "%s: could not remove pid file, %v\n", pidFilePath, err)
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
	db             idb.IndexerDb
	profFile       io.WriteCloser
)

func globalIndexerDb(opts *idb.IndexerDbOptions) idb.IndexerDb {
	if db == nil {
		if postgresAddr != "" {
			var err error
			db, err = idb.IndexerDbByName("postgres", postgresAddr, opts)
			maybeFail(err, "could not init db, %v\n", err)
		} else if dummyIndexerDb {
			db = idb.DummyIndexerDb()
		} else {
			fmt.Fprintf(os.Stderr, "no import db set\n")
			os.Exit(1)
		}
	}
	return db
}

func init() {
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(daemonCmd)

	rootCmd.PersistentFlags().StringVarP(&postgresAddr, "postgres", "P", "", "connection string for postgres database")
	rootCmd.PersistentFlags().BoolVarP(&dummyIndexerDb, "dummydb", "n", false, "use dummy indexer db")
	rootCmd.PersistentFlags().StringVarP(&cpuProfile, "cpuprofile", "", "", "file to record cpu profile to")
	rootCmd.PersistentFlags().StringVarP(&pidFilePath, "pidfile", "", "", "file to write daemon's process id to")
	rootCmd.PersistentFlags().BoolVarP(&doVersion, "version", "v", false, "print version and exit")

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
			// Config file not found; the error message indicates locations where we look
			fmt.Println(err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
			os.Exit(1)
		}
	}

	viper.SetEnvPrefix(config.EnvPrefix)
	viper.AutomaticEnv()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
