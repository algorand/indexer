package localledger

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	algodConfig "github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/fetcher"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/processor"
	"github.com/algorand/indexer/processor/blockprocessor"
	"github.com/algorand/indexer/util"
	log "github.com/sirupsen/logrus"
)

// RunMigration executes the migration core functionality.
func RunMigration(round uint64, opts *idb.IndexerDbOptions) error {
	logger := log.New()
	ctx, cf := context.WithCancel(context.Background())
	defer cf()
	{
		cancelCh := make(chan os.Signal, 1)
		signal.Notify(cancelCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			<-cancelCh
			logger.Errorf("Ledger migration interrupted")
			os.Exit(1)
		}()
	}

	var bot fetcher.Fetcher
	var err error
	if opts.IndexerDatadir == "" {
		return fmt.Errorf("RunMigration() err: indexer data directory missing")
	}
	if opts.AlgodDataDir != "" {
		bot, err = fetcher.ForDataDir(opts.AlgodDataDir, logger)
		if err != nil {
			return fmt.Errorf("RunMigration() err: %w", err)
		}
	} else if opts.AlgodAddr != "" && opts.AlgodToken != "" {
		bot, err = fetcher.ForNetAndToken(opts.AlgodAddr, opts.AlgodToken, logger)
		if err != nil {
			return fmt.Errorf("RunMigration() err: %w", err)
		}
	} else {
		return fmt.Errorf("RunMigration() err: unable to create algod client")
	}
	genesis, err := getGenesis(bot.Algod())
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}
	genesisBlock, err := getGenesisBlock(bot.Algod())
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}
	initState, err := util.CreateInitState(&genesis, &genesisBlock)
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}

	localLedger, err := ledger.OpenLedger(logging.NewLogger(), filepath.Join(path.Dir(opts.IndexerDatadir), "ledger"), false, initState, algodConfig.GetDefaultLocal())
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}
	defer localLedger.Close()
	if uint64(localLedger.Latest()) == round {
		return nil
	}
	bot.SetNextRound(uint64(localLedger.Latest()) + 1)
	proc, err := blockprocessor.MakeProcessor(localLedger, nil)
	if err != nil {
		return fmt.Errorf("RunMigration() err: %w", err)
	}
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
	// wait for commit to disk
	localLedger.WaitForCommit(localLedger.Latest())
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

func getGenesisBlock(client *algod.Client) (bookkeeping.Block, error) {
	data, err := client.BlockRaw(0).Do(context.Background())
	if err != nil {
		return bookkeeping.Block{}, fmt.Errorf("getGenesisBlock() client err: %w", err)
	}

	var block rpcs.EncodedBlockCert
	err = protocol.Decode(data, &block)
	if err != nil {
		return bookkeeping.Block{}, fmt.Errorf("getGenesisBlock() decode err: %w", err)
	}

	return block.Block, nil
}
