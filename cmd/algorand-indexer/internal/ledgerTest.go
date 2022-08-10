package internal

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/algorand/go-algorand/ledger/ledgercore"

	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/processor/blockprocessor"
	"github.com/algorand/indexer/util"
)

// LedgerTestCmd is the account validator utility.
var LedgerTestCmd *cobra.Command

type params struct {
	indexerDataDir string
	catchpoint     string
	algodAddr      string
	algodToken     string
}

func init() {
	var (
		config params
	)

	LedgerTestCmd = &cobra.Command{
		Use:   "ledger-test",
		Short: "ledger-test",
		Long:  "Initialize the ledger without a database.",
		Run: func(cmd *cobra.Command, _ []string) {
			run(config)
		},
	}

	LedgerTestCmd.Flags().StringVar(&config.indexerDataDir, "data-dir", "", "Indexer data directory.")
	LedgerTestCmd.Flags().StringVar(&config.catchpoint, "catchpoint", "", "Catchpoint to use for initializing.")
	LedgerTestCmd.Flags().StringVar(&config.algodAddr, "algod-net", "", "host:port of algod")
	LedgerTestCmd.Flags().StringVar(&config.algodToken, "algod-token", "", "api access token for algod")

	LedgerTestCmd.MarkFlagRequired("algod-net")
	LedgerTestCmd.MarkFlagRequired("algod-token")
	LedgerTestCmd.MarkFlagRequired("data-dir")
	LedgerTestCmd.MarkFlagRequired("catchpoint")
}

func run(config params) {
	logger := log.New()
	logger.SetFormatter(&log.JSONFormatter{
		DisableHTMLEscape: true,
	})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(log.InfoLevel)

	// Create indexer data dir if it does not exist.
	_, err := os.Stat(config.indexerDataDir)
	if os.IsNotExist(err) {
		err = os.Mkdir(config.indexerDataDir, 0755)
		util.MaybeFail(err, "failed to create data directory: %s", err)
	} else {
		util.MaybeFail(err, "indexer data directory error: %s", err)
	}

	// Grab round to make sure we provide the correct target for fast catchup.
	nextDBRound, _, err := ledgercore.ParseCatchpointLabel(config.catchpoint)
	util.MaybeFail(err, "Unable to parse catchpoint: %s", err)

	// Lookup genesis
	bot, err := fetcher.ForNetAndToken(config.algodAddr, config.algodToken, logger)
	util.MaybeFail(err, "fetcher setup, %v", err)
	genesisString, err := bot.Algod().GetGenesis().Do(context.Background())
	util.MaybeFail(err, "unable to fetch genesis from algod")
	genesis, err := util.ReadGenesis(strings.NewReader(genesisString))
	util.MaybeFail(err, "Error reading genesis file")

	// Start block processor
	opts := idb.IndexerDbOptions{
		IndexerDatadir: config.indexerDataDir,
		AlgodAddr:      config.algodAddr,
		AlgodToken:     config.algodToken,
	}
	proc, err := blockprocessor.MakeProcessorWithLedgerInit(context.Background(), logger, config.catchpoint, &genesis, uint64(nextDBRound)+1, opts, handler)
	if err != nil {
		util.MaybeFail(err, "blockprocessor.MakeProcessor() err %v", err)
	}

	fmt.Printf("Started! Next round: %d\n", proc.NextRoundToProcess())
}

func handler(block *ledgercore.ValidatedBlock) error {
	return nil
}
