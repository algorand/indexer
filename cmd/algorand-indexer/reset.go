package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "run indexer reset",
	Long:  "run indexer reset. Serve api on HTTP.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		config.BindFlags(cmd)
		err = configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			os.Exit(1)
		}
		opts := idb.IndexerDbOptions{}
		db := globalIndexerDb(&opts)
		ctx, cf := context.WithCancel(context.Background())
		defer cf()
		os.Stdout.Write("Are you sure? [y/N]")
		var b [1]byte
		for true {

		}
	},
}
