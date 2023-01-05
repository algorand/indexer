package helpers

import (
	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand/data/basics"
)

// TODO: remove this file once all types have been converted to sdk types.

func ConvertParamsTemp(params models.AssetParams) sdk.AssetParams {

	metaDataHash := [32]byte{}
	copy(metaDataHash[:],params.MetadataHash[:32])

	manager := [32]byte{}
	copy(manager[:], params.Manager[:32])

	reserve := [32]byte{}
	copy(reserve[:], params.Reserve[:32])

	freeze := [32]byte{}
	copy(freeze[:], params.Freeze[:32])

	clawback := [32]byte{}
	copy(clawback[:], params.Clawback[:32])

	return sdk.AssetParams{
		Total:         params.Total,
		Decimals:      uint32(params.Decimals),
		DefaultFrozen: params.DefaultFrozen,
		UnitName:      params.UnitName,
		AssetName:     params.Name,
		URL:           params.Url,
		MetadataHash:  metaDataHash,
		Manager:       manager,
		Reserve:       reserve,
		Freeze:        freeze,
		Clawback:      clawback,
	}
}

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
