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

// EncodeStringForQuery converts a string into something postgres can use to query a jsonb column.
func EncodeStringForQuery(str string) string {
	return strings.ReplaceAll(str, "\x00", `\\u0000`)
}

// EncodeString converts a string into something postgres can store in a jsonb column.
func EncodeString(str string) string {
	return strings.ReplaceAll(str, "\x00", `\u0000`)
}

// EncodeAssetParams sanitizes all AssetParams that need it.
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
func EncodeAssetParams(params types.AssetParams) types.AssetParams {
	params.AssetName = EncodeString(params.AssetName)
	params.UnitName = EncodeString(params.UnitName)
	params.URL = EncodeString(params.URL)
	return params
}

func convertSignedTxnWithAD(stxn types.SignedTxnWithAD) types.SignedTxnWithAD {
	stxn.Txn.AssetParams = EncodeAssetParams(stxn.Txn.AssetParams)
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
