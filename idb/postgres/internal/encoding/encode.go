package encoding

import (
	"encoding/base64"

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

func convertSignedTxnWithAD(stxn types.SignedTxnWithAD) types.SignedTxnWithAD {
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
