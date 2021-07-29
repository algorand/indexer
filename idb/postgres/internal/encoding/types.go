package encoding

import (
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/types"
)

type assetParams struct {
	sdk_types.AssetParams
	UnitNameBytes  []byte `codec:"un64,omitempty"`
	AssetNameBytes []byte `codec:"an64,omitempty"`
	URLBytes       []byte `codec:"au64,omitempty"`
}

type transaction struct {
	types.Transaction
	AssetParamsOverride assetParams `codec:"apar,omitempty"`
}

type signedTxnWithAD struct {
	types.SignedTxnWithAD
	TxnOverride transaction `codec:"txn,omitempty"`
}
