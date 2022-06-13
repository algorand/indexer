package localledger

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	algodConfig "github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/node"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/processor"
	"github.com/algorand/indexer/processor/blockprocessor"
	log "github.com/sirupsen/logrus"
)

// RunMigrationSimple executes the migration core functionality.
func RunMigrationSimple(round uint64, opts *idb.IndexerDbOptions) error {
	logger := log.New()
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
		return fmt.Errorf("RunMigrationSimple() err: indexer data directory missing")
	}
	// create algod client
	bot, err = getFetcher(opts)
	if err != nil {
		return fmt.Errorf("RunMigrationFastCatchup() err: %w", err)
	}
	logger.Info("initializing ledger")
	genesis, err := getGenesis(bot.Algod())
	if err != nil {
		return fmt.Errorf("RunMigrationSimple() err: %w", err)
	}

	genesisBlock, err := getGenesisBlock(&genesis)
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}

	proc, err := blockprocessor.MakeProcessor(&genesis, genesisBlock, opts.IndexerDatadir, nil)
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
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

// RunMigrationFastCatchup executes the migration core functionality.
func RunMigrationFastCatchup(logger logging.Logger, catchpoint string, opts *idb.IndexerDbOptions) error {
	if opts.IndexerDatadir == "" {
		return fmt.Errorf("RunMigrationFastCatchup() err: indexer data directory missing")
	}
	// catchpoint round
	catchpointAr := strings.Split(catchpoint, "#")
	round, err := strconv.ParseUint(string(catchpointAr[0]), 10, 64)
	if err != nil {
		return fmt.Errorf("RunMigrationFastCatchup() err: %w", err)
	}
	// create algod client
	bot, err := getFetcher(opts)
	if err != nil {
		return fmt.Errorf("RunMigrationFastCatchup() err: %w", err)
	}
	genesis, err := getGenesis(bot.Algod())
	if err != nil {
		return fmt.Errorf("RunMigrationFastCatchup() err: %w", err)
	}
	node, err := node.MakeFull(
		logging.NewLogger(),
		opts.IndexerDatadir,
		algodConfig.AutogenLocal,
		nil,
		genesis)
	// remove node directory after when exiting fast catchup mode
	//defer os.RemoveAll(filepath.Join(opts.IndexerDatadir, genesis.ID()))
	node.Start()
	time.Sleep(5 * time.Second)
	logger.Info("algod node running")
	status, err := node.Status()
	node.StartCatchup(catchpoint)
	//  If the node isn't in fast catchup mode, catchpoint will be empty.
	logger.Infof("Running fast catchup using catchpoint %s", catchpoint)
	for uint64(status.LastRound) < round {
		time.Sleep(2 * time.Second)
		status, err = node.Status()
		if status.CatchpointCatchupTotalBlocks > 0 {
			logger.Debugf("current round %d ", status.LastRound)
		}
	}
	logger.Info("fast catchup completed")
	node.Stop()
	logger.Info("algod node stopped")
	// move ledger to indexer directory
	ledgerFiles := []string{
		"ledger.block.sqlite",
		"ledger.tracker.sqlite",
	}
	for _, f := range ledgerFiles {
		err = os.Rename(filepath.Join(opts.IndexerDatadir, genesis.ID(), f), filepath.Join(opts.IndexerDatadir, f))
		if err != nil {
			return fmt.Errorf("RunMigrationFastCatchup() err: %w", err)
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
				break
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

func getGenesisBlock(genesis *bookkeeping.Genesis) (*bookkeeping.Block, error) {
	accounts := make(map[basics.Address]basics.AccountData)
	for _, alloc := range genesis.Allocation {
		addr, err := basics.UnmarshalChecksumAddress(alloc.Address)
		if err != nil {
			return nil, fmt.Errorf("getGenesisBlock() err: %w", err)
		}
		accounts[addr] = alloc.State
	}

	feeSink, err := basics.UnmarshalChecksumAddress(genesis.FeeSink)
	if err != nil {
		return nil, fmt.Errorf("getGenesisBlock() err: %w", err)
	}
	rewardsPool, err := basics.UnmarshalChecksumAddress(genesis.RewardsPool)
	if err != nil {
		return nil, fmt.Errorf("getGenesisBlock() err: %w", err)
	}
	genesisBal := bookkeeping.MakeGenesisBalances(accounts, feeSink, rewardsPool)
	genesisBlock, err := bookkeeping.MakeGenesisBlock(genesis.Proto, genesisBal, genesis.ID(), crypto.HashObj(genesis))
	return &genesisBlock, nil
}

func getFetcher(opts *idb.IndexerDbOptions) (fetcher.Fetcher, error) {
	logger := log.New()
	var err error
	var bot fetcher.Fetcher
	if opts.AlgodDataDir != "" {
		bot, err = fetcher.ForDataDir(opts.AlgodDataDir, logger)
		if err != nil {
			return nil, fmt.Errorf("RunMigrationFastCatchup() err: %w", err)
		}
	} else if opts.AlgodAddr != "" && opts.AlgodToken != "" {
		bot, err = fetcher.ForNetAndToken(opts.AlgodAddr, opts.AlgodToken, logger)
		if err != nil {
			return nil, fmt.Errorf("RunMigrationFastCatchup() err: %w", err)
		}
	} else {
		return nil, fmt.Errorf("RunMigrationFastCatchup() err: unable to create algod client")
	}
	return bot, nil
}
