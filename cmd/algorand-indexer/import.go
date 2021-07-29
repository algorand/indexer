package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "import block file or tar file of blocks",
	Long:  "import block file or tar file of blocks. arguments are interpret as file globs (e.g. *.tar.bz2)",
	Run: func(cmd *cobra.Command, args []string) {
		config.BindFlags(cmd)
		err := configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			os.Exit(1)
		}

		db, availableCh := indexerDbFromFlags(idb.IndexerDbOptions{})
		<-availableCh

		cache, err := db.GetDefaultFrozen()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to initialize the default frozen cache: %v", err)
			os.Exit(1)
		}

		helper := importer.NewImportHelper(
			cache,
			genesisJSONPath,
			numRoundsLimit,
			blockFileLimit,
			logger)

		helper.Import(db, args)
	},
}

var (
	genesisJSONPath string
	numRoundsLimit  int
	blockFileLimit  int
)

func init() {
	importCmd.Flags().StringVarP(&genesisJSONPath, "genesis", "g", "", "path to genesis.json")
	importCmd.Flags().IntVarP(&numRoundsLimit, "num-rounds-limit", "", 0, "number of rounds to process")
	importCmd.Flags().IntVarP(&blockFileLimit, "block-file-limit", "", 0, "number of block files to process (for debugging)")
}
