package encoding

import (
	"encoding/base64"
	"unicode"
	"unicode/utf8"

	sdk_types "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-codec/codec"

	"github.com/algorand/indexer/types"
	"github.com/algorand/indexer/util"
)

var jsonCodecHandle *codec.JsonHandle

// EncodeJSON converts an object into JSON
func EncodeJSON(obj interface{}) []byte {
	var buf []byte
	enc := codec.NewEncoderBytes(&buf, jsonCodecHandle)
	enc.MustEncode(obj)
	return buf
}

func convertAssetParams(params sdk_types.AssetParams) assetParams {
	ret := assetParams{
		AssetParams:    params,
		AssetNameBytes: []byte(params.AssetName),
		UnitNameBytes:  []byte(params.UnitName),
		URLBytes:       []byte(params.URL),
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

// EncodeAssetParams returns a json string where all byte arrays are base64 encoded.
func EncodeAssetParams(params sdk_types.AssetParams) []byte {
	return EncodeJSON(convertAssetParams(params))
}

// Base64 encodes a byte array to a base64 string.
func Base64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func convertStateDelta(delta types.StateDelta) types.StateDelta {
	if delta == nil {
		return nil
	}

	res := make(map[string]types.ValueDelta, len(delta))
	for k, v := range delta {
		res[Base64([]byte(k))] = v
	}
	return res
}

func convertLocalDeltas(deltas map[uint64]types.StateDelta) map[uint64]types.StateDelta {
	if deltas == nil {
		return nil
	}

	res := make(map[uint64]types.StateDelta, len(deltas))
	for i, delta := range deltas {
		res[i] = convertStateDelta(delta)
	}
	return res
}

// printableUTF8OrEmpty checks to see if the entire string is a UTF8 printable string.
// If this is the case, the string is returned as is. Otherwise, the empty string is returned.
func printableUTF8OrEmpty(in string) string {
	// iterate throughout all the characters in the string to see if they are all printable.
	// when range iterating on go strings, go decode each element as a utf8 rune.
	for _, c := range in {
		// is this a printable character, or invalid rune ?
		if c == utf8.RuneError || !unicode.IsPrint(c) {
			return ""
		}
	}
	return in
}

func convertItxnSignedTxnWithAD(stxn types.SignedTxnWithAD) types.SignedTxnWithAD {
	stxn.EvalDelta = convertEvalDelta(stxn.EvalDelta)
	// Remove non UTF8 characters from Asset params in Inner Transactions
	stxn.Txn.AssetParams.AssetName = util.PrintableUTF8OrEmpty(stxn.Txn.AssetParams.AssetName)
	stxn.Txn.AssetParams.UnitName = util.PrintableUTF8OrEmpty(stxn.Txn.AssetParams.UnitName)
	stxn.Txn.AssetParams.URL = util.PrintableUTF8OrEmpty(stxn.Txn.AssetParams.URL)
	return stxn
}

func convertInnerTxns(innerTxns []types.SignedTxnWithAD) []types.SignedTxnWithAD {
	if innerTxns == nil {
		return nil
	}

	res := make([]types.SignedTxnWithAD, len(innerTxns))
	for i, innerTxn := range innerTxns {
		res[i] = convertItxnSignedTxnWithAD(innerTxn)
	}
	return res
}

func removeNonUTF8Chars(logs []string) []string {
	if logs == nil {
		return nil
	}
	res := make([]string, len(logs))
	for i, log := range logs {
		res[i] = printableUTF8OrEmpty(log)
	}
	return res
}

func convertEvalDelta(evalDelta types.EvalDelta) types.EvalDelta {
	evalDelta.Logs = removeNonUTF8Chars(evalDelta.Logs)
	evalDelta.GlobalDelta = convertStateDelta(evalDelta.GlobalDelta)
	evalDelta.LocalDeltas = convertLocalDeltas(evalDelta.LocalDeltas)
	evalDelta.InnerTxns = convertInnerTxns(evalDelta.InnerTxns)
	return evalDelta
}

func convertTransaction(txn types.Transaction) transaction {
	return transaction{
		Transaction:         txn,
		AssetParamsOverride: convertAssetParams(txn.AssetParams),
	}
}

func convertSignedTxnWithAD(stxn types.SignedTxnWithAD) signedTxnWithAD {
	stxn.EvalDelta = convertEvalDelta(stxn.EvalDelta)
	return signedTxnWithAD{
		SignedTxnWithAD: stxn,
		TxnOverride:     convertTransaction(stxn.Txn),
	}
}

// EncodeSignedTxnWithAD returns a json string where all byte arrays are base64 encoded.
func EncodeSignedTxnWithAD(stxn types.SignedTxnWithAD) []byte {
	return EncodeJSON(convertSignedTxnWithAD(stxn))
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
