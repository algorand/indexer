package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/algorand/go-algorand/rpcs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/util/metrics"
)

var (
	algodDataDir     string
	algodAddr        string
	algodToken       string
	daemonServerAddr string
	noAlgod          bool
	developerMode    bool
	allowMigration   bool
	metricsMode      string
	tokenString      string
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "run indexer daemon",
	Long:  "run indexer daemon. Serve api on HTTP.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		config.BindFlags(cmd)
		err = configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			os.Exit(1)
		}

		if algodDataDir == "" {
			algodDataDir = os.Getenv("ALGORAND_DATA")
		}

		ctx, cf := context.WithCancel(context.Background())
		defer cf()
		var bot fetcher.Fetcher
		if noAlgod {
			logger.Info("algod block following disabled")
		} else if algodAddr != "" && algodToken != "" {
			bot, err = fetcher.ForNetAndToken(algodAddr, algodToken, logger)
			maybeFail(err, "fetcher setup, %v", err)
		} else if algodDataDir != "" {
			bot, err = fetcher.ForDataDir(algodDataDir, logger)
			maybeFail(err, "fetcher setup, %v", err)
		} else {
			// no algod was found
			noAlgod = true
		}
		opts := idb.IndexerDbOptions{}
		if noAlgod && !allowMigration {
			opts.ReadOnly = true
		}
		db, availableCh := indexerDbFromFlags(opts)
		if bot != nil {
			go func() {
				// Wait until the database is available.
				<-availableCh

				// Initial import if needed.
				importer.InitialImport(db, genesisJSONPath, bot.Algod(), logger)

				logger.Info("Initializing block import handler.")

				nextRound, err := db.GetNextRoundToLoad()
				maybeFail(err, "failed to get next round, %v", err)
				bot.SetNextRound(nextRound)

				cache, err := db.GetDefaultFrozen()
				maybeFail(err, "failed to get default frozen cache")

				bih := blockImporterHandler{
					imp:   importer.NewDBImporter(db),
					db:    db,
					cache: cache,
				}
				bot.AddBlockHandler(&bih)
				bot.SetContext(ctx)

				logger.Info("Starting block importer.")
				bot.Run()
				cf()
			}()
		} else {
			logger.Info("No block importer configured.")
		}

		// TODO: trap SIGTERM and call cf() to exit gracefully
		fmt.Printf("serving on %s\n", daemonServerAddr)
		logger.Infof("serving on %s", daemonServerAddr)
		api.Serve(ctx, daemonServerAddr, db, bot, logger, makeOptions())
	},
}

func init() {
	daemonCmd.Flags().StringVarP(&algodDataDir, "algod", "d", "", "path to algod data dir, or $ALGORAND_DATA")
	daemonCmd.Flags().StringVarP(&algodAddr, "algod-net", "", "", "host:port of algod")
	daemonCmd.Flags().StringVarP(&algodToken, "algod-token", "", "", "api access token for algod")
	daemonCmd.Flags().StringVarP(&genesisJSONPath, "genesis", "g", "", "path to genesis.json (defaults to genesis.json in algod data dir if that was set)")
	daemonCmd.Flags().StringVarP(&daemonServerAddr, "server", "S", ":8980", "host:port to serve API on (default :8980)")
	daemonCmd.Flags().BoolVarP(&noAlgod, "no-algod", "", false, "disable connecting to algod for block following")
	daemonCmd.Flags().StringVarP(&tokenString, "token", "t", "", "an optional auth token, when set REST calls must use this token in a bearer format, or in a 'X-Indexer-API-Token' header")
	daemonCmd.Flags().BoolVarP(&developerMode, "dev-mode", "", false, "allow performance intensive operations like searching for accounts at a particular round")
	daemonCmd.Flags().BoolVarP(&allowMigration, "allow-migration", "", false, "allow migrations to happen even when no algod connected")
	daemonCmd.Flags().StringVarP(&metricsMode, "metrics-mode", "", "OFF", "configure the /metrics endpoint to [ON, OFF, VERBOSE]")

	viper.RegisterAlias("algod", "algod-data-dir")
	viper.RegisterAlias("algod-net", "algod-address")
	viper.RegisterAlias("server", "server-address")
	viper.RegisterAlias("token", "api-token")
}

// makeOptions converts CLI options to server options
func makeOptions() (options api.ExtraOptions) {
	options.DeveloperMode = developerMode
	if tokenString != "" {
		options.Tokens = append(options.Tokens, tokenString)
	}
	switch strings.ToUpper(metricsMode) {
	case "OFF":
		options.MetricsEndpoint = false
		options.MetricsEndpointVerbose = false
	case "ON":
		options.MetricsEndpoint = true
		options.MetricsEndpointVerbose = false
	case "VERBOSE":
		options.MetricsEndpoint = true
		options.MetricsEndpointVerbose = true

	}
	return
}

type blockImporterHandler struct {
	imp   importer.Importer
	db    idb.IndexerDb
	cache map[uint64]bool
}

func (bih *blockImporterHandler) HandleBlock(block *rpcs.EncodedBlockCert) {
	start := time.Now()
	_, err := bih.imp.ImportDecodedBlock(block)
	maybeFail(err, "ImportDecodedBlock %d", block.Block.Round())
	metrics.BlockUploadTimeSeconds.Observe(time.Since(start).Seconds())
	startRound, err := bih.db.GetNextRoundToAccount()
	maybeFail(err, "failed to get next round to account")
	// During normal operation StartRound and MaxRound will be the same round.
	filter := idb.UpdateFilter{
		StartRound: startRound,
		MaxRound:   uint64(block.Block.Round()),
	}
	rounds, txns := importer.UpdateAccounting(bih.db, bih.cache, filter, logger)
	dt := time.Since(start)

	// Ignore calls that update >1 round (sneaky migration) and round 0 (which is empty)
	if rounds <= 1 && startRound != 0 {
		metrics.BlockImportTimeSeconds.Observe(dt.Seconds())
		metrics.ImportedTxnsPerBlock.Observe(float64(txns))
		metrics.ImportedRoundGauge.Set(float64(startRound))
	}

	logger.Infof("round r=%d (%d txn) imported in %s", block.Block.Round(), len(block.Block.Payset), dt.String())
}
