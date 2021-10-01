package encoding

import (
	"encoding/base64"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-codec/codec"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/types"
	"github.com/algorand/indexer/util"
)

var jsonCodecHandle *codec.JsonHandle

// encodeJSON converts an object into JSON
func encodeJSON(obj interface{}) []byte {
	var buf []byte
	enc := codec.NewEncoderBytes(&buf, jsonCodecHandle)
	enc.MustEncode(obj)
	return buf
}

// Base64 encodes a byte array to a base64 string.
func Base64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func convertBlockHeader(header bookkeeping.BlockHeader) blockHeader {
	return blockHeader{
		BlockHeader:         header,
		BranchOverride:      crypto.Digest(header.Branch),
		FeeSinkOverride:     crypto.Digest(header.FeeSink),
		RewardsPoolOverride: crypto.Digest(header.RewardsPool),
	}
}

// EncodeBlockHeader encodes block header into json.
func EncodeBlockHeader(header bookkeeping.BlockHeader) []byte {
	return encodeJSON(convertBlockHeader(header))
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

// EncodeAssetParams encodes asset params into json.
func EncodeAssetParams(params basics.AssetParams) []byte {
	return encodeJSON(convertAssetParams(params))
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

func convertValueDelta(delta basics.ValueDelta) valueDelta {
	return valueDelta{
		ValueDelta:    delta,
		BytesOverride: []byte(delta.Bytes),
	}
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

func convertEvalDelta(delta transactions.EvalDelta) evalDelta {
	return evalDelta{
		EvalDelta:           delta,
		GlobalDeltaOverride: convertStateDelta(delta.GlobalDelta),
		LocalDeltasOverride: convertLocalDeltas(delta.LocalDeltas),
	}
}

func convertSignedTxnWithAD(stxn transactions.SignedTxnWithAD) signedTxnWithAD {
	return signedTxnWithAD{
		SignedTxnWithAD:   stxn,
		TxnOverride:       convertTransaction(stxn.Txn),
		AuthAddrOverride:  crypto.Digest(stxn.AuthAddr),
		EvalDeltaOverride: convertEvalDelta(stxn.EvalDelta),
	}
}

// EncodeSignedTxnWithAD encodes signed transaction with apply data into json.
func EncodeSignedTxnWithAD(stxn transactions.SignedTxnWithAD) []byte {
	return encodeJSON(convertSignedTxnWithAD(stxn))
}

// TrimAccountData deletes various information from account data that we do not write to
// `account.account_data`.
func TrimAccountData(ad basics.AccountData) basics.AccountData {
	ad.MicroAlgos = basics.MicroAlgos{}
	ad.RewardsBase = 0
	ad.RewardedMicroAlgos = basics.MicroAlgos{}
	ad.AssetParams = nil
	ad.Assets = nil
	ad.AppLocalStates = nil
	ad.AppParams = nil
	ad.TotalAppSchema = basics.StateSchema{}
	ad.TotalExtraAppPages = 0

	return ad
}

func convertTrimmedAccountData(ad basics.AccountData) trimmedAccountData {
	return trimmedAccountData{
		AccountData:      ad,
		AuthAddrOverride: crypto.Digest(ad.AuthAddr),
	}
}

// EncodeTrimmedAccountData encodes account data into json.
func EncodeTrimmedAccountData(ad basics.AccountData) []byte {
	return encodeJSON(convertTrimmedAccountData(ad))
}

func convertTealValue(tv basics.TealValue) tealValue {
	return tealValue{
		TealValue:     tv,
		BytesOverride: []byte(tv.Bytes),
	}
}

func convertTealKeyValue(tkv basics.TealKeyValue) tealKeyValue {
	if tkv == nil {
		return nil
	}

	res := make([]keyTealValue, 0, len(tkv))
	for k, tv := range tkv {
		ktv := keyTealValue{
			Key: []byte(k),
			Tv:  convertTealValue(tv),
		}
		res = append(res, ktv)
	}
	return res
}

func convertAppLocalState(state basics.AppLocalState) appLocalState {
	return appLocalState{
		AppLocalState:    state,
		KeyValueOverride: convertTealKeyValue(state.KeyValue),
	}
}

// EncodeAppLocalState encodes local application state into json.
func EncodeAppLocalState(state basics.AppLocalState) []byte {
	return encodeJSON(convertAppLocalState(state))
}

func convertAppParams(params basics.AppParams) appParams {
	return appParams{
		AppParams:           params,
		GlobalStateOverride: convertTealKeyValue(params.GlobalState),
	}
}

// EncodeAppParams encodes application params into json.
func EncodeAppParams(params basics.AppParams) []byte {
	return encodeJSON(convertAppParams(params))
}

func convertSpecialAddresses(special transactions.SpecialAddresses) specialAddresses {
	return specialAddresses{
		SpecialAddresses:    special,
		FeeSinkOverride:     crypto.Digest(special.FeeSink),
		RewardsPoolOverride: crypto.Digest(special.RewardsPool),
	}
}

// EncodeSpecialAddresses encodes special addresses (sink and rewards pool) into json.
func EncodeSpecialAddresses(special transactions.SpecialAddresses) []byte {
	return encodeJSON(convertSpecialAddresses(special))
}

// EncodeTxnExtra encodes transaction extra info into json.
func EncodeTxnExtra(extra *idb.TxnExtra) []byte {
	return encodeJSON(extra)
}

// EncodeImportState encodes import state into json.
func EncodeImportState(state *types.ImportState) []byte {
	return encodeJSON(state)
}

// EncodeMigrationState encodes migration state into json.
func EncodeMigrationState(state *types.MigrationState) []byte {
	return encodeJSON(state)
}

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
