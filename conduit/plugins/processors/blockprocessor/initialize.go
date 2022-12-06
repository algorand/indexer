package blockprocessor

import (
	"context"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/conduit/plugins/processors/blockprocessor/internal"
	"github.com/algorand/indexer/fetcher"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
)

// InitializeLedger will initialize a ledger to the directory given by the
// IndexerDbOpts.
// nextRound - next round to process after initializing.
// catchpoint - if provided, attempt to use fast catchup.
func InitializeLedger(ctx context.Context, logger *log.Logger, nextDbRound uint64, genesis bookkeeping.Genesis, config *Config) error {
	if nextDbRound > 0 {
		if config.Catchpoint != "" {
			round, _, err := ledgercore.ParseCatchpointLabel(config.Catchpoint)
			if err != nil {
				return fmt.Errorf("InitializeLedger() label err: %w", err)
			}
			if uint64(round) >= nextDbRound {
				return fmt.Errorf("invalid catchpoint: catchpoint round %d should not be ahead of target round %d", uint64(round), nextDbRound-1)
			}
			err = InitializeLedgerFastCatchup(ctx, logger, config.Catchpoint, config.LedgerDir, genesis)
			if err != nil {
				return fmt.Errorf("InitializeLedger() fast catchup err: %w", err)
			}
		}
		err := InitializeLedgerSimple(ctx, logger, nextDbRound-1, &genesis, config)
		if err != nil {
			return fmt.Errorf("InitializeLedger() simple catchup err: %w", err)
		}
	}
	return nil
}

// InitializeLedgerFastCatchup executes the migration core functionality.
func InitializeLedgerFastCatchup(ctx context.Context, logger *log.Logger, catchpoint, dataDir string, genesis bookkeeping.Genesis) error {
	if dataDir == "" {
		return fmt.Errorf("InitializeLedgerFastCatchup() err: indexer data directory missing")
	}

	err := internal.CatchupServiceCatchup(ctx, logger, catchpoint, dataDir, genesis)
	if err != nil {
		return fmt.Errorf("InitializeLedgerFastCatchup() err: %w", err)
	}
	return nil
}

// InitializeLedgerSimple initializes a ledger with the block processor by
// sending it one block at a time and letting it update the ledger as usual.
func InitializeLedgerSimple(ctx context.Context, logger *log.Logger, round uint64, genesis *bookkeeping.Genesis, config *Config) error {
	ctx, cf := context.WithCancel(ctx)
	defer cf()
	var bot fetcher.Fetcher
	var err error
	if config.LedgerDir == "" {
		return fmt.Errorf("InitializeLedgerSimple() err: indexer data directory missing")
	}
	// create algod client
	bot, err = getFetcher(logger, config)
	if err != nil {
		return fmt.Errorf("InitializeLedgerSimple() err: %w", err)
	}
	logger.Info("initializing ledger")

	proc, err := MakeBlockProcessor(logger, genesis, round, config.LedgerDir, nil)
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}
	// ledger and db states are in sync
	if proc.NextRoundToProcess()-1 == round {
		return nil
	}
	bot.SetNextRound(proc.NextRoundToProcess())

	procHandler := MakeBlockProcessorHandlerAdapter(&proc, nil)

	handler := blockHandler(logger, round, procHandler, cf, 1*time.Second)
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

// blockHandler creates a handler complying to the fetcher block handler interface. In case of a failure it keeps
// attempting to add the block until the fetcher shuts down.
func blockHandler(logger *log.Logger, dbRound uint64, procHandler func(block *rpcs.EncodedBlockCert) error, cancel context.CancelFunc, retryDelay time.Duration) func(context.Context, *rpcs.EncodedBlockCert) error {
	return func(ctx context.Context, block *rpcs.EncodedBlockCert) error {
		for {
			err := handleBlock(logger, block, procHandler)
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

func handleBlock(logger *log.Logger, block *rpcs.EncodedBlockCert, procHandler func(block *rpcs.EncodedBlockCert) error) error {
	start := time.Now()
	err := procHandler(block)
	if err != nil {
		logger.WithError(err).Errorf(
			"block %d import failed", block.Block.Round())
		return fmt.Errorf("handleBlock() err: %w", err)
	}
	logger.Infof("Initialize Ledger: added block %d to ledger (%s)", block.Block.Round(), time.Since(start))
	return nil
}

func getFetcher(logger *log.Logger, config *Config) (fetcher.Fetcher, error) {
	var err error
	var bot fetcher.Fetcher
	if config.AlgodDataDir != "" {
		bot, err = fetcher.ForDataDir(config.AlgodDataDir, logger)
		if err != nil {
			return nil, fmt.Errorf("InitializeLedgerFastCatchup() err: %w", err)
		}
	} else if config.AlgodAddr != "" && config.AlgodToken != "" {
		bot, err = fetcher.ForNetAndToken(config.AlgodAddr, config.AlgodToken, logger)
		if err != nil {
			return nil, fmt.Errorf("InitializeLedgerFastCatchup() err: %w", err)
		}
	} else {
		return nil, fmt.Errorf("InitializeLedgerFastCatchup() err: unable to create algod client")
	}
	return bot, nil
}
