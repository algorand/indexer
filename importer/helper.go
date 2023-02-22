package importer

import (
	"errors"
	"fmt"

	"github.com/algorand/indexer/idb"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// EnsureInitialImport imports the genesis block if needed. Returns true if the initial import occurred.
func EnsureInitialImport(db idb.IndexerDb, genesis sdk.Genesis) (bool, error) {
	_, err := db.GetNextRoundToAccount()
	// Exit immediately or crash if we don't see ErrorNotInitialized.
	if err != idb.ErrorNotInitialized {
		if err != nil {
			return false, fmt.Errorf("getting import state, %v", err)
		}
		err = checkGenesisHash(db, genesis.Hash())
		if err != nil {
			return false, err
		}
		return false, nil
	}

	// Import genesis file from file or algod.
	err = db.LoadGenesis(genesis)
	if err != nil {
		return false, fmt.Errorf("could not load genesis json, %v", err)
	}
	return true, nil
}

func checkGenesisHash(db idb.IndexerDb, gh sdk.Digest) error {
	network, err := db.GetNetworkState()
	if errors.Is(err, idb.ErrorNotInitialized) {
		err = db.SetNetworkState(gh)
		if err != nil {
			return fmt.Errorf("error setting network state %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("unable to fetch network state from db %w", err)
	}
	if network.GenesisHash != gh {
		return fmt.Errorf("genesis hash not matching")
	}
	return nil
}
