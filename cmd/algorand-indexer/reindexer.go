package main

import (
	"fmt"
	"os"

	"github.com/algorand/go-algorand-sdk/types"
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
)

var (
	reindexAccount string
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "reindex a single account",
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

		addr, err := types.DecodeAddress(reindexAccount)
		maybeFail(err, "unable to decode address: %s", reindexAccount)

		maxRounds, err := db.GetMaxRound()
		maybeFail(err, "unable to fetch max rounds")

		limit := int(maxRounds)
		filter := idb.UpdateFilter{
			Address: &addr,
			StartRound: 0,
			RoundLimit: &limit,
		}
		err = db.DeleteAccount(addr)
		maybeFail(err, "unable to delete the account: %s", addr.String())
		importer.UpdateAccounting(db, filter, logger)
	},
}

func init() {
	reindexCmd.Flags().StringVarP(&reindexAccount, "account", "a", "", "account which should be re-indexed")
	reindexCmd.MarkFlagRequired("account")
	reindexCmd.Hidden = true
}
