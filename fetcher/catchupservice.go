package fetcher

import (
	"context"
	"errors"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/network"
	"github.com/labstack/gommon/log"
)

type catchupService struct {
	net          network.GossipNode
	log          logging.Logger
	cfg          config.Local
	genesis      bookkeeping.Genesis
	peerSelector *peerSelector
}

// makeNodeInfo initializes the node provider.
func makeNodeInfo(ctx context.Context) nodeInfo {
	return nodeInfo{
		ctx: ctx,
	}
}

// nodeInfo implements two services required to start the catchpoint catchup service.
type nodeInfo struct {
	ctx context.Context
}

func (n nodeInfo) IsParticipating() bool { return false }

func MakeCatchupService(genesis bookkeeping.Genesis, ctx context.Context) (serviceDr *catchupService) {
	serviceDr = &catchupService{}
	serviceDr.cfg = config.AutogenLocal
	serviceDr.genesis = genesis
	serviceDr.log = logging.NewLogger()
	nodeinfo := makeNodeInfo(ctx)
	serviceDr.net, _ = network.NewWebsocketNetwork(serviceDr.log, serviceDr.cfg, nil, genesis.ID(), genesis.Network, nodeinfo)
	return serviceDr
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
