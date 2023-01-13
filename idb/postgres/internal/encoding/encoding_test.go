package encoding

import (
	"reflect"
	"testing"

	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/crypto/merklesignature"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	itypes "github.com/algorand/indexer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/types"
)

func TestEncodeSignedTxnWithAD(t *testing.T) {
	type txnMsgpackJSON struct {
		name    string
		msgpack []byte
		json    string
	}

	var testTxns = []txnMsgpackJSON{
		{
			"some fields",
			[]uint8{0x83, 0xa2, 0x64, 0x74, 0x81, 0xa2, 0x67, 0x64, 0x81, 0xa9, 0xfe, 0xfe, 0xff, 0xef, 0x0, 0x0, 0x11, 0x22, 0x33, 0x82, 0xa2, 0x61, 0x74, 0x1, 0xa2, 0x62, 0x73, 0xc4, 0x3, 0x78, 0x78, 0x78, 0xa3, 0x73, 0x69, 0x67, 0xc4, 0x40, 0x51, 0xca, 0x9f, 0x32, 0xca, 0x9d, 0x66, 0x4b, 0xde, 0xa0, 0x98, 0xd9, 0x1b, 0xd, 0xe, 0x4d, 0x39, 0xca, 0x2, 0x4c, 0x4e, 0xc4, 0xba, 0x88, 0x1a, 0xb6, 0xa, 0x63, 0xff, 0xb0, 0x95, 0xc6, 0xb6, 0x7d, 0x0, 0xb4, 0xdc, 0xef, 0x41, 0xe6, 0x3b, 0xc3, 0x43, 0x3e, 0xb5, 0xa2, 0xa0, 0x27, 0xad, 0x9c, 0xc0, 0x57, 0x93, 0x5c, 0x4e, 0xcd, 0x18, 0xea, 0xb0, 0x6b, 0xe3, 0x97, 0x17, 0x3, 0xa3, 0x74, 0x78, 0x6e, 0x8b, 0xa4, 0x61, 0x70, 0x61, 0x61, 0x91, 0xc4, 0x3, 0x78, 0x78, 0x78, 0xa4, 0x61, 0x70, 0x61, 0x70, 0xc4, 0x17, 0x2, 0x20, 0x1, 0x1, 0x26, 0x1, 0x9, 0xfe, 0xfe, 0xff, 0xef, 0x0, 0x0, 0x11, 0x22, 0x33, 0x28, 0x36, 0x1a, 0x0, 0x67, 0x22, 0x43, 0xa4, 0x61, 0x70, 0x67, 0x73, 0x81, 0xa3, 0x6e, 0x62, 0x73, 0x1, 0xa4, 0x61, 0x70, 0x73, 0x75, 0xc4, 0x5, 0x2, 0x20, 0x1, 0x1, 0x22, 0xa3, 0x66, 0x65, 0x65, 0xcd, 0x3, 0xe8, 0xa2, 0x66, 0x76, 0x4, 0xa2, 0x67, 0x68, 0xc4, 0x20, 0x8a, 0xae, 0xf2, 0xee, 0x8f, 0x3, 0x93, 0xb9, 0xa5, 0x47, 0x41, 0x35, 0x3b, 0x97, 0x96, 0xf3, 0xd, 0xcc, 0x52, 0x10, 0x9d, 0x21, 0x15, 0x9a, 0x64, 0xe8, 0x47, 0x52, 0xb2, 0xcc, 0x90, 0x6a, 0xa2, 0x6c, 0x76, 0xcd, 0x3, 0xec, 0xa4, 0x6e, 0x6f, 0x74, 0x65, 0xc4, 0x8, 0x13, 0xfa, 0x3c, 0x55, 0xe8, 0x7b, 0x23, 0xea, 0xa3, 0x73, 0x6e, 0x64, 0xc4, 0x20, 0x4a, 0x82, 0x63, 0xeb, 0xc0, 0xd2, 0xee, 0xed, 0xac, 0x73, 0xdb, 0xb9, 0xd0, 0x27, 0xa1, 0xb2, 0x32, 0x99, 0x7a, 0xed, 0xc5, 0xde, 0xa2, 0x25, 0x7f, 0x7f, 0x2c, 0x8b, 0xcd, 0x42, 0x5f, 0x1a, 0xa4, 0x74, 0x79, 0x70, 0x65, 0xa4, 0x61, 0x70, 0x70, 0x6c},
			`{"dt":{"gd":{"/v7/7wAAESIz":{"at":1,"bs":"eHh4"}}},"sig":"UcqfMsqdZkveoJjZGw0OTTnKAkxOxLqIGrYKY/+wlca2fQC03O9B5jvDQz61oqAnrZzAV5NcTs0Y6rBr45cXAw==","txn":{"apaa":["eHh4"],"apap":"AiABASYBCf7+/+8AABEiMyg2GgBnIkM=","apgs":{"nbs":1},"apsu":"AiABASI=","fee":1000,"fv":4,"gh":"iq7y7o8Dk7mlR0E1O5eW8w3MUhCdIRWaZOhHUrLMkGo=","lv":1004,"note":"E/o8Veh7I+o=","snd":"SoJj68DS7u2sc9u50CehsjKZeu3F3qIlf38si81CXxo=","type":"appl"}}`,
		},
		{
			"many fields",
			[]byte{0x83, 0xa2, 0x64, 0x74, 0x82, 0xa2, 0x67, 0x64, 0x82, 0xa3, 0x67, 0x6b, 0x62, 0x82, 0xa2, 0x61, 0x74, 0x1, 0xa2, 0x62, 0x73, 0xc4, 0x4, 0x74, 0x65, 0x73, 0x74, 0xa3, 0x67, 0x6b, 0x69, 0x82, 0xa2, 0x61, 0x74, 0x2, 0xa2, 0x75, 0x69, 0x64, 0xa2, 0x6c, 0x64, 0x81, 0x0, 0x82, 0xa3, 0x6c, 0x6b, 0x62, 0x82, 0xa2, 0x61, 0x74, 0x1, 0xa2, 0x62, 0x73, 0xc4, 0xb, 0x61, 0x6e, 0x6f, 0x74, 0x68, 0x65, 0x72, 0x74, 0x65, 0x73, 0x74, 0xa3, 0x6c, 0x6b, 0x69, 0x82, 0xa2, 0x61, 0x74, 0x2, 0xa2, 0x75, 0x69, 0xcc, 0xc8, 0xa3, 0x73, 0x69, 0x67, 0xc4, 0x40, 0xc9, 0x25, 0xb2, 0xa, 0x42, 0xda, 0x15, 0xbe, 0x74, 0x16, 0x1d, 0x45, 0xc9, 0x3b, 0xf, 0xa4, 0xcc, 0xdd, 0x86, 0xbd, 0xa, 0x53, 0x1e, 0x43, 0xb3, 0x7e, 0xf9, 0xcc, 0xaf, 0x44, 0x38, 0xce, 0x35, 0xa5, 0xaa, 0xb, 0x96, 0x28, 0x79, 0x6, 0xf8, 0xe1, 0xfb, 0x96, 0xe3, 0x79, 0x9b, 0x27, 0xfa, 0xa4, 0x51, 0x10, 0xc7, 0xb1, 0x84, 0x79, 0x46, 0xf8, 0xd8, 0x6a, 0x6c, 0x96, 0x93, 0x6, 0xa3, 0x74, 0x78, 0x6e, 0x8a, 0xa4, 0x61, 0x70, 0x61, 0x61, 0x91, 0xc4, 0x5, 0x66, 0x69, 0x72, 0x73, 0x74, 0xa4, 0x61, 0x70, 0x61, 0x6e, 0x1, 0xa4, 0x61, 0x70, 0x69, 0x64, 0x23, 0xa3, 0x66, 0x65, 0x65, 0xcd, 0x3, 0xe8, 0xa2, 0x66, 0x76, 0x7, 0xa2, 0x67, 0x68, 0xc4, 0x20, 0x8a, 0xae, 0xf2, 0xee, 0x8f, 0x3, 0x93, 0xb9, 0xa5, 0x47, 0x41, 0x35, 0x3b, 0x97, 0x96, 0xf3, 0xd, 0xcc, 0x52, 0x10, 0x9d, 0x21, 0x15, 0x9a, 0x64, 0xe8, 0x47, 0x52, 0xb2, 0xcc, 0x90, 0x6a, 0xa2, 0x6c, 0x76, 0xcd, 0x3, 0xef, 0xa4, 0x6e, 0x6f, 0x74, 0x65, 0xc4, 0x8, 0xc9, 0x83, 0x5, 0x5f, 0x20, 0x45, 0x8f, 0x98, 0xa3, 0x73, 0x6e, 0x64, 0xc4, 0x20, 0x32, 0xf8, 0xa1, 0x14, 0x66, 0x60, 0x7, 0xb7, 0xfe, 0x8, 0xd2, 0x48, 0x83, 0xdf, 0x28, 0x86, 0x16, 0x74, 0xa3, 0xb2, 0x5, 0x48, 0x1d, 0x4a, 0x45, 0x8e, 0x50, 0xb4, 0xba, 0x2f, 0x34, 0xde, 0xa4, 0x74, 0x79, 0x70, 0x65, 0xa4, 0x61, 0x70, 0x70, 0x6c},
			`{"dt":{"gd":{"/v7/7wAAESIz":{"at":1,"bs":"eHh4"},"Z2ti":{"at":1,"bs":"dGVzdA=="},"Z2tp":{"at":2,"ui":100}},"ld":{"0":{"bGti":{"at":1,"bs":"YW5vdGhlcnRlc3Q="},"bGtp":{"at":2,"ui":200}}}},"sig":"ySWyCkLaFb50Fh1FyTsPpMzdhr0KUx5Ds375zK9EOM41paoLlih5Bvjh+5bjeZsn+qRREMexhHlG+NhqbJaTBg==","txn":{"apaa":["Zmlyc3Q="],"apan":1,"apap":"AiABASYBCf7+/+8AABEiMyg2GgBnIkM=","apgs":{"nbs":1},"apid":35,"apsu":"AiABASI=","fee":1000,"fv":7,"gh":"iq7y7o8Dk7mlR0E1O5eW8w3MUhCdIRWaZOhHUrLMkGo=","lv":1007,"note":"yYMFXyBFj5g=","snd":"MvihFGZgB7f+CNJIg98ohhZ0o7IFSB1KRY5QtLovNN4=","type":"appl"}}`,
		},
		{
			"app call with inner transactions",
			[]byte{0x83, 0xa2, 0x64, 0x74, 0x81, 0xa3, 0x69, 0x74, 0x78, 0x91, 0x82, 0xa2, 0x64, 0x74, 0x81, 0xa3, 0x69, 0x74, 0x78, 0x91, 0x81, 0xa3, 0x74, 0x78, 0x6e, 0x84, 0xa4, 0x61, 0x61, 0x6d, 0x74, 0xcd, 0x1, 0xc8, 0xa4, 0x61, 0x72, 0x63, 0x76, 0xc4, 0x20, 0x32, 0x63, 0xff, 0x43, 0xce, 0xbb, 0x73, 0xa1, 0x61, 0x15, 0x11, 0x44, 0x39, 0xa6, 0x21, 0x0, 0x23, 0xe9, 0xe0, 0x89, 0x49, 0x1a, 0x44, 0x23, 0x66, 0xd5, 0x36, 0x93, 0xf7, 0x9e, 0x93, 0xc3, 0xa3, 0x73, 0x6e, 0x64, 0xc4, 0x20, 0x32, 0x63, 0xff, 0x43, 0xce, 0xbb, 0x73, 0xa1, 0x61, 0x15, 0x11, 0x44, 0x39, 0xa6, 0x21, 0x0, 0x23, 0xe9, 0xe0, 0x89, 0x49, 0x1a, 0x44, 0x23, 0x66, 0xd5, 0x36, 0x93, 0xf7, 0x9e, 0x93, 0xc3, 0xa4, 0x74, 0x79, 0x70, 0x65, 0xa5, 0x61, 0x78, 0x66, 0x65, 0x72, 0xa3, 0x74, 0x78, 0x6e, 0x84, 0xa3, 0x61, 0x6d, 0x74, 0x7b, 0xa3, 0x72, 0x63, 0x76, 0xc4, 0x20, 0x32, 0x63, 0xff, 0x43, 0xce, 0xbb, 0x73, 0xa1, 0x61, 0x15, 0x11, 0x44, 0x39, 0xa6, 0x21, 0x0, 0x23, 0xe9, 0xe0, 0x89, 0x49, 0x1a, 0x44, 0x23, 0x66, 0xd5, 0x36, 0x93, 0xf7, 0x9e, 0x93, 0xc3, 0xa3, 0x73, 0x6e, 0x64, 0xc4, 0x20, 0x32, 0x63, 0xff, 0x43, 0xce, 0xbb, 0x73, 0xa1, 0x61, 0x15, 0x11, 0x44, 0x39, 0xa6, 0x21, 0x0, 0x23, 0xe9, 0xe0, 0x89, 0x49, 0x1a, 0x44, 0x23, 0x66, 0xd5, 0x36, 0x93, 0xf7, 0x9e, 0x93, 0xc3, 0xa4, 0x74, 0x79, 0x70, 0x65, 0xa3, 0x70, 0x61, 0x79, 0xa3, 0x73, 0x69, 0x67, 0xc4, 0x40, 0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa3, 0x74, 0x78, 0x6e, 0x85, 0xa4, 0x61, 0x70, 0x61, 0x70, 0xc4, 0x5, 0x2, 0x20, 0x1, 0x1, 0x22, 0xa4, 0x61, 0x70, 0x73, 0x75, 0xc4, 0x5, 0x2, 0x20, 0x1, 0x1, 0x22, 0xa2, 0x67, 0x68, 0xc4, 0x20, 0x4d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa3, 0x73, 0x6e, 0x64, 0xc4, 0x20, 0x32, 0x63, 0xff, 0x43, 0xce, 0xbb, 0x73, 0xa1, 0x61, 0x15, 0x11, 0x44, 0x39, 0xa6, 0x21, 0x0, 0x23, 0xe9, 0xe0, 0x89, 0x49, 0x1a, 0x44, 0x23, 0x66, 0xd5, 0x36, 0x93, 0xf7, 0x9e, 0x93, 0xc3, 0xa4, 0x74, 0x79, 0x70, 0x65, 0xa4, 0x61, 0x70, 0x70, 0x6c},
			`{"dt":{"gd":{"/v7/7wAAESIz":{"at":1,"bs":"eHh4"},"Z2ti":{"at":1,"bs":"dGVzdA=="},"Z2tp":{"at":2,"ui":100}},"itx":[{"dt":{"itx":[{"txn":{"aamt":456,"arcv":"MmP/Q867c6FhFRFEOaYhACPp4IlJGkQjZtU2k/eek8M=","snd":"MmP/Q867c6FhFRFEOaYhACPp4IlJGkQjZtU2k/eek8M=","type":"axfer"}}]},"txn":{"amt":123,"rcv":"MmP/Q867c6FhFRFEOaYhACPp4IlJGkQjZtU2k/eek8M=","snd":"MmP/Q867c6FhFRFEOaYhACPp4IlJGkQjZtU2k/eek8M=","type":"pay"}}],"ld":{"0":{"bGti":{"at":1,"bs":"YW5vdGhlcnRlc3Q="},"bGtp":{"at":2,"ui":200}}}},"sig":"WAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==","txn":{"apaa":["Zmlyc3Q="],"apan":1,"apap":"AiABASI=","apgs":{"nbs":1},"apid":35,"apsu":"AiABASI=","fee":1000,"fv":7,"gh":"TQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","lv":1007,"note":"yYMFXyBFj5g=","snd":"MmP/Q867c6FhFRFEOaYhACPp4IlJGkQjZtU2k/eek8M=","type":"appl"}}`,
		},
	}

	var stxn sdk.SignedTxnWithAD
	for _, mt := range testTxns {
		t.Run(mt.name, func(t *testing.T) {
			msgpack.Decode(mt.msgpack, &stxn)
			js := EncodeSignedTxnWithAD(stxn)
			require.Equal(t, mt.json, string(js))
		})
	}
}

