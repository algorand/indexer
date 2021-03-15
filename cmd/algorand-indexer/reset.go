package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/idb"
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
				fmt.Println("Done. To re-build, re-start algorand-indexer daemon")
				return
			}
		}
		err = scanner.Err()
		maybeFail(err, "getting prompt char")
		fmt.Println("not resetting")
	},
}
