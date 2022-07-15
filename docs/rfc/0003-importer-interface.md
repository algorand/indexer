# Importer Interface RFC

- Contribution Name: Importer Interface/Plugin Framework
- Implementation Owner: Ganesh Vanahalli

## Problem Statement

Users would like to choose how to fetch blocks, either by downloading from an algod rest endpoint or directly from the gossip network or read blocks from a file which was previously written by the exporter.

The current implementation of indexer only allows downloading blocks sequentially from an algod rest endpoint starting at the round number of recently written block to postgres db and users cannot choose a specific block to download.

## Proposal

Importer interface allows users to fetch any particular block either from algod rest endpoint (or directly from the network or from a file written to by the exporter plugin). Current interface also stores round number of the latest fetched block. 

### Plugin Interface
A sample interface is shown below:

```GO
package importerplugin

// Importer defines the interface for importer plugin
type Importer interface {
	// Algod returns the importer's algod client
	Algod() *algod.Client

	// Round returns the recent downloaded block
	Round() uint64

	// GetBlock fetches block of round number rnd
	GetBlock(rnd uint64) (rpcs.EncodedBlockCert, error)
}
```

### Plugin implementation
A sample implementation of importer:

``` GO
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
```

## Open Questions

- The current assumption is that importer fetches blocks one by one, upon each `GetBlock` function call from either the user/processor_plugin, should this be updated to instead fetching a range of blocks? eg: fetch blocks from 0 to 10