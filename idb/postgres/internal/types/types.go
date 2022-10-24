package types

import (
	"github.com/algorand/go-algorand/crypto"
)

// ImportState encodes an import round counter.
type ImportState struct {
	NextRoundToAccount uint64 `codec:"next_account_round"`
}

// MigrationState is metadata used by the postgres migrations.
type MigrationState struct {
	NextMigration int `json:"next"`

	// The following are deprecated.
	NextRound    int64  `json:"round,omitempty"`
	NextAssetID  int64  `json:"assetid,omitempty"`
	PointerRound *int64 `json:"pointerRound,omitempty"`
	PointerIntra *int64 `json:"pointerIntra,omitempty"`

	// Note: a generic "data" field here could be a good way to deal with this growing over time.
	//       It would require a mechanism to clear the data field between migrations to avoid using migration data
	//       from the previous migration.
}

// NetworkState encodes network metastate.
type NetworkState struct {
	GenesisHash crypto.Digest `codec:"genesis-hash"`
}

// DeleteStatus encodes pruned metastate.
type DeleteStatus struct {
	LastPruned  string `codec:"last_pruned"`
	OldestRound uint64 `codec:"oldest_txn_round"`
}