func TestEncodeSignedTxnWithADSynthetic(t *testing.T) {
	nonutf8b := []byte{254, 254, 255, 239, 0, 0, 17, 34, 51}
	nonutf8 := string(nonutf8b)
	var stxn sdk.SignedTxnWithAD
	stxn.EvalDelta.GlobalDelta = make(map[string]sdk.ValueDelta)
	stxn.EvalDelta.GlobalDelta[nonutf8] = sdk.ValueDelta{
		Action: sdk.SetBytesAction,
		Bytes:  string(nonutf8b),
	}
	stxn.EvalDelta.LocalDeltas = make(map[uint64]sdk.StateDelta, 1)
	ld := make(map[string]sdk.ValueDelta)
	ld[nonutf8] = sdk.ValueDelta{
		Action: sdk.SetBytesAction,
		Bytes:  string(nonutf8b),
	}
	stxn.EvalDelta.LocalDeltas[1] = ld
	js := EncodeSignedTxnWithAD(stxn)
	require.Equal(t, `{"dt":{"gd":{"/v7/7wAAESIz":{"at":1,"bs":"/v7/7wAAESIz"}},"ld":{"1":{"/v7/7wAAESIz":{"at":1,"bs":"/v7/7wAAESIz"}}}}}`, string(js))
}

