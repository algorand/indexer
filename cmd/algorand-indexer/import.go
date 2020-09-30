package main

import (
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/importer"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "import block file or tar file of blocks",
	Long:  "import block file or tar file of blocks. arguments are interpret as file globs (e.g. *.tar.bz2)",
	Run: func(cmd *cobra.Command, args []string) {
		config.BindFlags(cmd)

		db := globalIndexerDb(nil)

		helper := importer.ImportHelper{
			BlockFileLimit:  blockFileLimit,
			GenesisJsonPath: genesisJsonPath,
			NumRoundsLimit:  numRoundsLimit,
		}

		helper.Import(db, args)
	},
}

var (
	genesisJsonPath string
	numRoundsLimit  int
	blockFileLimit  int
)

func init() {
	importCmd.Flags().StringVarP(&genesisJsonPath, "genesis", "g", "", "path to genesis.json")
	importCmd.Flags().IntVarP(&numRoundsLimit, "num-rounds-limit", "", 0, "number of rounds to process")
	importCmd.Flags().IntVarP(&blockFileLimit, "block-file-limit", "", 0, "number of block files to process (for debugging)")
}
