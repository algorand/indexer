package encoding

import sdk_types "github.com/algorand/go-algorand-sdk/types"

type assetParams struct {
	sdk_types.AssetParams
	UnitNameBytes  []byte `codec:"un64,omitempty"`
	AssetNameBytes []byte `codec:"an64,omitempty"`
	URLBytes       []byte `codec:"au64,omitempty"`
}
