package types

import (
	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

// A ValidatedBlock represents an Block that has been successfully validated
// and can now be recorded in the ledger.
type ValidatedBlock struct {
	Block sdk.Block
	// todo: replace when statedelta endpoint is available
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
