package types

import (
	sdk "github.com/algorand/go-algorand-sdk/types"
	models "github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
)

// TODO: should these types be defined in the SDK? In the future there could be automation to keep certain
// types in go-algorand and go-algorand-sdk synchronized.

// A ValidatedBlock represents an Block that has been successfully validated
// and can now be recorded in the ledger.
type ValidatedBlock struct {
	Block sdk.Block
	Delta models.LedgerStateDelta
}

type Certificate map[string]interface{}

// EncodedBlockCert contains the encoded block and the corresponding encoded certificate
type EncodedBlockCert struct {
	_struct struct{} `codec:""`

	Block       sdk.Block              `codec:"block"`
	Certificate Certificate `codec:"cert"`
}

// SpecialAddresses holds addresses with nonstandard properties.
type SpecialAddresses struct {
	FeeSink     sdk.Address
	RewardsPool sdk.Address
}