// Test that encoding to JSON and decoding results in the same object.
func TestJSONEncoding(t *testing.T) {
	type Y struct {
		Num int
	}
	type T struct {
		Num    uint64
		Str    string
		Bytes  []byte
		Object Y
	}

	x := T{
		Num:    1,
		Str:    "abc",
		Bytes:  []byte{4, 5, 6},
		Object: Y{Num: 7},
	}
	buf := encodeJSON(x)

	var xx T
	err := DecodeJSON(buf, &xx)
	require.NoError(t, err)

	assert.Equal(t, x, xx)
}

// Test that encoding of Transaction is as expected and that decoding results in the same object.
func TestTransactionEncoding(t *testing.T) {
	i := byte(0)
	newaddr := func() sdk.Address {
		i++
		var address sdk.Address
		address[0] = i
		return address
	}

	tests := []struct {
		name     string
		params   sdk.SignedTxnWithAD
		expected string
	}{
		{
			name: "simple",
			params: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						AssetConfigTxnFields: sdk.AssetConfigTxnFields{
							AssetParams: sdk.AssetParams{
								Total:     99999,
								AssetName: "\r",
								UnitName:  "💰",
								URL:       "https://my.\000asset",
								Manager:   newaddr(),
								Reserve:   newaddr(),
								Freeze:    newaddr(),
								Clawback:  newaddr(),
							},
						},
					},
				},
			},
			expected: `{"txn":{"apar":{"an64":"DQ==","au64":"aHR0cHM6Ly9teS4AYXNzZXQ=","c":"BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","f":"AwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","m":"AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","r":"AgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","t":99999,"un":"💰"}}}`,
		},
	}

	for _, testcase := range tests {
		testcase := testcase
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()
			buf := EncodeSignedTxnWithAD(testcase.params)
			assert.Equal(t, testcase.expected, string(buf))

			paramsNew, err := DecodeSignedTxnWithAD(buf)
			require.NoError(t, err)
			assert.Equal(t, testcase.params, paramsNew)
		})
	}
}

