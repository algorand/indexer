package encoding

import (
	"github.com/algorand/indexer/types"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	sdk_types "github.com/algorand/go-algorand-sdk/types"
)

// DecodeJSON decodes a json string.
var DecodeJSON = json.Decode

func unconvertAssetParams(params assetParams) sdk_types.AssetParams {
	res := params.AssetParams
	if len(res.AssetName) == 0 {
		res.AssetName = string(params.AssetNameBytes)
	}
	if len(res.UnitName) == 0 {
		res.UnitName = string(params.UnitNameBytes)
	}
	if len(res.URL) == 0 {
		res.URL = string(params.URLBytes)
	}
	return res
}

// DecodeAssetParams converts the postgres assetParams object into AssetParams.
func DecodeAssetParams(data []byte) (sdk_types.AssetParams, error) {
	var params assetParams
	err := DecodeJSON(data, &params)
	if err != nil {
		return sdk_types.AssetParams{}, err
	}

	return unconvertAssetParams(params), nil
}

func unconvertTransaction(txn transaction) types.Transaction {
	res := txn.Transaction
	res.AssetParams = unconvertAssetParams(txn.AssetParamsOverride)
	return res
}

func unconvertSignedTxnWithAD(stxn signedTxnWithAD) types.SignedTxnWithAD {
	res := stxn.SignedTxnWithAD
	res.Txn = unconvertTransaction(stxn.TxnOverride)
	return res
}

// DecodeSignedTxnWithAD converts the postgres signedTxnWithAD object into SignedTxnWithAD.
func DecodeSignedTxnWithAD(data []byte) (types.SignedTxnWithAD, error) {
	var stxn signedTxnWithAD
	err := DecodeJSON(data, &stxn)
	if err != nil {
		return types.SignedTxnWithAD{}, err
	}

	return unconvertSignedTxnWithAD(stxn), nil
}

func unconvertAssetParamsArray(paramsArr []assetParams) []sdk_types.AssetParams {
	if paramsArr == nil {
		return nil
	}

	res := make([]sdk_types.AssetParams, 0, len(paramsArr))
	for _, params := range paramsArr {
		res = append(res, unconvertAssetParams(params))
	}
	return res
}

// DecodeAssetParamsArray converts an array of the postgres assetParams object into AssetParams.
func DecodeAssetParamsArray(data []byte) ([]sdk_types.AssetParams, error) {
	var paramsArr []assetParams
	err := DecodeJSON(data, &paramsArr)
	if err != nil {
		return nil, err
	}

	return unconvertAssetParamsArray(paramsArr), nil
}
