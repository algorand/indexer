package encoding

import (
	"encoding/base64"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/types"
)

// DecodeJSON is a function that decodes json.
var DecodeJSON = protocol.DecodeJSON

func decodeBase64(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

// DecodeBlockHeader decodes block header from json.
func DecodeBlockHeader(data []byte) (bookkeeping.BlockHeader, error) {
	var res bookkeeping.BlockHeader
	err := DecodeJSON(data, &res)
	if err != nil {
		return bookkeeping.BlockHeader{}, err
	}

	return res, nil
}

func unconvertAssetParams(params assetParams) basics.AssetParams {
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

// DecodeAssetParams decodes asset params from json.
func DecodeAssetParams(data []byte) (basics.AssetParams, error) {
	var params assetParams
	err := DecodeJSON(data, &params)
	if err != nil {
		return basics.AssetParams{}, err
	}

	return unconvertAssetParams(params), nil
}

func unconvertTransaction(txn transaction) transactions.Transaction {
	res := txn.Transaction
	res.AssetParams = unconvertAssetParams(txn.AssetParamsOverride)
	return res
}

func unconvertValueDelta(delta valueDelta) basics.ValueDelta {
	res := delta.ValueDelta
	res.Bytes = string(delta.BytesOverride)
	return res
}

func unconvertStateDelta(delta stateDelta) basics.StateDelta {
	if delta == nil {
		return nil
	}

	res := make(map[string]basics.ValueDelta, len(delta))
	for k, v := range delta {
		res[k.data] = unconvertValueDelta(v)
	}
	return res
}

func unconvertLocalDeltas(deltas map[uint64]stateDelta) map[uint64]basics.StateDelta {
	if deltas == nil {
		return nil
	}

	res := make(map[uint64]basics.StateDelta, len(deltas))
	for i, delta := range deltas {
		res[i] = unconvertStateDelta(delta)
	}
	return res
}

func unconvertEvalDelta(delta evalDelta) transactions.EvalDelta {
	res := delta.EvalDelta
	res.GlobalDelta = unconvertStateDelta(delta.GlobalDeltaOverride)
	res.LocalDeltas = unconvertLocalDeltas(delta.LocalDeltasOverride)
	return res
}

func unconvertSignedTxnWithAD(stxn signedTxnWithAD) transactions.SignedTxnWithAD {
	res := stxn.SignedTxnWithAD
	res.Txn = unconvertTransaction(stxn.TxnOverride)
	res.AuthAddr = basics.Address(stxn.AuthAddrOverride)
	res.EvalDelta = unconvertEvalDelta(stxn.EvalDeltaOverride)
	return res
}

// DecodeSignedTxnWithAD decodes signed txn with apply data from json.
func DecodeSignedTxnWithAD(data []byte) (transactions.SignedTxnWithAD, error) {
	var stxn signedTxnWithAD
	err := DecodeJSON(data, &stxn)
	if err != nil {
		return transactions.SignedTxnWithAD{}, err
	}

	return unconvertSignedTxnWithAD(stxn), nil
}

// DecodeTrimmedAccountData decodes trimmed account data from json.
func DecodeTrimmedAccountData(data []byte) (basics.AccountData, error) {
	var res basics.AccountData
	err := DecodeJSON(data, &res)
	if err != nil {
		return basics.AccountData{}, err
	}

	return res, nil
}

func unconvertTealValue(tv tealValue) basics.TealValue {
	res := tv.TealValue
	res.Bytes = string(tv.BytesOverride)
	return res
}

func unconvertTealKeyValue(tkv tealKeyValue) basics.TealKeyValue {
	if tkv == nil {
		return nil
	}

	res := basics.TealKeyValue(make(map[string]basics.TealValue, len(tkv)))
	for _, ktv := range tkv {
		res[string(ktv.Key)] = unconvertTealValue(ktv.Tv)
	}
	return res
}

func unconvertAppLocalState(state appLocalState) basics.AppLocalState {
	res := state.AppLocalState
	res.KeyValue = unconvertTealKeyValue(state.KeyValueOverride)
	return res
}

// DecodeAppLocalState decodes local application state from json.
func DecodeAppLocalState(data []byte) (basics.AppLocalState, error) {
	var state appLocalState
	err := DecodeJSON(data, &state)
	if err != nil {
		return basics.AppLocalState{}, err
	}

	return unconvertAppLocalState(state), nil
}

func unconvertAppParams(params appParams) basics.AppParams {
	res := params.AppParams
	res.GlobalState = unconvertTealKeyValue(params.GlobalStateOverride)
	return res
}

// DecodeAppParams decodes application params from json.
func DecodeAppParams(data []byte) (basics.AppParams, error) {
	var params appParams
	err := DecodeJSON(data, &params)
	if err != nil {
		return basics.AppParams{}, nil
	}

	return unconvertAppParams(params), nil
}

func unconvertAssetParamsArray(paramsArr []assetParams) []basics.AssetParams {
	if paramsArr == nil {
		return nil
	}

	res := make([]basics.AssetParams, 0, len(paramsArr))
	for _, params := range paramsArr {
		res = append(res, unconvertAssetParams(params))
	}
	return res
}

// DecodeAssetParamsArray decodes an array of asset params from a json array.
func DecodeAssetParamsArray(data []byte) ([]basics.AssetParams, error) {
	var paramsArr []assetParams
	err := DecodeJSON(data, &paramsArr)
	if err != nil {
		return nil, err
	}

	return unconvertAssetParamsArray(paramsArr), nil
}

func unconvertAppParamsArray(paramsArr []appParams) []basics.AppParams {
	if paramsArr == nil {
		return nil
	}

	res := make([]basics.AppParams, 0, len(paramsArr))
	for _, params := range paramsArr {
		res = append(res, unconvertAppParams(params))
	}
	return res
}

// DecodeAppParamsArray decodes an array of application params from a json array.
func DecodeAppParamsArray(data []byte) ([]basics.AppParams, error) {
	var paramsArr []appParams
	err := DecodeJSON(data, &paramsArr)
	if err != nil {
		return nil, err
	}

	return unconvertAppParamsArray(paramsArr), nil
}

func unconvertAppLocalStateArray(array []appLocalState) []basics.AppLocalState {
	if array == nil {
		return nil
	}

	res := make([]basics.AppLocalState, 0, len(array))
	for _, state := range array {
		res = append(res, unconvertAppLocalState(state))
	}
	return res
}

// DecodeAppLocalStateArray decodes an array of local application states from a json
// array.
func DecodeAppLocalStateArray(data []byte) ([]basics.AppLocalState, error) {
	var array []appLocalState
	err := DecodeJSON(data, &array)
	if err != nil {
		return nil, err
	}

	return unconvertAppLocalStateArray(array), nil
}

// DecodeSpecialAddresses decodes special addresses (sink and rewards pool) from json.
func DecodeSpecialAddresses(data []byte) (transactions.SpecialAddresses, error) {
	var res transactions.SpecialAddresses
	err := DecodeJSON(data, &res)
	if err != nil {
		return transactions.SpecialAddresses{}, err
	}

	return res, nil
}

// DecodeTxnExtra decodes transaction extra info from json.
func DecodeTxnExtra(data []byte) (idb.TxnExtra, error) {
	var extra idb.TxnExtra
	err := DecodeJSON(data, &extra)
	if err != nil {
		return idb.TxnExtra{}, err
	}

	return extra, nil
}

// DecodeImportState decodes import state from json.
func DecodeImportState(data []byte) (types.ImportState, error) {
	var state types.ImportState
	err := DecodeJSON(data, &state)
	if err != nil {
		return types.ImportState{}, err
	}

	return state, nil
}

// DecodeMigrationState decodes migration state from json.
func DecodeMigrationState(data []byte) (types.MigrationState, error) {
	var state types.MigrationState
	err := DecodeJSON(data, &state)
	if err != nil {
		return types.MigrationState{}, err
	}

	return state, nil
}
