package types

import (
	sdk "github.com/algorand/go-algorand-sdk/types"
)

type ValidatedBlock struct {
	Block sdk.Block
	Delta sdk.StateDelta
}

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