// Test that encoding of AssetParams is as expected and that decoding results in the
// same object.
func TestAssetParamsEncoding(t *testing.T) {
	i := byte(0)
	newaddr := func() sdk.Address {
		i++
		var address sdk.Address
		address[0] = i
		return address
	}

	tests := []struct {
		name     string
		params   sdk.AssetParams
		expected string
	}{
		{
			name: "simple",
			params: sdk.AssetParams{
				Total:    99999,
				URL:      "https://my.asset",
				Manager:  newaddr(),
				Reserve:  newaddr(),
				Freeze:   newaddr(),
				Clawback: newaddr(),
			},
			expected: `{"au":"https://my.asset","c":"BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","f":"AwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","m":"AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","r":"AgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","t":99999}`,
		},
		{
			name: "embedded null / non-printable / emoji char",
			params: sdk.AssetParams{
				Total:     99999,
				AssetName: "\r",
				UnitName:  "💰",
				URL:       "https://my.\000asset",
				Manager:   newaddr(),
				Reserve:   newaddr(),
				Freeze:    newaddr(),
				Clawback:  newaddr(),
			},
			expected: `{"an64":"DQ==","au64":"aHR0cHM6Ly9teS4AYXNzZXQ=","c":"CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","f":"BwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","m":"BQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","r":"BgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","t":99999,"un":"💰"}`,
		},
	}

	for _, testcase := range tests {
		testcase := testcase
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()
			buf := EncodeAssetParams(testcase.params)
			assert.Equal(t, testcase.expected, string(buf))

			paramsNew, err := DecodeAssetParams(buf)
			require.NoError(t, err)
			assert.Equal(t, testcase.params, paramsNew)
		})
	}
}

