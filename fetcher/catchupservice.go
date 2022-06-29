package fetcher

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/network"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/util/metrics"
	"github.com/labstack/gommon/log"
)

type catchupService struct {
	net          network.GossipNode
	log          logging.Logger
	cfg          config.Local
	genesis      bookkeeping.Genesis
	peerSelector *peerSelector
}

// makeNodeInfo initializes nodeInfo.
func makeNodeInfo(ctx context.Context) nodeInfo {
	return nodeInfo{
		ctx: ctx,
	}
}

// nodeInfo has context and implements one service required to create a gossip node
type nodeInfo struct {
	ctx context.Context
}

type task func() basics.Round

func (n nodeInfo) IsParticipating() bool { return false }

// MakeCatchupService creats a catchup service and initialzes a gossipnode
func MakeCatchupService(genesis bookkeeping.Genesis, ctx context.Context) (serviceDr *catchupService) {
	serviceDr = &catchupService{}
	serviceDr.cfg = config.AutogenLocal
	serviceDr.genesis = genesis
	serviceDr.log = logging.NewLogger()
	nodeinfo := makeNodeInfo(ctx)
	serviceDr.net, _ = network.NewWebsocketNetwork(serviceDr.log, serviceDr.cfg, nil, genesis.ID(), genesis.Network, nodeinfo)
	return serviceDr
}

func (s *catchupService) pipelineCallback(r basics.Round, thisFetchComplete chan bool, prevFetchCompleteChan chan bool, lookbackChan chan bool, bot *fetcherImpl, ctx context.Context) func() basics.Round {
	return func() basics.Round {
		psp, _ := s.peerSelector.getNextPeer()
		for {
			start := time.Now()
			if psp.Peer == nil {
				psp, _ = s.peerSelector.getNextPeer()
			} else {
				blk, cert, err1 := s.directNetworkFetch(ctx, uint64(r), psp, psp.Peer)
				if err1 != nil {
					psp, _ = s.peerSelector.getNextPeer()
					// If context has expired.
					if ctx.Err() != nil {
						return 0
					}
				} else if uint64(blk.Round()) == uint64(r) {
					block := new(rpcs.EncodedBlockCert)
					block.Block = *blk
					block.Certificate = *cert
					select {
					case <-ctx.Done():
						return 0
					case prevFetchSuccess := <-prevFetchCompleteChan:
						if prevFetchSuccess {
							bot.blockQueue <- block
							thisFetchComplete <- true
							thisFetchComplete <- true
							bot.nextRound++
							dt := time.Since(start)
							metrics.GetAlgodRawBlockTimeSeconds.Observe(dt.Seconds())
							// If we successfully handle the block, clear out any transient error which may have occurred.
							bot.setError(nil)
							bot.failingSince = time.Time{}
							return r
						}
						thisFetchComplete <- false
						thisFetchComplete <- false
						return 0
					}
				}
			}
		}
	}
}

// parallelization attempt
func (s *catchupService) pipelinedFetch(seedLookback uint64, bot *fetcherImpl, ctx context.Context) error {
	var err error
	s.peerSelector = s.createPeerSelector(true)
	if _, err := s.peerSelector.getNextPeer(); err != nil {
		fmt.Println(err)
	}

	// pipeline fetch code
	parallelRequests := uint64(32)

	completed := make(chan basics.Round, parallelRequests)
	taskCh := make(chan task, parallelRequests)
	var wg sync.WaitGroup

	defer func() {
		close(taskCh)
		wg.Wait()
		close(completed)
	}()

	wg.Add(int(parallelRequests))
	for i := uint64(0); i < parallelRequests; i++ {
		go func() {
			defer wg.Done()
			for t := range taskCh {
				completed <- t() // This write to completed comes after a read from taskCh, so the invariant is preserved.
			}
		}()
	}

	recentReqs := make([]chan bool, 0)
	for i := 0; i < int(seedLookback); i++ {
		// the fetch result will be read at most twice (once as the lookback block and once as the prev block, so we write the result twice)
		reqComplete := make(chan bool, 2)
		reqComplete <- true
		reqComplete <- true
		recentReqs = append(recentReqs, reqComplete)
	}
	from := basics.Round(bot.nextRound)
	nextRound := from
	// loop to catchup
	for ; nextRound < from+basics.Round(parallelRequests); nextRound++ {
		currentRoundComplete := make(chan bool, 2)
		// len(taskCh) + (# pending writes to completed) increases by 1
		taskCh <- s.pipelineCallback(nextRound, currentRoundComplete, recentReqs[len(recentReqs)-1], recentReqs[len(recentReqs)-int(seedLookback)], bot, ctx)
		recentReqs = append(recentReqs[1:], currentRoundComplete)
	}
	completedRounds := make(map[basics.Round]bool)
	for {
		select {
		case round := <-completed:
			if round == 0 {
				// there was an error
				return err
			}
			completedRounds[round] = true
			// keep checking if the round has been sent to completed channel, thereby updating completedRounds
			// keep incrementing nextRound until to find a false or not existing nextRound-parallelRequests Since that-
			// -round has not been registered into completedRounds yet!
			for completedRounds[nextRound-basics.Round(parallelRequests)] {
				delete(completedRounds, nextRound)
				currentRoundComplete := make(chan bool, 2)
				// len(taskCh) + (# pending writes to completed) increases by 1
				taskCh <- s.pipelineCallback(nextRound, currentRoundComplete, recentReqs[len(recentReqs)-1], recentReqs[0], bot, ctx)
				recentReqs = append(recentReqs[1:], currentRoundComplete)
				nextRound++
			}
		case <-ctx.Done():
			return err
		}
	}
}

