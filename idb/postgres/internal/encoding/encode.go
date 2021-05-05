package encoding

import (
	"encoding/base64"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-codec/codec"
)

var jsonCodecHandle *codec.JsonHandle

// EncodeJSON converts an object into JSON
func EncodeJSON(obj interface{}) []byte {
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

func EncodeBlockHeader(header bookkeeping.BlockHeader) []byte {
	return EncodeJSON(convertBlockHeader(header))
}

func convertAssetParams(params basics.AssetParams) assetParams {
	return assetParams{
		AssetParams:      params,
		ManagerOverride:  crypto.Digest(params.Manager),
		ReserveOverride:  crypto.Digest(params.Reserve),
		FreezeOverride:   crypto.Digest(params.Freeze),
		ClawbackOverride: crypto.Digest(params.Clawback),
	}
}

// Return a json string where all byte arrays are base64 encoded.
func EncodeAssetParams(params basics.AssetParams) []byte {
	return EncodeJSON(convertAssetParams(params))
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

func (ba byteArray) MarshalText() ([]byte, error) {
	return []byte(Base64([]byte(ba.data))), nil
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

func convertEvalDelta(delta basics.EvalDelta) evalDelta {
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

// Return a json string where all byte arrays are base64 encoded.
func EncodeSignedTxnWithAD(stxn transactions.SignedTxnWithAD) []byte {
	return EncodeJSON(convertSignedTxnWithAD(stxn))
}

func convertAccountData(ad basics.AccountData) accountData {
	return accountData{
		AccountData:      ad,
		AuthAddrOverride: crypto.Digest(ad.AuthAddr),
	}
}

// Return a json string where all byte arrays are base64 encoded.
func EncodeAccountData(ad basics.AccountData) []byte {
	return EncodeJSON(convertAccountData(ad))
}

func convertTealValue(tv basics.TealValue) tealValue {
	return tealValue{
		TealValue:     tv,
		BytesOverride: []byte(tv.Bytes),
	}
}

func convertTealKeyValue(tkv basics.TealKeyValue) tealKeyValue {
	var res tealKeyValue

	if tkv == nil {
		return res
	}

	res.They = make([]keyTealValue, 0, len(tkv))
	for k, tv := range tkv {
		ktv := keyTealValue{
			Key: []byte(k),
			Tv:  convertTealValue(tv),
		}
		res.They = append(res.They, ktv)
	}
	return res
}

func convertAppLocalState(state basics.AppLocalState) appLocalState {
	return appLocalState{
		AppLocalState:    state,
		KeyValueOverride: convertTealKeyValue(state.KeyValue),
	}
}

func EncodeAppLocalState(state basics.AppLocalState) []byte {
	return EncodeJSON(convertAppLocalState(state))
}

func convertAppParams(params basics.AppParams) appParams {
	return appParams{
		AppParams:           params,
		GlobalStateOverride: convertTealKeyValue(params.GlobalState),
	}
}

func EncodeAppParams(params basics.AppParams) []byte {
	return EncodeJSON(convertAppParams(params))
}

func convertSpecialAddresses(special transactions.SpecialAddresses) specialAddresses {
	return specialAddresses{
		SpecialAddresses:    special,
		FeeSinkOverride:     crypto.Digest(special.FeeSink),
		RewardsPoolOverride: crypto.Digest(special.RewardsPool),
	}
}

func EncodeSpecialAddresses(special transactions.SpecialAddresses) []byte {
	return EncodeJSON(convertSpecialAddresses(special))
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