// Test that encoding of BlockHeader is as expected and that decoding results in the
// same object.
func TestBlockHeaderEncoding(t *testing.T) {
	i := byte(0)
	newaddr := func() sdk.Address {
		i++
		var address sdk.Address
		address[0] = i
		return address
	}

	var branch sdk.BlockHash
	branch[0] = 5

	header := sdk.BlockHeader{
		Round:  3,
		Branch: branch,
		RewardsState: sdk.RewardsState{
			FeeSink:     newaddr(),
			RewardsPool: newaddr(),
		},
	}

	buf := EncodeBlockHeader(header)

	expectedString := `{"fees":"AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","prev":"BQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","rnd":3,"rwd":"AgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`
	assert.Equal(t, expectedString, string(buf))

	headerNew, err := DecodeBlockHeader(buf)
	require.NoError(t, err)
	assert.Equal(t, header, headerNew)
}

// Test that the encoding of byteArray in JSON is as expected and that decoding results in
// the same object.
func TestByteArrayEncoding(t *testing.T) {
	type T struct {
		ByteArray byteArray
		Map       map[byteArray]int
	}
	x := T{
		ByteArray: byteArray{string([]byte{0xff})}, // try a non-utf8 key
		Map: map[byteArray]int{
			{string([]byte{0xff})}: 3,
		},
	}
	buf := encodeJSON(x)

	expectedString := `{"ByteArray":"/w==","Map":{"/w==":3}}`
	assert.Equal(t, expectedString, string(buf))

	var xx T
	err := DecodeJSON(buf, &xx)
	require.NoError(t, err)
	assert.Equal(t, x, xx)
}

