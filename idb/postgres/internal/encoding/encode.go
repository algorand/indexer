package encoding

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/algorand/go-codec/codec"

	"github.com/algorand/indexer/types"
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

// ConvertStringForQuery converts a string into something postgres can use to query a jsonb column.
func ConvertStringForQuery(str string) string {
	return strings.ReplaceAll(ConvertString(str), "\\", "\\\\")
}

// ConvertString converts a string into something postgres can store in a jsonb column.
func ConvertString(str string) string {
	return EscapeNulls(str)
	/*
	strBytes := []byte(str)
	i := 0
	// Check each rune to see if base64 encoding is needed
	for i < len(strBytes) {
		c, csize := utf8.DecodeRune(strBytes[i:])
		if c == utf8.RuneError {
			break
		}
		if !unicode.IsPrint(c) {
			break
		}
		i += csize
	}

	if i != len(strBytes) {
		return base64.StdEncoding.EncodeToString(strBytes)
	}
	return str
	 */
}

// ConvertAssetParams sanitizes all AssetParams that need it.
// The AssetParams encoding policy needs to take into account that algod accepts
// any user defined string that go accepts. The notable part here is that postgres
// does not allow the null character:
//                            https://www.postgresql.org/docs/11/datatype-json.html
// Our policy is a uni-directional encoding. If the AssetParam object contains
// any zero byte characters, they are converted to `\\u0000`. When the AssetParams
// are returned by '/v2/assets' or '/v2/accounts', the response contains the
// encoded string instead of a zero byte.
//
// Note that '/v2/transactions' returns the raw transaction bytes, so this
// endpoint returns the correct string complete with zero bytes.
func ConvertAssetParams(params types.AssetParams) types.AssetParams {
	params.AssetName = ConvertString(params.AssetName)
	params.UnitName = ConvertString(params.UnitName)
	params.URL = ConvertString(params.URL)
	return params
}

func convertSignedTxnWithAD(stxn types.SignedTxnWithAD) types.SignedTxnWithAD {
	stxn.Txn.AssetParams = ConvertAssetParams(stxn.Txn.AssetParams)
	stxn.EvalDelta = convertEvalDelta(stxn.EvalDelta)
	return stxn
}

// EncodeSignedTxnWithAD returns a json string where all byte arrays are base64 encoded.
func EncodeSignedTxnWithAD(stxn types.SignedTxnWithAD) []byte {
	return EncodeJSON(convertSignedTxnWithAD(stxn))
}

func nothingToEscape(x string) bool {
	strBytes := []byte(x)
	i := 0
	// Check each rune to see if base64 encoding is needed
	for i < len(strBytes) {
		c, csize := utf8.DecodeRune(strBytes[i:])
		if c == utf8.RuneError {
			return false
		}
		if !unicode.IsPrint(c) {
			return false
		}
		if c == '\\' {
			return false
		}
		i += csize
	}
	return true
}

func EscapeNulls(x string) string {
	if nothingToEscape(x) {
		return x
	}

	xb := []byte(x)

	escapenull := []byte("\\u0000")
	var out strings.Builder
	var runehex = [6]byte{'\\', 'u', '0', '0'}
	start := 0
	outpos := 0
	i := 0
	for i < len(xb) {
		c, csize := utf8.DecodeRune(xb[i:])
		if c == utf8.RuneError {
			out.Write(xb[start:i])
			outpos += i - start
			start = i + csize
			for xi := i; xi < start; xi++ {
				hex.Encode(runehex[4:], xb[xi:xi+1])
				out.Write(runehex[:])
				outpos += 6
			}
			i += csize
			continue
		}
		switch c {
		case 0:
			out.Write(xb[start:i])
			outpos += i - start
			start = i + 1
			out.Write(escapenull)
			outpos += 6
		case '\\':
			out.Write(xb[start:i])
			outpos += i - start
			start = i + 1
			out.Write([]byte("\\\\"))
			outpos+=2
		default:
		}
		i += csize
	}
	if start < len(xb) {
		out.Write(xb[start:])
	}
	return out.String()
}

// UnescapeNulls is the inverse function of EscapeNulls.
// UnescapeNulls converts \\ and \uXXXX back into their unescaped form but may not be fully general for input not generated by EscapeNulls().
func UnescapeNulls(x string) string {
	newlen := len(x)
	start := 0
	for i, c := range x {
		if i < start {
			continue
		}
		if c == '\\' {
			if x[i+1] == '\\' {
				start = i + 2
				newlen--
			} else if x[i+1:i+4] == "u00" {
				start = i + 6
				newlen -= 5
			} // else shrug? warning? panic?
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
			if x[i+1:i+4] == "u00" {
				copy(out[outpos:], xb[start:i])
				outpos += i - start
				start = i + 6
				hex.Decode(out[outpos:], xb[i+4:i+6])
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
