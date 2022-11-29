package internal

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/util"

	"github.com/algorand/go-algorand/catchup"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/network"
)

// Delay is the time to wait for catchup service startup
var Delay = 5 * time.Second

// makeNodeProvider initializes the node provider.
func makeNodeProvider(ctx context.Context) nodeProvider {
	return nodeProvider{
		ctx: ctx,
	}
}

// nodeProvider implements two services required to start the catchpoint catchup service.
type nodeProvider struct {
	ctx context.Context
}

// IsParticipating is from the NodeInfo interface used by the WebsocketNetwork
// in order to determine which gossip messages to subscribe to.
func (n nodeProvider) IsParticipating() bool {
	return false
}

// SetCatchpointCatchupMode is a callback provided by the catchpoint catchup
// service which notifies listeners that the catchup mode is changing. The
// context channel is used to to stop the catchpoint service, or if the channel
// is closed, indicate that the listener is stopping.
func (n nodeProvider) SetCatchpointCatchupMode(enabled bool) (newContextCh <-chan context.Context) {
	ch := make(chan context.Context)
	go func() {
		if enabled {
			ch <- n.ctx
		}
	}()
	return ch
}

// CatchupServiceCatchup initializes a ledger using the catchup service.
func CatchupServiceCatchup(ctx context.Context, logger *log.Logger, catchpoint, dataDir string, genesis bookkeeping.Genesis) error {
	if catchpoint == "" {
		return fmt.Errorf("CatchupServiceCatchup() catchpoint missing")
	}
	catchpointRound, _, err := ledgercore.ParseCatchpointLabel(catchpoint)
	if err != nil {
		return fmt.Errorf("CatchupServiceCatchup() invalid catchpoint err: %w", err)
	}

	logger.Infof("Starting catchup service with catchpoint: %s", catchpoint)

	start := time.Now()
	cfg := config.AutogenLocal

	node := makeNodeProvider(ctx)
	l, err := util.MakeLedger(logger, false, &genesis, dataDir)
	if err != nil {
		return fmt.Errorf("CatchupServiceCatchup() MakeLedger err: %w", err)
	}
	defer l.Close()

	// If the ledger is beyond the catchpoint round, we're done. Return with no error.
	if l.Latest() >= catchpointRound {
		return nil
	}

	wrappedLogger := logging.NewWrappedLogger(logger)
	net, err := network.NewWebsocketNetwork(wrappedLogger, cfg, nil, genesis.ID(), genesis.Network, node)
	if err != nil {
		return fmt.Errorf("CatchupServiceCatchup() NewWebsocketNetwork err: %w", err)
	}
	net.Start()
	defer func() {
		net.ClearHandlers()
		net.Stop()
	}()

	// TODO: Can use MakeResumedCatchpointCatchupService if ledger exists.
	//       Without this ledger is re-initialized instead of resumed on restart.
	service, err := catchup.MakeNewCatchpointCatchupService(
		catchpoint,
		node,
		wrappedLogger,
		net,
		ledger.MakeCatchpointCatchupAccessor(l, wrappedLogger),
		cfg,
	)
	if err != nil {
		return fmt.Errorf("CatchupServiceCatchup() MakeNewCatchpointCatchupService err: %w", err)
	}

	select {
	case <-time.After(Delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	service.Start(ctx)
	defer service.Stop()

	// Report progress periodically while waiting for catchup to complete
	running := true
	for running {
		select {
		case <-time.After(Delay):
		case <-ctx.Done():
			return ctx.Err()
		}
		stats := service.GetStatistics()
		running = stats.TotalBlocks == 0 || stats.TotalBlocks != stats.VerifiedBlocks

		switch {
		case !running:
			break
		case stats.VerifiedBlocks > 0:
			logger.Infof("catchup phase 4 of 4 (Verified Blocks): %d / %d", stats.VerifiedBlocks, stats.TotalBlocks)
		case stats.AcquiredBlocks > 0:
			logger.Infof("catchup phase 3 of 4 (Aquired Blocks): %d / %d", stats.AcquiredBlocks, stats.TotalBlocks)
		case stats.VerifiedAccounts > 0:
			logger.Infof("catchup phase 2 of 4 (Verified Accounts):  %d / %d", stats.VerifiedAccounts, stats.TotalAccounts)
		case stats.ProcessedAccounts > 0:
			logger.Infof("catchup phase 1 of 4 (Processed Accounts): %d / %d", stats.ProcessedAccounts, stats.TotalAccounts)
		}
	}

	logger.Infof("Catchup finished in %s", time.Since(start))
	l.WaitForCommit(l.Latest())
	return nil
}