// Test that the encoding of SignedTxnWithAD is as expected and that decoding results in
// the same object.
func TestSignedTxnWithADEncoding(t *testing.T) {
	i := byte(0)
	newaddr := func() sdk.Address {
		i++
		var address sdk.Address
		address[0] = i
		return address
	}

	stxn := sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Header: sdk.Header{
					Sender:  newaddr(),
					RekeyTo: newaddr(),
				},
				PaymentTxnFields: sdk.PaymentTxnFields{
					Receiver:         newaddr(),
					CloseRemainderTo: newaddr(),
				},
				AssetConfigTxnFields: sdk.AssetConfigTxnFields{
					AssetParams: sdk.AssetParams{
						Manager:  newaddr(),
						Reserve:  newaddr(),
						Freeze:   newaddr(),
						Clawback: newaddr(),
					},
				},
				AssetTransferTxnFields: sdk.AssetTransferTxnFields{
					AssetSender:   newaddr(),
					AssetReceiver: newaddr(),
					AssetCloseTo:  newaddr(),
				},
				AssetFreezeTxnFields: sdk.AssetFreezeTxnFields{
					FreezeAccount: newaddr(),
				},
				ApplicationFields: sdk.ApplicationFields{
					ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
						Accounts: []sdk.Address{newaddr(), newaddr()},
					},
				},
			},
			AuthAddr: newaddr(),
		},
		ApplyData: sdk.ApplyData{
			EvalDelta: sdk.EvalDelta{
				GlobalDelta: map[string]sdk.ValueDelta{
					"abc": {
						Action: 44,
						Bytes:  "xyz",
						Uint:   33,
					},
				},
				LocalDeltas: map[uint64]sdk.StateDelta{
					2: map[string]sdk.ValueDelta{
						"bcd": {
							Action: 55,
							Bytes:  "yzx",
							Uint:   66,
						},
					},
				},
				Logs: []string{
					"xyz",
					"\000",
				},
				InnerTxns: []sdk.SignedTxnWithAD{{
					ApplyData: sdk.ApplyData{
						EvalDelta: sdk.EvalDelta{
							GlobalDelta: map[string]sdk.ValueDelta{
								"xyz": {
									Action: sdk.SetBytesAction,
									Bytes:  string("xyz"),
								},
							},
							LocalDeltas: map[uint64]sdk.StateDelta{
								1: {"xyz": {
									Action: sdk.SetBytesAction,
									Bytes:  string("xyz"),
								}},
							},
							Logs: []string{
								"xyz",
								"\000",
							},
						},
					},
				}},
			},
		},
	}
	buf := EncodeSignedTxnWithAD(stxn)

	expectedString := `{"dt":{"gd":{"YWJj":{"at":44,"bs":"eHl6","ui":33}},"itx":[{"dt":{"gd":{"eHl6":{"at":1,"bs":"eHl6"}},"ld":{"1":{"eHl6":{"at":1,"bs":"eHl6"}}},"lg":["eHl6","AA=="]}}],"ld":{"2":{"YmNk":{"at":55,"bs":"eXp4","ui":66}}},"lg":["eHl6","AA=="]},"sgnr":"DwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","txn":{"aclose":"CwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","apar":{"c":"CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","f":"BwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","m":"BQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","r":"BgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},"apat":["DQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","DgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="],"arcv":"CgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","asnd":"CQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","close":"BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","fadd":"DAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","rcv":"AwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","rekey":"AgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","snd":"AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}`
	assert.Equal(t, expectedString, string(buf))

	newStxn, err := DecodeSignedTxnWithAD(buf)
	require.NoError(t, err)

	assert.Equal(t, stxn, newStxn)
}

