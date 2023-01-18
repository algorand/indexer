package types

import (
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

// TODO: should these types be defined in the SDK? In the future there could be automation to keep certain
// types in go-algorand and go-algorand-sdk synchronized.

// A ValidatedBlock represents an Block that has been successfully validated
// and can now be recorded in the ledger.
type ValidatedBlock struct {
	Block sdk.Block
	// TODO: replace when state delta endpoint is available
	Delta ledgercore.StateDelta
}

// EncodedBlockCert contains the encoded block and the corresponding encoded certificate
type EncodedBlockCert struct {
	_struct struct{} `codec:""`

	Block       sdk.Block              `codec:"block"`
	Certificate map[string]interface{} `codec:"cert"`
}

// SpecialAddresses holds addresses with nonstandard properties.
type SpecialAddresses struct {
	FeeSink     sdk.Address
	RewardsPool sdk.Address
}

// LegercoreValidatedBlock for serialization
type LegercoreValidatedBlock struct {
	Blk   bookkeeping.Block
	Delta ledgercore.StateDelta
}
