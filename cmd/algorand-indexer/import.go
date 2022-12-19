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
		config.BindFlagSet(cmd.Flags())
		err := configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			panic(exit{1})
		}

		db, availableCh, err := indexerDbFromFlags(idb.IndexerDbOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			return
		}
		defer db.Close()
		<-availableCh

		helper := importer.NewImportHelper(
			genesisJSONPath,
			blockFileLimit,
			logger)

		helper.Import(db, args)
	},
}

var (
	genesisJSONPath string
	blockFileLimit  int
)

func init() {
	importCmd.Flags().StringVarP(&genesisJSONPath, "genesis", "g", "", "path to genesis.json")
	importCmd.Flags().IntVarP(&blockFileLimit, "block-file-limit", "", 0, "number of block files to process (for debugging)")
}
