package encoding

import (
	"encoding/base64"
	"fmt"

	"github.com/algorand/go-codec/codec"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/types"
	"github.com/algorand/indexer/util"

	sdk_types "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
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

// MarshalText returns the address string as an array of bytes
func (addr *AlgodEncodedAddress) MarshalText() ([]byte, error) {
	var a sdk_types.Address
	copy(a[:], addr[:])
	return []byte(a.String()), nil
}

// UnmarshalText initializes the Address from an array of bytes.
// The bytes may be in the base32 checksum format, or the raw bytes base64 encoded.
// Copied from newer version of go-algorand-sdk.
func (addr *AlgodEncodedAddress) UnmarshalText(text []byte) error {
	address, err := sdk_types.DecodeAddress(string(text))
	if err == nil {
		copy(addr[:], address[:])
		return nil
	}
	// ignore the DecodeAddress error because it isn't the native MarshalText format.

	// Check if its b64 encoded
	data, err := base64.StdEncoding.DecodeString(string(text))
	if err == nil {
		if len(data) != len(addr[:]) {
			return fmt.Errorf("decoded address is the wrong length, should be %d bytes", len(addr[:]))
		}
		copy(addr[:], data[:])
		return nil
	}
	return err
}

func convertExpiredAccounts(accounts []basics.Address) []AlgodEncodedAddress {
	if len(accounts) == 0 {
		return nil
	}
	res := make([]AlgodEncodedAddress, len(accounts))
	for i, addr := range accounts {
		res[i] = AlgodEncodedAddress(addr)
	}
	return res
}

func unconvertExpiredAccounts(accounts []AlgodEncodedAddress) []basics.Address {
	if len(accounts) == 0 {
		return nil
	}
	res := make([]basics.Address, len(accounts))
	for i, addr := range accounts {
		res[i] = basics.Address(addr)
	}
	return res
}

func convertBlockHeader(header bookkeeping.BlockHeader) blockHeader {
	return blockHeader{
		BlockHeader:                          header,
		BranchOverride:                       crypto.Digest(header.Branch),
		FeeSinkOverride:                      crypto.Digest(header.FeeSink),
		RewardsPoolOverride:                  crypto.Digest(header.RewardsPool),
		ExpiredParticipationAccountsOverride: convertExpiredAccounts(header.ExpiredParticipationAccounts),
	}
}

func unconvertBlockHeader(header blockHeader) bookkeeping.BlockHeader {
	res := header.BlockHeader
	res.Branch = bookkeeping.BlockHash(header.BranchOverride)
	res.FeeSink = basics.Address(header.FeeSinkOverride)
	res.RewardsPool = basics.Address(header.RewardsPoolOverride)
	res.ExpiredParticipationAccounts = unconvertExpiredAccounts(header.ExpiredParticipationAccountsOverride)
	return res
}

// EncodeBlockHeader encodes block header into json.
func EncodeBlockHeader(header bookkeeping.BlockHeader) []byte {
	return encodeJSON(convertBlockHeader(header))
}

// DecodeBlockHeader decodes block header from json.
func DecodeBlockHeader(data []byte) (bookkeeping.BlockHeader, error) {
	var header blockHeader
	err := DecodeJSON(data, &header)
	if err != nil {
		return bookkeeping.BlockHeader{}, err
	}

	return unconvertBlockHeader(header), nil
}

func convertAssetParams(params basics.AssetParams) assetParams {
	ret := assetParams{
		AssetParams:      params,
		ManagerOverride:  crypto.Digest(params.Manager),
		ReserveOverride:  crypto.Digest(params.Reserve),
		FreezeOverride:   crypto.Digest(params.Freeze),
		ClawbackOverride: crypto.Digest(params.Clawback),
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
	res.Manager = basics.Address(params.ManagerOverride)
	res.Reserve = basics.Address(params.ReserveOverride)
	res.Freeze = basics.Address(params.FreezeOverride)
	res.Clawback = basics.Address(params.ClawbackOverride)
	return res
}

// EncodeAssetParams encodes asset params into json.
func EncodeAssetParams(params basics.AssetParams) []byte {
	return encodeJSON(convertAssetParams(params))
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

func convertAccounts(accounts []basics.Address) []crypto.Digest {
	if accounts == nil {
		return nil
	}

	res := make([]crypto.Digest, 0, len(accounts))
	for _, address := range accounts {
		res = append(res, crypto.Digest(address))
	}
	return res
}

func unconvertAccounts(accounts []crypto.Digest) []basics.Address {
	if accounts == nil {
		return nil
	}

	res := make([]basics.Address, 0, len(accounts))
	for _, address := range accounts {
		res = append(res, basics.Address(address))
	}
	return res
}

func convertTransaction(txn transactions.Transaction) transaction {
	return transaction{
		Transaction:              txn,
		SenderOverride:           crypto.Digest(txn.Sender),
		RekeyToOverride:          crypto.Digest(txn.RekeyTo),
		ReceiverOverride:         crypto.Digest(txn.Receiver),
		AssetParamsOverride:      convertAssetParams(txn.AssetParams),
		CloseRemainderToOverride: crypto.Digest(txn.CloseRemainderTo),
		AssetSenderOverride:      crypto.Digest(txn.AssetSender),
		AssetReceiverOverride:    crypto.Digest(txn.AssetReceiver),
		AssetCloseToOverride:     crypto.Digest(txn.AssetCloseTo),
		FreezeAccountOverride:    crypto.Digest(txn.FreezeAccount),
		AccountsOverride:         convertAccounts(txn.Accounts),
	}
}

func unconvertTransaction(txn transaction) transactions.Transaction {
	res := txn.Transaction
	res.Sender = basics.Address(txn.SenderOverride)
	res.RekeyTo = basics.Address(txn.RekeyToOverride)
	res.Receiver = basics.Address(txn.ReceiverOverride)
	res.CloseRemainderTo = basics.Address(txn.CloseRemainderToOverride)
	res.AssetParams = unconvertAssetParams(txn.AssetParamsOverride)
	res.AssetSender = basics.Address(txn.AssetSenderOverride)
	res.AssetReceiver = basics.Address(txn.AssetReceiverOverride)
	res.AssetCloseTo = basics.Address(txn.AssetCloseToOverride)
	res.FreezeAccount = basics.Address(txn.FreezeAccountOverride)
	res.Accounts = unconvertAccounts(txn.AccountsOverride)
	return res
}

func convertValueDelta(delta basics.ValueDelta) valueDelta {
	return valueDelta{
		ValueDelta:    delta,
		BytesOverride: []byte(delta.Bytes),
	}
}

func unconvertValueDelta(delta valueDelta) basics.ValueDelta {
	res := delta.ValueDelta
	res.Bytes = string(delta.BytesOverride)
	return res
}

func convertStateDelta(delta basics.StateDelta) stateDelta {
	if delta == nil {
		return nil
	}

	res := make(map[byteArray]valueDelta, len(delta))
	for k, v := range delta {
		res[byteArray{k}] = convertValueDelta(v)
	}
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

func convertLocalDeltas(deltas map[uint64]basics.StateDelta) map[uint64]stateDelta {
	if deltas == nil {
		return nil
	}

	res := make(map[uint64]stateDelta, len(deltas))
	for i, delta := range deltas {
		res[i] = convertStateDelta(delta)
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

func convertInnerTxns(innerTxns []transactions.SignedTxnWithAD) []signedTxnWithAD {
	if innerTxns == nil {
		return nil
	}

	res := make([]signedTxnWithAD, len(innerTxns))
	for i, innerTxn := range innerTxns {
		res[i] = convertSignedTxnWithAD(innerTxn)
	}
	return res
}

func unconvertInnerTxns(innerTxns []signedTxnWithAD) []transactions.SignedTxnWithAD {
	if innerTxns == nil {
		return nil
	}

	res := make([]transactions.SignedTxnWithAD, len(innerTxns))
	for i, innerTxn := range innerTxns {
		res[i] = unconvertSignedTxnWithAD(innerTxn)
	}
	return res
}

func convertEvalDelta(delta transactions.EvalDelta) evalDelta {
	return evalDelta{
		EvalDelta:           delta,
		GlobalDeltaOverride: convertStateDelta(delta.GlobalDelta),
		LocalDeltasOverride: convertLocalDeltas(delta.LocalDeltas),
		LogsOverride:        convertLogs(delta.Logs),
		InnerTxnsOverride:   convertInnerTxns(delta.InnerTxns),
	}
}

func unconvertEvalDelta(delta evalDelta) transactions.EvalDelta {
	res := delta.EvalDelta
	res.GlobalDelta = unconvertStateDelta(delta.GlobalDeltaOverride)
	res.LocalDeltas = unconvertLocalDeltas(delta.LocalDeltasOverride)
	res.Logs = unconvertLogs(delta.LogsOverride)
	res.InnerTxns = unconvertInnerTxns(delta.InnerTxnsOverride)
	return res
}

func convertSignedTxnWithAD(stxn transactions.SignedTxnWithAD) signedTxnWithAD {
	return signedTxnWithAD{
		SignedTxnWithAD:   stxn,
		TxnOverride:       convertTransaction(stxn.Txn),
		AuthAddrOverride:  crypto.Digest(stxn.AuthAddr),
		EvalDeltaOverride: convertEvalDelta(stxn.EvalDelta),
	}
}

func unconvertSignedTxnWithAD(stxn signedTxnWithAD) transactions.SignedTxnWithAD {
	res := stxn.SignedTxnWithAD
	res.Txn = unconvertTransaction(stxn.TxnOverride)
	res.AuthAddr = basics.Address(stxn.AuthAddrOverride)
	res.EvalDelta = unconvertEvalDelta(stxn.EvalDeltaOverride)
	return res
}

// EncodeSignedTxnWithAD encodes signed transaction with apply data into json.
func EncodeSignedTxnWithAD(stxn transactions.SignedTxnWithAD) []byte {
	return encodeJSON(convertSignedTxnWithAD(stxn))
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
func convertSpecialAddresses(special transactions.SpecialAddresses) specialAddresses {
	return specialAddresses{
		SpecialAddresses:    special,
		FeeSinkOverride:     crypto.Digest(special.FeeSink),
		RewardsPoolOverride: crypto.Digest(special.RewardsPool),
	}
}

func unconvertSpecialAddresses(special specialAddresses) transactions.SpecialAddresses {
	res := special.SpecialAddresses
	res.FeeSink = basics.Address(special.FeeSinkOverride)
	res.RewardsPool = basics.Address(special.RewardsPoolOverride)
	return res
}

// EncodeSpecialAddresses encodes special addresses (sink and rewards pool) into json.
func EncodeSpecialAddresses(special transactions.SpecialAddresses) []byte {
	return encodeJSON(convertSpecialAddresses(special))
}

// DecodeSpecialAddresses decodes special addresses (sink and rewards pool) from json.
func DecodeSpecialAddresses(data []byte) (transactions.SpecialAddresses, error) {
	var special specialAddresses
	err := DecodeJSON(data, &special)
	if err != nil {
		return transactions.SpecialAddresses{}, err
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

// EncodeAccountTotals encodes account totals into json.
func EncodeAccountTotals(totals *ledgercore.AccountTotals) []byte {
	return encodeJSON(totals)
}

// DecodeAccountTotals decodes account totals from json.
func DecodeAccountTotals(data []byte) (ledgercore.AccountTotals, error) {
	var res ledgercore.AccountTotals
	err := DecodeJSON(data, &res)
	if err != nil {
		return ledgercore.AccountTotals{}, err
	}

	return res, nil
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