// Test that encoding of AccountData is as expected and that decoding results in the
// same object.
func TestAccountDataEncoding(t *testing.T) {
	var addr basics.Address
	addr[0] = 3

	ad := basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 22},
		AuthAddr:   addr,
	}

	buf := EncodeTrimmedAccountData(ad)

	expectedString := `{"algo":22,"spend":"AwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`
	assert.Equal(t, expectedString, string(buf))

	adNew, err := DecodeTrimmedAccountData(buf)
	require.NoError(t, err)
	assert.Equal(t, ad, adNew)
}

// Test that encoding of AppLocalState is as expected and that decoding results in the
// same object.
func TestAppLocalStateEncoding(t *testing.T) {
	state := basics.AppLocalState{
		Schema: basics.StateSchema{
			NumUint: 2,
		},
		KeyValue: map[string]basics.TealValue{
			string([]byte{0xff}): { // try a non-utf8 key
				Type: 3,
			},
		},
	}

	buf := EncodeAppLocalState(state)

	expectedString := `{"hsch":{"nui":2},"tkv":{"/w==":{"tt":3}}}`
	assert.Equal(t, expectedString, string(buf))

	stateNew, err := DecodeAppLocalState(buf)
	require.NoError(t, err)
	assert.Equal(t, state, stateNew)
}

// Test that encoding of AppLocalState is as expected and that decoding results in the
// same object.
func TestAppParamsEncoding(t *testing.T) {
	params := basics.AppParams{
		ApprovalProgram: []byte{0xff}, // try a non-utf8 key
		GlobalState: map[string]basics.TealValue{
			string([]byte{0xff}): { // try a non-utf8 key
				Type: 3,
			},
		},
	}

	buf := EncodeAppParams(params)

	expectedString := `{"approv":"/w==","gs":{"/w==":{"tt":3}}}`
	assert.Equal(t, expectedString, string(buf))

	paramsNew, err := DecodeAppParams(buf)
	require.NoError(t, err)
	assert.Equal(t, params, paramsNew)
}

// Test that encoding of AppLocalState is as expected and that decoding results in the
// same object.
func TestSpecialAddressesEncoding(t *testing.T) {
	i := byte(0)
	newaddr := func() sdk.Address {
		i++
		var address sdk.Address
		address[0] = i
		return address
	}

	special := itypes.SpecialAddresses{
		FeeSink:     newaddr(),
		RewardsPool: newaddr(),
	}

	buf := EncodeSpecialAddresses(special)

	expectedString := `{"FeeSink":"AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","RewardsPool":"AgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`
	assert.Equal(t, expectedString, string(buf))

	specialNew, err := DecodeSpecialAddresses(buf)
	require.NoError(t, err)
	assert.Equal(t, special, specialNew)
}

func TestTxnExtra(t *testing.T) {
	tests := []struct {
		name     string
		extra    idb.TxnExtra
		expected string
	}{
		{
			name: "basic",
			extra: idb.TxnExtra{
				AssetCloseAmount: 3,
				RootIntra:        idb.OptionalUint{Present: true, Value: 4},
				RootTxid:         "abc",
			},
			expected: `{"aca":3,"root-intra":"4","root-txid":"abc"}`,
		},
		{
			name: "root_intra_zero",
			extra: idb.TxnExtra{
				RootIntra: idb.OptionalUint{Present: true, Value: 0},
			},
			expected: `{"root-intra":"0"}`,
		},
		{
			name:     "no_root_intra",
			extra:    idb.TxnExtra{},
			expected: `{}`,
		},
	}

	for _, testcase := range tests {
		testcase := testcase
		t.Run(testcase.name, func(t *testing.T) {
			buf := EncodeTxnExtra(&testcase.extra)
			assert.Equal(t, testcase.expected, string(buf))

			extraNew, err := DecodeTxnExtra(buf)
			require.NoError(t, err)
			assert.Equal(t, testcase.extra, extraNew)
		})
	}
}

