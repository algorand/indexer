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
	Long:  "run indexer reset. Serve api on HTTP.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		config.BindFlags(cmd)
		err := configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			os.Exit(1)
		}
		opts := idb.IndexerDbOptions{NoMigrate: true}
		db := globalIndexerDb(&opts)
		timeout := time.Now().Add(5 * time.Second)
		health, err := db.Health()
		maybeFail(err, "could not get db health, %v", err)
		for health.IsMigrating || !health.DBAvailable {
			if time.Now().After(timeout) {
				fmt.Fprintf(os.Stderr, "timed out waiting for db to be ready\n")
				os.Exit(1)
				return
			}
			time.Sleep(100 * time.Millisecond)
			health, err = db.Health()
			maybeFail(err, "could not get db health, %v", err)
		}
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
					blockround, err := db.GetMaxRoundLoaded()
					maybeFail(err, "could not find max block loaded, %v", err)
					if blockround > 0 {
						fmt.Printf("Done resetting. Re-building accounting through block %d...", blockround)
						cache, err := db.GetDefaultFrozen()
						maybeFail(err, "failed to get default frozen cache")
						importer.UpdateAccounting(db, cache, blockround, genesisJSONPath, logger)
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
