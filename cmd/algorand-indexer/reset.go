package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres"
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
		os.Stdout.Write([]byte("Are you sure? [y/N] "))
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			answer := scanner.Text()
			if len(answer) > 0 && (answer[0] == 'y' || answer[0] == 'Y') {
				os.Stdout.Write([]byte("Resetting...\n"))
				time.Sleep(2)
				pdb := db.(*postgres.IndexerDb)
				err = pdb.Reset()
				maybeFail(err, "database reset failed")
				os.Stdout.Write([]byte("Done. To re-build, re-start algorand-indexer daemon\n"))
				return
			}
		}
		err = scanner.Err()
		maybeFail(err, "getting prompt char")
		os.Stdout.Write([]byte("not resetting\n"))
	},
}
