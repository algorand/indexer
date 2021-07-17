package encoding

import (
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/algorand/go-codec/codec"

	"github.com/algorand/indexer/types"
)

var jsonCodecHandle *codec.JsonHandle

// EncodeJSON converts an object into JSON
func EncodeJSON(obj interface{}) []byte {
	var buf []byte
	enc := codec.NewEncoderBytes(&buf, jsonCodecHandle)
	enc.MustEncode(obj)
	return buf
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

// ConvertAssetParams sanitizes asset param string fields before encoding to JSON bytes.
func ConvertAssetParams(ap types.AssetParamsWithExtra) types.AssetParamsWithExtra {
	ap.AssetName = printableUTF8OrEmpty(ap.AssetName)
	ap.UnitName = printableUTF8OrEmpty(ap.UnitName)
	ap.URL = printableUTF8OrEmpty(ap.URL)
	// If the string is printable, don't store the encoded version.
	// This is a nice optimization, and required for backwards compatibility.
	if len(ap.AssetName) > 0 {
		ap.AssetNameBytes = nil
	}
	if len(ap.UnitName) > 0 {
		ap.UnitNameBytes = nil
	}
	if len(ap.URL) > 0 {
		ap.URLBytes = nil
	}
	return ap
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

func convertEvalDelta(evalDelta types.EvalDelta) types.EvalDelta {
	evalDelta.GlobalDelta = convertStateDelta(evalDelta.GlobalDelta)
	evalDelta.LocalDeltas = convertLocalDeltas(evalDelta.LocalDeltas)
	return evalDelta
}

func convertSignedTxnWithAD(stxn types.SignedTxnWithAD) types.SignedTxnWithAD {
	stxn.EvalDelta = convertEvalDelta(stxn.EvalDelta)
	return stxn
}

// EncodeSignedTxnWithAD returns a json string where all byte arrays are base64 encoded.
func EncodeSignedTxnWithAD(stxn types.SignedTxnWithAD) []byte {
	ap := stxn.Txn.AssetParams
	ap.AssetName = strings.ReplaceAll(ap.AssetName, "\000", "")
	ap.UnitName = strings.ReplaceAll(ap.UnitName, "\000", "")
	ap.URL = strings.ReplaceAll(ap.URL, "\000", "")
	stxn.Txn.AssetParams = ap
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