func (s *catchupService) createPeerSelector(pipelineFetch bool) *peerSelector {
	var peerClasses []peerClass
	if s.cfg.EnableCatchupFromArchiveServers {
		if pipelineFetch {
			if s.cfg.NetAddress != "" { // Relay node
				peerClasses = []peerClass{
					{initialRank: peerRankInitialFirstPriority, peerClass: network.PeersConnectedOut},
					{initialRank: peerRankInitialSecondPriority, peerClass: network.PeersPhonebookArchivers},
					{initialRank: peerRankInitialThirdPriority, peerClass: network.PeersPhonebookRelays},
					{initialRank: peerRankInitialFourthPriority, peerClass: network.PeersConnectedIn},
				}
			} else {
				peerClasses = []peerClass{
					{initialRank: peerRankInitialFirstPriority, peerClass: network.PeersPhonebookArchivers},
					{initialRank: peerRankInitialSecondPriority, peerClass: network.PeersConnectedOut},
					{initialRank: peerRankInitialThirdPriority, peerClass: network.PeersPhonebookRelays},
				}
			}
		} else {
			if s.cfg.NetAddress != "" { // Relay node
				peerClasses = []peerClass{
					{initialRank: peerRankInitialFirstPriority, peerClass: network.PeersConnectedOut},
					{initialRank: peerRankInitialSecondPriority, peerClass: network.PeersConnectedIn},
					{initialRank: peerRankInitialThirdPriority, peerClass: network.PeersPhonebookRelays},
					{initialRank: peerRankInitialFourthPriority, peerClass: network.PeersPhonebookArchivers},
				}
			} else {
				peerClasses = []peerClass{
					{initialRank: peerRankInitialFirstPriority, peerClass: network.PeersConnectedOut},
					{initialRank: peerRankInitialSecondPriority, peerClass: network.PeersPhonebookRelays},
					{initialRank: peerRankInitialThirdPriority, peerClass: network.PeersPhonebookArchivers},
				}
			}
		}
	} else {
		if pipelineFetch {
			if s.cfg.NetAddress != "" { // Relay node
				peerClasses = []peerClass{
					{initialRank: peerRankInitialFirstPriority, peerClass: network.PeersConnectedOut},
					{initialRank: peerRankInitialSecondPriority, peerClass: network.PeersPhonebookRelays},
					{initialRank: peerRankInitialThirdPriority, peerClass: network.PeersConnectedIn},
				}
			} else {
				peerClasses = []peerClass{
					{initialRank: peerRankInitialFirstPriority, peerClass: network.PeersConnectedOut},
					{initialRank: peerRankInitialSecondPriority, peerClass: network.PeersPhonebookRelays},
				}
			}
		} else {
			if s.cfg.NetAddress != "" { // Relay node
				peerClasses = []peerClass{
					{initialRank: peerRankInitialFirstPriority, peerClass: network.PeersConnectedOut},
					{initialRank: peerRankInitialSecondPriority, peerClass: network.PeersConnectedIn},
					{initialRank: peerRankInitialThirdPriority, peerClass: network.PeersPhonebookRelays},
				}
			} else {
				peerClasses = []peerClass{
					{initialRank: peerRankInitialFirstPriority, peerClass: network.PeersConnectedOut},
					{initialRank: peerRankInitialSecondPriority, peerClass: network.PeersPhonebookRelays},
				}
			}
		}
	}
	return makePeerSelector(s.net, peerClasses)
}

// directNetworkFetch given a block number and peer, fetches the block from the network.
func (s *catchupService) directNetworkFetch(ctx context.Context, rnd uint64, psp *peerSelectorPeer, peer network.Peer) (blk *bookkeeping.Block, cert *agreement.Certificate, err error) {
	fetch := makeUniversalBlockFetcher(s.log, s.net, s.cfg)
	blk, cert, _, err = fetch.fetchBlock(ctx, basics.Round(rnd), peer)
	// Check that the block's contents match the block header (necessary with an untrusted block because b.Hash() only hashes the header)
	if blk == nil || cert == nil {
		err = errors.New("invalid block download")
	} else if !blk.ContentsMatchHeader() && blk.Round() > 0 {
		s.peerSelector.rankPeer(psp, peerRankInvalidDownload)
		// Check if this mismatch is due to an unsupported protocol version
		if _, ok := config.Consensus[blk.BlockHeader.CurrentProtocol]; !ok {
			log.Errorf("fetchAndWrite(%v): unsupported protocol version detected: '%v'", rnd, blk.BlockHeader.CurrentProtocol)
		}
		log.Warnf("fetchAndWrite(%v): block contents do not match header (attempt %d)", rnd, 1)
		// continue // retry the fetch: add a loop over here
		err = errors.New("invalid block download")
	}
	return
}
