package types

import (
	"github.com/algorand/go-algorand-sdk/types"
)

// SpecialAddresses holds addresses with nonstandard properties.
type SpecialAddresses struct {
	FeeSink     types.Address
	RewardsPool types.Address
}
