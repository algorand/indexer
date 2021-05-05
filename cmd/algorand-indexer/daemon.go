package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/algorand/go-algorand/rpcs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
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

// importTimeHistogramSeconds is used to record the block import time metric.
var importTimeHistogramSeconds = prometheus.NewSummary(
	prometheus.SummaryOpts{
		Subsystem: "indexer_daemon",
		Name:      "import_time_sec",
		Help:      "Block import and processing time in seconds.",
	})

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "run indexer daemon",
	Long:  "run indexer daemon. Serve api on HTTP.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {
		// register metric with global prometheus metrics handler
		prometheus.Register(importTimeHistogramSeconds)

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
		db := indexerDbFromFlags(opts)
		if bot != nil {
			logger.Info("Initializing block import handler.")

			nextRound, err := db.GetNextRoundToAccount()
			maybeFail(err, "failed to get next round, %v", err)
			bot.SetNextRound(nextRound)

			bih := blockImporterHandler{imp: importer.NewImporter(db)}
			bot.AddBlockHandler(&bih)
			bot.SetContext(ctx)

			go func() {
				waitForDBAvailable(db)

				// Initial import if needed.
				importer.InitialImport(db, genesisJSONPath, bot.Algod(), logger)

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

// waitForDBAvailable wait for the IndexerDb to report that it is available.
func waitForDBAvailable(db idb.IndexerDb) {
	statusInterval := 5 * time.Minute
	checkInterval := 5 * time.Second
	var now time.Time
	nextStatusTime := time.Now()
	for true {
		now = time.Now()
		health, err := db.Health()
		if err != nil {
			logger.WithError(err).Errorf("Problem fetching database health.")
			os.Exit(1)
		}

		// Exit function when the database is available
		if health.DBAvailable {
			return
		}

		// Log status periodically
		if nextStatusTime.Sub(now) <= 0 {
			logger.Info("Block importer waiting for database to become available.")
			nextStatusTime = nextStatusTime.Add(statusInterval)
		}

		time.Sleep(checkInterval)
	}
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
	imp importer.Importer
}

func (bih *blockImporterHandler) HandleBlock(block *rpcs.EncodedBlockCert) {
	start := time.Now()
	err := bih.imp.ImportBlock(block)
	maybeFail(err, "Adding block %d to database failed", block.Block.Round())
	dt := time.Since(start)

	// record metric
	importTimeHistogramSeconds.Observe(dt.Seconds())
	logger.Infof("round r=%d (%d txn) imported in %s", block.Block.Round(), len(block.Block.Payset), dt.String())
}
