package importerPlugin

import (
	"strings"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/rpcs"
	log "github.com/sirupsen/logrus"
)

// Importer defines the interface for importer plugin
type Importer interface {
	// Algod returns the importer's algod client
	Algod() *algod.Client

	// Round returns the recent downloaded block
	Round() uint64

	// GetBlock fetches block of round number rnd
	GetBlock(rnd uint64) (rpcs.EncodedBlockCert, error)
}

type importerImpl struct {
	aclient   *algod.Client
	lastRound uint64
	log       *log.Logger

	// To determine if the importer should fetch blocks from algod or gossip network
	algodFetch bool
}

// Algod returns the importer's algod client
func (bot *importerImpl) Algod() *algod.Client {
	return bot.aclient
}

// Round returns the recent downloaded block
func (bot *importerImpl) Round() uint64 {
	return bot.lastRound
}

// Getblock takes the round number as an input and downloads that specific block, updates the 'lastRound' local variable and returns
// an encodedBlockCert struct consisting of Block and Certificate
func (bot *importerImpl) GetBlock(rnd uint64) (blk rpcs.EncodedBlockCert, err error) {
	/* code to fetch block number rnd */

	// after fetching the block, update lastround
	bot.lastRound = rnd
	return
}

// RegisterImporter will be called during initialization by the user/processor_plugin, to initialize necessary connections
// Used for initializing algod client, which is the last block number that processor_plugin has (used to syncup with importer)
// Can also be used for initializing gossip node in the case when importer fetches blocks directly from the network
func RegisterImporter(netaddr, token string, log *log.Logger, algodFetch bool, lastRound uint64) (bot Importer, err error) {
	// for downloading blocks directly from the gossip network
	if !algodFetch {
		/* code to initialize gossip network block fetcher */
		bot = &importerImpl{log: log, lastRound: lastRound, algodFetch: algodFetch}
		return
	}

	// for downloading blocks from algod rest endpoint
	var client *algod.Client
	if !strings.HasPrefix(netaddr, "http") {
		netaddr = "http://" + netaddr
	}
	client, err = algod.MakeClient(netaddr, token)
	if err != nil {
		return
	}
	bot = &importerImpl{aclient: client, log: log, lastRound: lastRound, algodFetch: algodFetch}
	return
}
