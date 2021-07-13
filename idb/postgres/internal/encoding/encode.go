package encoding

import (
	"encoding/base64"
	"strings"

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

func desanitizeNull(str string) string {
	return strings.ReplaceAll(str, `\u0000`, "\x00")
}

// SanitizeNullForQuery converts a string into something postgres can store in a jsonb column.
func SanitizeNullForQuery(str string) string {
	return strings.ReplaceAll(str, "\x00", `\\u0000`)
}

// sanitizeNull converts a string into something postgres can store in a jsonb column.
func sanitizeNull(str string) string {
	str = strings.ReplaceAll(str, "\\", "\\\\")
	return strings.ReplaceAll(str, "\x00", `\u0000`)
}

// SanitizeParams sanitizes all AssetParams that need it.
func SanitizeParams(params types.AssetParams) types.AssetParams {
	params.AssetName = sanitizeNull(params.AssetName)
	params.UnitName = sanitizeNull(params.UnitName)
	params.URL = sanitizeNull(params.URL)
	return params
}

// DesanitizeParams desanitizes all AssetParams that need it.
func DesanitizeParams(params types.AssetParams) types.AssetParams {
	params.AssetName = desanitizeNull(params.AssetName)
	params.UnitName = desanitizeNull(params.UnitName)
	params.URL = desanitizeNull(params.URL)
	return params
}

func convertSignedTxnWithAD(stxn types.SignedTxnWithAD) types.SignedTxnWithAD {
	stxn.Txn.AssetParams = SanitizeParams(stxn.Txn.AssetParams)

	stxn.EvalDelta = convertEvalDelta(stxn.EvalDelta)
	return stxn
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

func EscapeNulls(x string) string {
	newlen := 0
	for _, c := range x {
		switch c {
		case 0:
			newlen += 6 // \u0000
		case '\\':
			newlen += 2
		default:
			newlen += 1
		}
	}
	if len(x) == newlen {
		return x
	}
	xb := []byte(x)
	const escapenull = "\\u0000"
	out := make([]byte, newlen)
	start := 0
	outpos := 0
	for i, c := range x {
		switch c {
		case 0:
			copy(out[outpos:], xb[start:i])
			outpos += i - start
			start = i + 1
			copy(out[outpos:], escapenull)
			outpos += 6
		case '\\':
			copy(out[outpos:], xb[start:i])
			outpos += i - start
			start = i + 1
			out[outpos] = '\\'
			outpos++
			out[outpos] = '\\'
			outpos++
		default:
		}
	}
	if start < len(xb) {
		copy(out[outpos:], xb[start:])
	}
	return string(out)
}

func UnescapeNulls(x string) string {
	newlen := len(x)
	start := 0
	for i, c := range x {
		if i < start {
			continue
		}
		if c == '\\' {
			if x[i+1:i+6] == "u0000" {
				start = i + 6
				newlen -= 5
			} else if x[i+1] == '\\' {
				start = i + 2
				newlen--
			}
		}
	}
	if newlen == len(x) {
		return x
	}
	xb := []byte(x)
	out := make([]byte, newlen)
	start = 0
	outpos := 0
	for i, c := range x {
		if i < start {
			continue
		}
		if c == '\\' {
			if x[i+1:i+6] == "u0000" {
				copy(out[outpos:], xb[start:i])
				outpos += i - start
				start = i + 6
				out[outpos] = 0
				outpos++
			} else if x[i+1] == '\\' {
				copy(out[outpos:], xb[start:i+1])
				outpos += i + 1 - start
				start = i + 2
			}
		}
	}
	if start < len(xb) {
		copy(out[outpos:], xb[start:])
	}
	return string(out)
}
