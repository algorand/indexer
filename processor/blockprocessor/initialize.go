package blockprocessor

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	algodConfig "github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/node"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/processor"
)

// InitializeLedgerSimple executes the migration core functionality.
func InitializeLedgerSimple(logger *log.Logger, round uint64, opts *idb.IndexerDbOptions) error {
	ctx, cf := context.WithCancel(context.Background())
	defer cf()
	{
		cancelCh := make(chan os.Signal, 1)
		signal.Notify(cancelCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			<-cancelCh
			logger.Errorf("Ledger migration interrupted")
			// exit process if migration is interrupted so that
			// migration state doesn't get updated in db
			os.Exit(1)
		}()
	}

	var bot fetcher.Fetcher
	var err error
	if opts.IndexerDatadir == "" {
		return fmt.Errorf("InitializeLedgerSimple() err: indexer data directory missing")
	}
	// create algod client
	bot, err = getFetcher(opts)
	if err != nil {
		return fmt.Errorf("InitializeLedgerSimple() err: %w", err)
	}
	logger.Info("initializing ledger")
	genesis, err := getGenesis(bot.Algod())
	if err != nil {
		return fmt.Errorf("InitializeLedgerSimple() err: %w", err)
	}

	proc, err := MakeProcessor(logger, &genesis, round, opts.IndexerDatadir, nil)
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}
	// ledger and db states are in sync
	if proc.NextRoundToProcess()-1 == round {
		return nil
	}
	bot.SetNextRound(proc.NextRoundToProcess())
	handler := blockHandler(round, proc, cf, 1*time.Second)
	bot.SetBlockHandler(handler)

	logger.Info("Starting ledger migration.")
	err = bot.Run(ctx)
	if err != nil {
		// If context is not expired.
		if ctx.Err() == nil {
			logger.WithError(err).Errorf("fetcher exited with error")
			os.Exit(1)
		}
	}
	return nil
}

func fullNodeCatchup(logger *log.Logger, round basics.Round, catchpoint, dataDir string, genesis bookkeeping.Genesis) error {
	wrappedLogger := logging.NewWrappedLogger(logger)

	node, err := node.MakeFull(
		wrappedLogger,
		dataDir,
		algodConfig.AutogenLocal,
		nil,
		genesis)
	if err != nil {
		return err
	}
	// remove node directory after when exiting fast catchup mode
	defer os.RemoveAll(filepath.Join(dataDir, genesis.ID()))
	node.Start()
	time.Sleep(5 * time.Second)
	logger.Info("algod node running")
	status, _ := node.Status()
	node.StartCatchup(catchpoint)
	//  If the node isn't in fast catchup mode, catchpoint will be empty.
	logger.Infof("Running fast catchup using catchpoint %s", catchpoint)
	for status.LastRound < round {
		time.Sleep(2 * time.Second)
		status, _ = node.Status()
		if status.CatchpointCatchupTotalBlocks > 0 {
			logger.Debugf("current round %d ", status.LastRound)
		}
	}
	logger.Info("fast catchup completed")
	node.Stop()
	logger.Info("algod node stopped")
	return nil
}

// InitializeLedgerFastCatchup executes the migration core functionality.
func InitializeLedgerFastCatchup(logger *log.Logger, catchpoint, dataDir string, genesis bookkeeping.Genesis) error {
	if dataDir == "" {
		return fmt.Errorf("InitializeLedgerFastCatchup() err: indexer data directory missing")
	}
	// catchpoint round
	round, _, err := ledgercore.ParseCatchpointLabel(catchpoint)
	if err != nil {
		return fmt.Errorf("InitializeLedgerFastCatchup() err: %w", err)
	}

	// TODO: switch to catchup service catchup.
	//err = internal.CatchupServiceCatchup(logger, round, catchpoint, dataDir, genesis)
	err = fullNodeCatchup(logger, round, catchpoint, dataDir, genesis)
	if err != nil {
		return fmt.Errorf("fullNodeCatchup() err: %w", err)
	}

	// move ledger to indexer directory
	ledgerFiles := []string{
		"ledger.block.sqlite",
		"ledger.block.sqlite-shm",
		"ledger.block.sqlite-wal",
		"ledger.tracker.sqlite",
		"ledger.tracker.sqlite-shm",
		"ledger.tracker.sqlite-wal",
	}
	for _, f := range ledgerFiles {
		err = os.Rename(filepath.Join(dataDir, genesis.ID(), f), filepath.Join(dataDir, f))
		if err != nil {
			return fmt.Errorf("InitializeLedgerFastCatchup() err: %w", err)
		}
	}
	return nil
}

// blockHandler creates a handler complying to the fetcher block handler interface. In case of a failure it keeps
// attempting to add the block until the fetcher shuts down.
func blockHandler(dbRound uint64, proc processor.Processor, cancel context.CancelFunc, retryDelay time.Duration) func(context.Context, *rpcs.EncodedBlockCert) error {
	return func(ctx context.Context, block *rpcs.EncodedBlockCert) error {
		for {
			err := handleBlock(block, proc)
			if err == nil {
				if uint64(block.Block.Round()) == dbRound {
					// migration completes
					cancel()
				} else {
					// return on success.
					return nil
				}
			}
			// Delay or terminate before next attempt.
			select {
			case <-ctx.Done():
				return err
			case <-time.After(retryDelay):
				// noop
			}
		}
	}
}

func handleBlock(block *rpcs.EncodedBlockCert, proc processor.Processor) error {
	logger := log.New()
	err := proc.Process(block)
	if err != nil {
		logger.WithError(err).Errorf(
			"block %d import failed", block.Block.Round())
		return fmt.Errorf("handleBlock() err: %w", err)
	}
	return nil
}
func getGenesis(client *algod.Client) (bookkeeping.Genesis, error) {
	data, err := client.GetGenesis().Do(context.Background())
	if err != nil {
		return bookkeeping.Genesis{}, fmt.Errorf("getGenesis() client err: %w", err)
	}

	var res bookkeeping.Genesis
	err = protocol.DecodeJSON([]byte(data), &res)
	if err != nil {
		return bookkeeping.Genesis{}, fmt.Errorf("getGenesis() decode err: %w", err)
	}

	return res, nil
}
func getFetcher(opts *idb.IndexerDbOptions) (fetcher.Fetcher, error) {
	logger := log.New()
	var err error
	var bot fetcher.Fetcher
	if opts.AlgodDataDir != "" {
		bot, err = fetcher.ForDataDir(opts.AlgodDataDir, logger)
		if err != nil {
			return nil, fmt.Errorf("InitializeLedgerFastCatchup() err: %w", err)
		}
	} else if opts.AlgodAddr != "" && opts.AlgodToken != "" {
		bot, err = fetcher.ForNetAndToken(opts.AlgodAddr, opts.AlgodToken, logger)
		if err != nil {
			return nil, fmt.Errorf("InitializeLedgerFastCatchup() err: %w", err)
		}
	} else {
		return nil, fmt.Errorf("InitializeLedgerFastCatchup() err: unable to create algod client")
	}
	return bot, nil
}
