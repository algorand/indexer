package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
)

var (
	andRebuild bool
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "run indexer reset",
	Long:  "run indexer reset. This will delete a lot of data. With --and-rebuild it will be rebuilt now, otherwise algorand-indexer daemon will try to rebuild when next it runs. This is mostly a tool for developers and people who are really really sure they want to do this.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		config.BindFlags(cmd)
		err := configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			os.Exit(1)
		}
		opts := idb.IndexerDbOptions{ReadOnly: true}
		db := indexerDbFromFlags(opts)
		fmt.Println("Prior to resetting the database make sure no daemons are connected.")
		fmt.Println("After this command finishes start the daemon again and it will begin to")
		fmt.Println("re-index the data. This can take hours or days depending on the network")
		fmt.Println("size. During this time data will be incomplete, similar to the initial import.")

		fmt.Print("\nAre you sure you would like to reset? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			answer := scanner.Text()
			if len(answer) > 0 && (answer[0] == 'y' || answer[0] == 'Y') {
				fmt.Println("Resetting...")
				time.Sleep(2) // leave some ^C time
				err = db.Reset()
				maybeFail(err, "database reset failed")
				if andRebuild {
					nextRound, err := db.GetNextRoundToLoad()
					maybeFail(err, "failed to get next round, %v", err)

					if nextRound > 0 {
						fmt.Printf(
							"Done resetting. Re-building accounting through block %d...",
							nextRound-1)
						opts.ReadOnly = false
						// db.Close() // TODO: add Close() to IndxerDb interface?
						db = indexerDbFromFlags(opts)
						cache, err := db.GetDefaultFrozen()
						maybeFail(err, "failed to get default frozen cache")
						filter := idb.UpdateFilter{
							MaxRound: nextRound - 1,
						}
						importer.UpdateAccounting(db, cache, filter, logger)
						fmt.Println("Done rebuilding accounting.")
					} else {
						fmt.Println("Done. No blocks to rebuild accounting from.")
					}
				} else {
					fmt.Println("Done. To re-build, re-start algorand-indexer daemon")
				}
				return
			}
		}
		err = scanner.Err()
		maybeFail(err, "getting prompt char")
		fmt.Println("not resetting")
	},
}

func init() {
	resetCmd.Flags().BoolVarP(&andRebuild, "and-rebuild", "", false, "re-run accounting after reset (this could take a while)")
	resetCmd.Flags().StringVarP(&genesisJSONPath, "genesis", "g", "", "path to genesis.json (defaults to genesis.json in algod data dir if that was set)")
}
