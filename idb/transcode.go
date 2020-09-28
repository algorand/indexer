package idb

import (
	"fmt"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"

	"github.com/algorand/go-codec/codec"
)

func Stringify(ob interface{}) interface{} {
	switch v := ob.(type) {
	case map[interface{}]interface{}:
		return stringifyMap(v)
	case []interface{}:
		for i := range v {
			v[i] = Stringify(v[i])
		}
		return v
	default:
		return ob
	}
}

// modifes ob IN PLACE
func stringifyMap(ob map[interface{}]interface{}) map[interface{}]interface{} {
	out := make(map[interface{}]interface{}, len(ob))
	for tk, vv := range ob {
		switch k := tk.(type) {
		case string, []byte:
			out[k] = Stringify(vv)
		default:
			nk := fmt.Sprint(tk)
			out[nk] = Stringify(vv)
		}
	}
	return out
}

func MsgpackToJson(msgp []byte) (js []byte, err error) {
	var ob map[interface{}]interface{}
	err = msgpack.Decode(msgp, &ob)
	if err != nil {
		return
	}
	return JsonOneLine(Stringify(ob)), nil
}

func JsonOneLine(obj interface{}) []byte {
	var b []byte
	enc := codec.NewEncoderBytes(&b, OneLineJsonCodecHandle)
	enc.MustEncode(obj)
	return b
}

var OneLineJsonCodecHandle *codec.JsonHandle

func init() {
	// like github.com/algorand/go-algorand-sdk/encoding/json/json.go but no indent
	OneLineJsonCodecHandle = new(codec.JsonHandle)
	OneLineJsonCodecHandle.ErrorIfNoField = true
	OneLineJsonCodecHandle.ErrorIfNoArrayExpand = true
	OneLineJsonCodecHandle.Canonical = true
	OneLineJsonCodecHandle.RecursiveEmptyCheck = true
	OneLineJsonCodecHandle.Indent = 0
	OneLineJsonCodecHandle.HTMLCharsAsIs = true
}
