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

	// Getblock takes the round number as an input and downloads that specific block, updates the 'lastRound' local variable and returns an encodedBlockCert struct consisting of Block and Certificate
	GetBlock(rnd uint64) (rpcs.EncodedBlockCert, error)
}
```

### Plugin implementation
A sample implementation of importer:

``` GO
type importerImpl struct {
	// aclient is the algod client used to fetch block
	aclient   *algod.Client

	// lastRound is the last successfully fetched block
	lastRound uint64

	// log.Logger given by the user through RegisterImporter method
	log       *log.Logger

	// To determine if the importer should fetch blocks from algod or gossip network or from a file
	fetchMethod string
}

// RegisterImporter will be called during initialization by the user/processor_plugin, to initialize necessary connections
// Used for initializing algod client, which is the last block number that processor_plugin has (used to syncup with importer)
// Can also be used for initializing gossip node in the case when importer fetches blocks directly from the network
func RegisterImporter(netaddr, token string, log *log.Logger, fetchMethod string, lastRound uint64) (bot Importer, err error) {	
}
```

## Open Questions

- The current assumption is that importer fetches blocks one by one, upon each `GetBlock` function call from either the user/processor_plugin, should this be updated to instead fetching a range of blocks? eg: fetch blocks from 0 to 10