// Test that encoding of NetworkState is as expected and that decoding results in the
// same object.
func TestNetworkStateEncoding(t *testing.T) {
	network := types.NetworkState{
		GenesisHash: sdk.Digest{77},
	}

	buf := EncodeNetworkState(&network)

	expectedString := `{"genesis-hash":"TQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`
	assert.Equal(t, expectedString, string(buf))

	decodedNetwork, err := DecodeNetworkState(buf)
	require.NoError(t, err)
	assert.Equal(t, network, decodedNetwork)
}

// Test that encoding of ledgercore.AccountData is as expected and that decoding
// results in the same object.
func TestLcAccountDataEncoding(t *testing.T) {
	var authAddr basics.Address
	authAddr[0] = 6

	var voteID crypto.OneTimeSignatureVerifier
	voteID[0] = 14

	var selectionID crypto.VRFVerifier
	selectionID[0] = 15

	var stateProofID merklesignature.Commitment
	stateProofID[0] = 19

	ad := ledgercore.AccountData{
		AccountBaseData: ledgercore.AccountBaseData{
			Status:   basics.Online,
			AuthAddr: authAddr,
			TotalAppSchema: basics.StateSchema{
				NumUint:      7,
				NumByteSlice: 8,
			},
			TotalExtraAppPages:  9,
			TotalAppParams:      10,
			TotalAppLocalStates: 11,
			TotalAssetParams:    12,
			TotalAssets:         13,
			TotalBoxes:          20,
			TotalBoxBytes:       21,
		},
		VotingData: ledgercore.VotingData{
			VoteID:          voteID,
			SelectionID:     selectionID,
			StateProofID:    stateProofID,
			VoteFirstValid:  16,
			VoteLastValid:   17,
			VoteKeyDilution: 18,
		},
	}
	buf := EncodeTrimmedLcAccountData(ad)

	expectedString := `{"onl":1,"sel":"DwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","spend":"BgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","stprf":"EwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==","tapl":11,"tapp":10,"tas":13,"tasp":12,"tbx":20,"tbxb":21,"teap":9,"tsch":{"nbs":8,"nui":7},"vote":"DgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","voteFst":16,"voteKD":18,"voteLst":17}`
	assert.Equal(t, expectedString, string(buf))

	decodedAd, err := DecodeTrimmedLcAccountData(buf)
	require.NoError(t, err)
	assert.Equal(t, ad, decodedAd)
}

// structFields recursively gets all field names in a struct
func structFields(theStruct interface{}, skip map[string]bool, names map[string]bool) {
	rStruct := reflect.TypeOf(theStruct)
	numFields := rStruct.NumField()
	for i := 0; i < numFields; i++ {
		field := rStruct.Field(i)
		name := field.Name
		if totalSkip, nameSkip := skip[name]; nameSkip {
			if totalSkip {
				continue
			}
		} else {
			names[name] = true
		}
		if field.Type.Kind() == reflect.Struct {
			structFields(reflect.New(field.Type).Elem().Interface(), skip, names)
		}
	}
}

// Test that all fields in go-algorand's AccountBaseData are either in local baseAccountData
// or are accounted for explicitly in "skip"
func TestBaseAccountDataVersusAccountBaseDataParity(t *testing.T) {

	skip := map[string]bool{
		"_struct":               true,
		"MicroAlgos":            true,
		"RewardsBase":           true,
		"RewardedMicroAlgos":    true,
		"MicroAlgosWithRewards": true,
		"VotingData":            false, // skip the name, but continue with the recursion
	}

	goalNames := map[string]bool{}
	structFields(ledgercore.AccountBaseData{}, skip, goalNames)
	structFields(ledgercore.OnlineAccountData{}, skip, goalNames)

	indexerNames := map[string]bool{}
	structFields(baseAccountData{}, skip, indexerNames)

	for name := range goalNames {
		require.Contains(t, indexerNames, name)
	}
}
