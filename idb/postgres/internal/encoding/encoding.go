package encoding

import (
	"encoding/base64"
	"fmt"

	"github.com/algorand/go-codec/codec"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/types"
	itypes "github.com/algorand/indexer/types"
	"github.com/algorand/indexer/util"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

var jsonCodecHandle *codec.JsonHandle

func init() {
	jsonCodecHandle = new(codec.JsonHandle)
	jsonCodecHandle.ErrorIfNoField = true
	jsonCodecHandle.ErrorIfNoArrayExpand = true
	jsonCodecHandle.Canonical = true
	jsonCodecHandle.RecursiveEmptyCheck = true
	jsonCodecHandle.HTMLCharsAsIs = true
	jsonCodecHandle.Indent = 0
	jsonCodecHandle.MapKeyAsString = true
}

// encodeJSON converts an object into JSON
func encodeJSON(obj interface{}) []byte {
	var buf []byte
	enc := codec.NewEncoderBytes(&buf, jsonCodecHandle)
	enc.MustEncode(obj)
	return buf
}

// DecodeJSON is a function that decodes json.
func DecodeJSON(b []byte, objptr interface{}) error {
	dec := codec.NewDecoderBytes(b, jsonCodecHandle)
	return dec.Decode(objptr)
}

// Base64 encodes a byte array to a base64 string.
func Base64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func decodeBase64(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

func convertBlockHeader(header sdk.BlockHeader) blockHeader {
	return blockHeader{
		BlockHeader:         header,
		BranchOverride:      sdk.Digest(header.Branch),
		FeeSinkOverride:     sdk.Digest(header.FeeSink),
		RewardsPoolOverride: sdk.Digest(header.RewardsPool),
	}
}

func unconvertBlockHeader(header blockHeader) sdk.BlockHeader {
	res := header.BlockHeader
	res.Branch = sdk.BlockHash(header.BranchOverride)
	res.FeeSink = sdk.Address(header.FeeSinkOverride)
	res.RewardsPool = sdk.Address(header.RewardsPoolOverride)
	return res
}

// EncodeBlockHeader encodes block header into json.
func EncodeBlockHeader(header sdk.BlockHeader) []byte {
	return encodeJSON(convertBlockHeader(header))
}

// DecodeBlockHeader decodes block header from json.
func DecodeBlockHeader(data []byte) (sdk.BlockHeader, error) {
	var header blockHeader
	err := DecodeJSON(data, &header)
	if err != nil {
		return sdk.BlockHeader{}, err
	}

	return unconvertBlockHeader(header), nil
}

func convertAssetParams(params sdk.AssetParams) assetParams {
	ret := assetParams{
		AssetParams:      params,
		ManagerOverride:  sdk.Digest(params.Manager),
		ReserveOverride:  sdk.Digest(params.Reserve),
		FreezeOverride:   sdk.Digest(params.Freeze),
		ClawbackOverride: sdk.Digest(params.Clawback),
		AssetNameBytes:   []byte(params.AssetName),
		UnitNameBytes:    []byte(params.UnitName),
		URLBytes:         []byte(params.URL),
	}

	ret.AssetName = util.PrintableUTF8OrEmpty(params.AssetName)
	ret.UnitName = util.PrintableUTF8OrEmpty(params.UnitName)
	ret.URL = util.PrintableUTF8OrEmpty(params.URL)

	// If the string is printable, don't store the encoded version.
	// This is a nice optimization, and required for backwards compatibility.
	if len(ret.AssetName) > 0 {
		ret.AssetNameBytes = nil
	}
	if len(ret.UnitName) > 0 {
		ret.UnitNameBytes = nil
	}
	if len(ret.URL) > 0 {
		ret.URLBytes = nil
	}

	return ret
}

func unconvertAssetParams(params assetParams) sdk.AssetParams {
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
	res.Manager = sdk.Address(params.ManagerOverride)
	res.Reserve = sdk.Address(params.ReserveOverride)
	res.Freeze = sdk.Address(params.FreezeOverride)
	res.Clawback = sdk.Address(params.ClawbackOverride)
	return res
}

// EncodeAssetParams encodes asset params into json.
func EncodeAssetParams(params sdk.AssetParams) []byte {
	return encodeJSON(convertAssetParams(params))
}

// DecodeAssetParams decodes asset params from json.
func DecodeAssetParams(data []byte) (sdk.AssetParams, error) {
	var params assetParams
	err := DecodeJSON(data, &params)
	if err != nil {
		return sdk.AssetParams{}, err
	}

	return unconvertAssetParams(params), nil
}

func convertAccounts(accounts []sdk.Address) []sdk.Digest {
	if accounts == nil {
		return nil
	}

	res := make([]sdk.Digest, 0, len(accounts))
	for _, address := range accounts {
		res = append(res, sdk.Digest(address))
	}
	return res
}

func unconvertAccounts(accounts []sdk.Digest) []sdk.Address {
	if accounts == nil {
		return nil
	}

	res := make([]sdk.Address, 0, len(accounts))
	for _, address := range accounts {
		res = append(res, sdk.Address(address))
	}
	return res
}

func convertTransaction(txn sdk.Transaction) transaction {
	return transaction{
		Transaction:              txn,
		SenderOverride:           sdk.Digest(txn.Sender),
		RekeyToOverride:          sdk.Digest(txn.RekeyTo),
		ReceiverOverride:         sdk.Digest(txn.Receiver),
		AssetParamsOverride:      convertAssetParams(txn.AssetParams),
		CloseRemainderToOverride: sdk.Digest(txn.CloseRemainderTo),
		AssetSenderOverride:      sdk.Digest(txn.AssetSender),
		AssetReceiverOverride:    sdk.Digest(txn.AssetReceiver),
		AssetCloseToOverride:     sdk.Digest(txn.AssetCloseTo),
		FreezeAccountOverride:    sdk.Digest(txn.FreezeAccount),
		AccountsOverride:         convertAccounts(txn.Accounts),
	}
}

func unconvertTransaction(txn transaction) sdk.Transaction {
	res := txn.Transaction
	res.Sender = sdk.Address(txn.SenderOverride)
	res.RekeyTo = sdk.Address(txn.RekeyToOverride)
	res.Receiver = sdk.Address(txn.ReceiverOverride)
	res.CloseRemainderTo = sdk.Address(txn.CloseRemainderToOverride)
	res.AssetParams = unconvertAssetParams(txn.AssetParamsOverride)
	res.AssetSender = sdk.Address(txn.AssetSenderOverride)
	res.AssetReceiver = sdk.Address(txn.AssetReceiverOverride)
	res.AssetCloseTo = sdk.Address(txn.AssetCloseToOverride)
	res.FreezeAccount = sdk.Address(txn.FreezeAccountOverride)
	res.Accounts = unconvertAccounts(txn.AccountsOverride)
	return res
}

func convertValueDelta(delta sdk.ValueDelta) valueDelta {
	return valueDelta{
		ValueDelta:    delta,
		BytesOverride: []byte(delta.Bytes),
	}
}

func unconvertValueDelta(delta valueDelta) sdk.ValueDelta {
	res := delta.ValueDelta
	res.Bytes = string(delta.BytesOverride)
	return res
}

func convertStateDelta(delta sdk.StateDelta) stateDelta {
	if delta == nil {
		return nil
	}

	res := make(map[byteArray]valueDelta, len(delta))
	for k, v := range delta {
		res[byteArray{k}] = convertValueDelta(v)
	}
	return res
}

func unconvertStateDelta(delta stateDelta) sdk.StateDelta {
	if delta == nil {
		return nil
	}

	res := make(map[string]sdk.ValueDelta, len(delta))
	for k, v := range delta {
		res[k.data] = unconvertValueDelta(v)
	}
	return res
}

func convertLocalDeltas(deltas map[uint64]sdk.StateDelta) map[uint64]stateDelta {
	if deltas == nil {
		return nil
	}

	res := make(map[uint64]stateDelta, len(deltas))
	for i, delta := range deltas {
		res[i] = convertStateDelta(delta)
	}
	return res
}

func unconvertLocalDeltas(deltas map[uint64]stateDelta) map[uint64]sdk.StateDelta {
	if deltas == nil {
		return nil
	}

	res := make(map[uint64]sdk.StateDelta, len(deltas))
	for i, delta := range deltas {
		res[i] = unconvertStateDelta(delta)
	}
	return res
}

func convertLogs(logs []string) [][]byte {
	if logs == nil {
		return nil
	}

	res := make([][]byte, len(logs))
	for i, log := range logs {
		res[i] = []byte(log)
	}
	return res
}

func unconvertLogs(logs [][]byte) []string {
	if logs == nil {
		return nil
	}

	res := make([]string, len(logs))
	for i, log := range logs {
		res[i] = string(log)
	}
	return res
}

func convertInnerTxns(innerTxns []sdk.SignedTxnWithAD) []signedTxnWithAD {
	if innerTxns == nil {
		return nil
	}

	res := make([]signedTxnWithAD, len(innerTxns))
	for i, innerTxn := range innerTxns {
		res[i] = convertSignedTxnWithAD(innerTxn)
	}
	return res
}

func unconvertInnerTxns(innerTxns []signedTxnWithAD) []sdk.SignedTxnWithAD {
	if innerTxns == nil {
		return nil
	}

	res := make([]sdk.SignedTxnWithAD, len(innerTxns))
	for i, innerTxn := range innerTxns {
		res[i] = unconvertSignedTxnWithAD(innerTxn)
	}
	return res
}

func convertEvalDelta(delta sdk.EvalDelta) evalDelta {
	return evalDelta{
		EvalDelta:           delta,
		GlobalDeltaOverride: convertStateDelta(delta.GlobalDelta),
		LocalDeltasOverride: convertLocalDeltas(delta.LocalDeltas),
		LogsOverride:        convertLogs(delta.Logs),
		InnerTxnsOverride:   convertInnerTxns(delta.InnerTxns),
	}
}

func unconvertEvalDelta(delta evalDelta) sdk.EvalDelta {
	res := delta.EvalDelta
	res.GlobalDelta = unconvertStateDelta(delta.GlobalDeltaOverride)
	res.LocalDeltas = unconvertLocalDeltas(delta.LocalDeltasOverride)
	res.Logs = unconvertLogs(delta.LogsOverride)
	res.InnerTxns = unconvertInnerTxns(delta.InnerTxnsOverride)
	return res
}

func convertSignedTxnWithAD(stxn sdk.SignedTxnWithAD) signedTxnWithAD {
	return signedTxnWithAD{
		SignedTxnWithAD:   stxn,
		TxnOverride:       convertTransaction(stxn.Txn),
		AuthAddrOverride:  sdk.Digest(stxn.AuthAddr),
		EvalDeltaOverride: convertEvalDelta(stxn.EvalDelta),
	}
}

func unconvertSignedTxnWithAD(stxn signedTxnWithAD) sdk.SignedTxnWithAD {
	res := stxn.SignedTxnWithAD
	res.Txn = unconvertTransaction(stxn.TxnOverride)
	res.AuthAddr = sdk.Address(stxn.AuthAddrOverride)
	res.EvalDelta = unconvertEvalDelta(stxn.EvalDeltaOverride)
	return res
}

// EncodeSignedTxnWithAD encodes signed transaction with apply data into json.
func EncodeSignedTxnWithAD(stxn sdk.SignedTxnWithAD) []byte {
	return encodeJSON(convertSignedTxnWithAD(stxn))
}

// DecodeSignedTxnWithAD decodes signed txn with apply data from json.
func DecodeSignedTxnWithAD(data []byte) (sdk.SignedTxnWithAD, error) {
	var stxn signedTxnWithAD
	err := DecodeJSON(data, &stxn)
	if err != nil {
		return sdk.SignedTxnWithAD{}, err
	}

	return unconvertSignedTxnWithAD(stxn), nil
}

func convertTrimmedAccountData(ad basics.AccountData) trimmedAccountData {
	return trimmedAccountData{
		AccountData:      ad,
		AuthAddrOverride: crypto.Digest(ad.AuthAddr),
	}
}

func unconvertTrimmedAccountData(ad trimmedAccountData) basics.AccountData {
	res := ad.AccountData
	res.AuthAddr = basics.Address(ad.AuthAddrOverride)
	return res
}

// EncodeTrimmedAccountData encodes account data into json.
func EncodeTrimmedAccountData(ad basics.AccountData) []byte {
	return encodeJSON(convertTrimmedAccountData(ad))
}

// DecodeTrimmedAccountData decodes account data from json.
func DecodeTrimmedAccountData(data []byte) (basics.AccountData, error) {
	var ado trimmedAccountData // ado - account data with override
	err := DecodeJSON(data, &ado)
	if err != nil {
		return basics.AccountData{}, err
	}

	return unconvertTrimmedAccountData(ado), nil
}

func convertTealValue(tv basics.TealValue) tealValue {
	return tealValue{
		TealValue:     tv,
		BytesOverride: []byte(tv.Bytes),
	}
}

func unconvertTealValue(tv tealValue) basics.TealValue {
	res := tv.TealValue
	res.Bytes = string(tv.BytesOverride)
	return res
}

func convertTealKeyValue(tkv basics.TealKeyValue) tealKeyValue {
	if tkv == nil {
		return nil
	}

	res := make(map[byteArray]tealValue, len(tkv))
	for k, tv := range tkv {
		res[byteArray{data: k}] = convertTealValue(tv)
	}
	return res
}

func unconvertTealKeyValue(tkv tealKeyValue) basics.TealKeyValue {
	if tkv == nil {
		return nil
	}

	res := make(map[string]basics.TealValue, len(tkv))
	for k, tv := range tkv {
		res[k.data] = unconvertTealValue(tv)
	}
	return res
}

func convertAppLocalState(state basics.AppLocalState) appLocalState {
	return appLocalState{
		AppLocalState:    state,
		KeyValueOverride: convertTealKeyValue(state.KeyValue),
	}
}

func unconvertAppLocalState(state appLocalState) basics.AppLocalState {
	res := state.AppLocalState
	res.KeyValue = unconvertTealKeyValue(state.KeyValueOverride)
	return res
}

// EncodeAppLocalState encodes local application state into json.
func EncodeAppLocalState(state basics.AppLocalState) []byte {
	return encodeJSON(convertAppLocalState(state))
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

func convertAppParams(params basics.AppParams) appParams {
	return appParams{
		AppParams:           params,
		GlobalStateOverride: convertTealKeyValue(params.GlobalState),
	}
}

func unconvertAppParams(params appParams) basics.AppParams {
	res := params.AppParams
	res.GlobalState = unconvertTealKeyValue(params.GlobalStateOverride)
	return res
}

// EncodeAppParams encodes application params into json.
func EncodeAppParams(params basics.AppParams) []byte {
	return encodeJSON(convertAppParams(params))
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

func unconvertAssetParamsArray(paramsArr []assetParams) []sdk.AssetParams {
	if paramsArr == nil {
		return nil
	}

	res := make([]sdk.AssetParams, 0, len(paramsArr))
	for _, params := range paramsArr {
		res = append(res, unconvertAssetParams(params))
	}
	return res
}

// DecodeAssetParamsArray decodes an array of asset params from a json array.
func DecodeAssetParamsArray(data []byte) ([]sdk.AssetParams, error) {
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
func convertSpecialAddresses(special itypes.SpecialAddresses) specialAddresses {
	return specialAddresses{
		SpecialAddresses:    special,
		FeeSinkOverride:     sdk.Digest(special.FeeSink),
		RewardsPoolOverride: sdk.Digest(special.RewardsPool),
	}
}

func unconvertSpecialAddresses(special specialAddresses) itypes.SpecialAddresses {
	res := special.SpecialAddresses
	res.FeeSink = sdk.Address(special.FeeSinkOverride)
	res.RewardsPool = sdk.Address(special.RewardsPoolOverride)
	return res
}

// EncodeSpecialAddresses encodes special addresses (sink and rewards pool) into json.
func EncodeSpecialAddresses(special itypes.SpecialAddresses) []byte {
	return encodeJSON(convertSpecialAddresses(special))
}

// DecodeSpecialAddresses decodes special addresses (sink and rewards pool) from json.
func DecodeSpecialAddresses(data []byte) (itypes.SpecialAddresses, error) {
	var special specialAddresses
	err := DecodeJSON(data, &special)
	if err != nil {
		return itypes.SpecialAddresses{}, err
	}

	return unconvertSpecialAddresses(special), nil
}

// EncodeTxnExtra encodes transaction extra info into json.
func EncodeTxnExtra(extra *idb.TxnExtra) []byte {
	return encodeJSON(extra)
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

// EncodeImportState encodes import state into json.
func EncodeImportState(state *types.ImportState) []byte {
	return encodeJSON(state)
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

// EncodeMigrationState encodes migration state into json.
func EncodeMigrationState(state *types.MigrationState) []byte {
	return encodeJSON(state)
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

// EncodeNetworkState encodes network metastate into json.
func EncodeNetworkState(state *types.NetworkState) []byte {
	return encodeJSON(state)
}

// DecodeNetworkState decodes network metastate from json.
func DecodeNetworkState(data []byte) (types.NetworkState, error) {
	var state types.NetworkState
	err := DecodeJSON(data, &state)
	if err != nil {
		return types.NetworkState{}, fmt.Errorf("DecodeNetworkState() err: %w", err)
	}

	return state, nil
}

// TrimLcAccountData deletes various information from account data that we do not write
// to `account.account_data`.
func TrimLcAccountData(ad ledgercore.AccountData) ledgercore.AccountData {
	ad.MicroAlgos = basics.MicroAlgos{}
	ad.RewardsBase = 0
	ad.RewardedMicroAlgos = basics.MicroAlgos{}
	return ad
}

func convertTrimmedLcAccountData(ad ledgercore.AccountData) baseAccountData {
	return baseAccountData{
		Status:              ad.Status,
		AuthAddr:            crypto.Digest(ad.AuthAddr),
		TotalAppSchema:      ad.TotalAppSchema,
		TotalExtraAppPages:  ad.TotalExtraAppPages,
		TotalAssetParams:    ad.TotalAssetParams,
		TotalAssets:         ad.TotalAssets,
		TotalAppParams:      ad.TotalAppParams,
		TotalAppLocalStates: ad.TotalAppLocalStates,
		TotalBoxes:          ad.TotalBoxes,
		TotalBoxBytes:       ad.TotalBoxBytes,
		baseOnlineAccountData: baseOnlineAccountData{
			VoteID:          ad.VoteID,
			SelectionID:     ad.SelectionID,
			StateProofID:    ad.StateProofID,
			VoteFirstValid:  ad.VoteFirstValid,
			VoteLastValid:   ad.VoteLastValid,
			VoteKeyDilution: ad.VoteKeyDilution,
		},
	}
}

func unconvertTrimmedLcAccountData(ba baseAccountData) ledgercore.AccountData {
	return ledgercore.AccountData{
		AccountBaseData: ledgercore.AccountBaseData{
			Status:              ba.Status,
			AuthAddr:            basics.Address(ba.AuthAddr),
			TotalAppSchema:      ba.TotalAppSchema,
			TotalExtraAppPages:  ba.TotalExtraAppPages,
			TotalAppParams:      ba.TotalAppParams,
			TotalAppLocalStates: ba.TotalAppLocalStates,
			TotalAssetParams:    ba.TotalAssetParams,
			TotalAssets:         ba.TotalAssets,
			TotalBoxes:          ba.TotalBoxes,
			TotalBoxBytes:       ba.TotalBoxBytes,
		},
		VotingData: ledgercore.VotingData{
			VoteID:          ba.VoteID,
			SelectionID:     ba.SelectionID,
			StateProofID:    ba.StateProofID,
			VoteFirstValid:  ba.VoteFirstValid,
			VoteLastValid:   ba.VoteLastValid,
			VoteKeyDilution: ba.VoteKeyDilution,
		},
	}
}

// EncodeTrimmedLcAccountData encodes ledgercore account data into json.
func EncodeTrimmedLcAccountData(ad ledgercore.AccountData) []byte {
	return encodeJSON(convertTrimmedLcAccountData(ad))
}

// DecodeTrimmedLcAccountData decodes ledgercore account data from json.
func DecodeTrimmedLcAccountData(data []byte) (ledgercore.AccountData, error) {
	var ba baseAccountData
	err := DecodeJSON(data, &ba)
	if err != nil {
		return ledgercore.AccountData{}, err
	}

	return unconvertTrimmedLcAccountData(ba), nil
}

// EncodeDeleteStatus encodes network metastate into json.
func EncodeDeleteStatus(p *types.DeleteStatus) []byte {
	return encodeJSON(p)
}

// DecodeDeleteStatus decodes network metastate from json.
func DecodeDeleteStatus(data []byte) (types.DeleteStatus, error) {
	var status types.DeleteStatus
	err := DecodeJSON(data, &status)
	if err != nil {
		return types.DeleteStatus{}, fmt.Errorf("DecodeDeleteStatus() err: %w", err)
	}

	return status, nil
}
