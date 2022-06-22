package internal

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/go-algorand/catchup"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/network"

	"github.com/algorand/indexer/util"
)

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

func CatchupServiceCatchup(logger *log.Logger, round basics.Round, catchpoint, dataDir string, genesis bookkeeping.Genesis) error {
	start := time.Now()
	ctx := context.Background()
	cfg := config.AutogenLocal

	node := makeNodeProvider(ctx)
	//l, err := MakeLedger(rootDir, genesis)
	l, err := util.MakeLedger(logger, &genesis, dataDir)
	if err != nil {
		return fmt.Errorf("CatchupServiceCatchup() MakeLedger err: %w", err)
	}

	wrappedLogger := logging.NewLogger()
	// TODO: Use new wrapped logger
	//wrappedLogger := logging.NewWrappedLogger(logger)
	p2pNode, err := network.NewWebsocketNetwork(wrappedLogger, cfg, nil, genesis.ID(), genesis.Network, node)
	if err != nil {
		return fmt.Errorf("CatchupServiceCatchup() NewWebsocketNetwork err: %w", err)
	}
	// TODO: Do we need to implement the peer prioritization interface?
	//p2pNode.SetPrioScheme(node)
	p2pNode.Start()

	service, err := catchup.MakeNewCatchpointCatchupService(
		catchpoint,
		node,
		wrappedLogger,
		p2pNode,
		l,
		cfg,
	)
	if err != nil {
		return fmt.Errorf("CatchupServiceCatchup() MakeNewCatchpointCatchupService err: %w", err)
	}

	time.Sleep(5 * time.Second)
	service.Start(ctx)

	running := true
	for running {
		//time.Sleep(5 * time.Second)
		stats := service.GetStatistics()
		/*
			fmt.Println("==========================")
			fmt.Printf("Time:               %s\n", time.Now())
			fmt.Printf("Ledger:             %d\n", l.Latest())
			fmt.Printf("Processed Accounts: %d / %d\n", stats.ProcessedAccounts, stats.TotalAccounts)
			fmt.Printf("Verified Accounts:  %d / %d\n", stats.VerifiedAccounts, stats.TotalAccounts)
			fmt.Printf("Aquired Blocks:     %d / %d\n", stats.AcquiredBlocks, stats.TotalBlocks)
			fmt.Printf("Verified Blocks:    %d / %d\n", stats.VerifiedBlocks, stats.TotalBlocks)
		*/
		running = stats.TotalBlocks == 0 || stats.TotalBlocks != stats.VerifiedBlocks
	}

	fmt.Printf("Done after: %s\n", time.Since(start))
	writing := time.Now()
	l.WaitForCommit(l.Latest())
	fmt.Printf("Done after: %s\n", time.Since(start))
	fmt.Printf("Write duration: %s\n", time.Since(writing))
	return nil
}
