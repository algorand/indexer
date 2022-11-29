package helpers

import (
	"encoding/base64"
	"fmt"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-stateproof-verification/stateproof"
	"github.com/algorand/indexer/types"
)

// ConvertParams converts basics.AssetParams to sdk.AssetParams
func ConvertParams(params basics.AssetParams) sdk.AssetParams {
	return sdk.AssetParams{
		Total:         params.Total,
		Decimals:      params.Decimals,
		DefaultFrozen: params.DefaultFrozen,
		UnitName:      params.UnitName,
		AssetName:     params.AssetName,
		URL:           params.URL,
		MetadataHash:  params.MetadataHash,
		Manager:       sdk.Address(params.Manager),
		Reserve:       sdk.Address(params.Reserve),
		Freeze:        sdk.Address(params.Freeze),
		Clawback:      sdk.Address(params.Clawback),
	}
}

// ConvertValidatedBlock converts ledgercore.ValidatedBlock to types.ValidatedBlock
func ConvertValidatedBlock(vb ledgercore.ValidatedBlock) (types.ValidatedBlock, error) {
	var ret types.ValidatedBlock
	b64data := base64.StdEncoding.EncodeToString(msgpack.Encode(vb.Block()))
	err := ret.Block.FromBase64String(b64data)
	if err != nil {
		return ret, fmt.Errorf("ConvertValidatedBlock err: %v", err)
	}
	ret.Delta = vb.Delta()
	return ret, nil
}

// IndexerTxn overrides StateProof
type IndexerTxn struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`
	sdk.Transaction
	// StateProof override with concrete type
	StateProof stateproof.StateProof `codec:"sp"`
}
