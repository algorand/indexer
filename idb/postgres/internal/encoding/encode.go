package encoding

import (
	"encoding/base64"
	sdk_types "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-codec/codec"
	"github.com/algorand/indexer/util"
	"strings"

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

// AssetParamsWithExtra adds byte arrays to store non UTF8 asset strings.
type AssetParamsWithExtra struct {
	sdk_types.AssetParams
	UnitNameBytes  []byte `codec:"un64"`
	AssetNameBytes []byte `codec:"an64"`
	URLBytes       []byte `codec:"au64"`
}

// ComputeMissing fills in any missing fields.
func (ap *AssetParamsWithExtra) ComputeMissing() {
	if len(ap.AssetName) > 0 {
		ap.AssetNameBytes = []byte(ap.AssetName)
	} else if len(ap.AssetNameBytes) > 0 {
		ap.AssetName = string(ap.AssetNameBytes)
	}

	if len(ap.UnitName) > 0 {
		ap.UnitNameBytes = []byte(ap.UnitName)
	} else if len(ap.UnitNameBytes) > 0 {
		ap.UnitName = string(ap.UnitNameBytes)
	}

	if len(ap.URL) > 0 {
		ap.URLBytes = []byte(ap.URL)
	} else if len(ap.URLBytes) > 0 {
		ap.URL = string(ap.URLBytes)
	}
}

// ConvertAssetParams sanitizes asset param string fields before encoding to JSON bytes.
func ConvertAssetParams(ap types.AssetParams) AssetParamsWithExtra {
	ret := AssetParamsWithExtra{
		AssetParams:    ap,
		AssetNameBytes: []byte(ap.AssetName),
		UnitNameBytes: []byte(ap.UnitName),
		URLBytes: []byte(ap.URL),
	}

	ret.AssetName = util.PrintableUTF8OrEmpty(ap.AssetName)
	ret.UnitName = util.PrintableUTF8OrEmpty(ap.UnitName)
	ret.URL = util.PrintableUTF8OrEmpty(ap.URL)

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
