package main

import (
	"context"
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

		accounts := db.GetAccounts(context.Background(), idb.AccountQueryOptions{
			EqualToAddress: addr[:],
		})

		num := 0
		var createdAt int64
		for acct := range accounts {
			num++
			createdAt = int64(*acct.Account.CreatedAtRound)
		}

		if num != 0 {
			logger.Errorf("expected one account but found %d for address: %s", num, addr.String())
		}

		filter := idb.UpdateFilter{Address: &addr, StartRound: createdAt}
		importer.UpdateAccounting(db, filter, logger)
	},
}

func init() {
	reindexCmd.Flags().StringVarP(&reindexAccount, "account", "a", "", "account which should be re-indexed")
	reindexCmd.MarkFlagRequired("account")
	reindexCmd.Hidden = true
}
