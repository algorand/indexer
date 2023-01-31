package util

import (
	"encoding/base64"
	"io/ioutil"
	"math/rand"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

func TestEncodeToAndFromFile(t *testing.T) {
	tempdir := t.TempDir()

	type test struct {
		One string `json:"one"`
		Two int    `json:"two"`
	}
	data := test{
		One: "one",
		Two: 2,
	}

	{
		pretty := path.Join(tempdir, "pretty.json")
		err := EncodeToFile(pretty, data, true)
		require.NoError(t, err)
		require.FileExists(t, pretty)
		var testDecode test
		err = DecodeFromFile(pretty, &testDecode, false)
		require.Equal(t, data, testDecode)

		// Check the pretty printing
		bytes, err := ioutil.ReadFile(pretty)
		require.NoError(t, err)
		require.Contains(t, string(bytes), "  \"one\": \"one\",\n")
	}

	{
		small := path.Join(tempdir, "small.json")
		err := EncodeToFile(small, data, false)
		require.NoError(t, err)
		require.FileExists(t, small)
		var testDecode test
		err = DecodeFromFile(small, &testDecode, false)
		require.Equal(t, data, testDecode)
	}

	// gzip test
	{
		small := path.Join(tempdir, "small.json.gz")
		err := EncodeToFile(small, data, false)
		require.NoError(t, err)
		require.FileExists(t, small)
		var testDecode test
		err = DecodeFromFile(small, &testDecode, false)
		require.Equal(t, data, testDecode)
	}
}

func TestPrintableUTF8OrEmpty(t *testing.T) {
	encodeArg := func(str string) string {
		return base64.StdEncoding.EncodeToString([]byte("input"))
	}
	tests := []struct {
		name   string
		argB64 string
		want   string
	}{
		{
			"Simple input",
			encodeArg("input"),
			"input",
		},
		{
			"Asset 510285544",
			"8J+qmSBNb25leSwgd2FudAo=",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg := tt.argB64

			// input is b64 encoded to support providing invalid inputs.
			if dec, err := base64.StdEncoding.DecodeString(tt.argB64); err == nil {
				arg = string(dec)
			}

			if got := PrintableUTF8OrEmpty(arg); got != tt.want {
				t.Errorf("PrintableUTF8OrEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeSignedTxnErrors(t *testing.T) {

	var b sdk.Block
	var tx sdk.SignedTxn

	// consensus protocol not found
	b.BlockHeader.CurrentProtocol = "test"
	_, err := EncodeSignedTxn(b.BlockHeader, tx, sdk.ApplyData{})
	require.Contains(t, err.Error(), "consensus protocol test not found")

	b.CurrentProtocol = "future"
	b.BlockHeader.GenesisID = "foo"

	// missing GenesisHash
	_, err = EncodeSignedTxn(b.BlockHeader, tx, sdk.ApplyData{})
	require.Contains(t, err.Error(), "GenesisHash required but missing")

	// gh mismatch
	_, err = rand.Read(b.BlockHeader.GenesisHash[:])
	require.NoError(t, err)
	_, err = rand.Read(tx.Txn.GenesisHash[:])
	require.NoError(t, err)
	_, err = EncodeSignedTxn(b.BlockHeader, tx, sdk.ApplyData{})
	require.Contains(t, err.Error(), "GenesisHash mismatch")

	// genesisID mismatch
	tx.Txn.GenesisHash = b.BlockHeader.GenesisHash
	tx.Txn.GenesisID = "bar"
	_, err = EncodeSignedTxn(b.BlockHeader, tx, sdk.ApplyData{})
	require.Contains(t, err.Error(), "GenesisID mismatch")

}

func TestDecodeSignedTxnErrors(t *testing.T) {

	var b sdk.Block
	var tx sdk.SignedTxn

	b.BlockHeader.GenesisID = "foo"
	_, err := rand.Read(b.BlockHeader.GenesisHash[:])
	require.NoError(t, err)
	b.CurrentProtocol = "future"
	tx.Txn.GenesisID = b.BlockHeader.GenesisID
	tx.Txn.GenesisHash = b.BlockHeader.GenesisHash
	txib, err := EncodeSignedTxn(b.BlockHeader, tx, sdk.ApplyData{})
	require.NoError(t, err)

	// v16.RequireGenesisHash = true
	b.BlockHeader.CurrentProtocol = "https://github.com/algorand/spec/tree/22726c9dcd12d9cddce4a8bd7e8ccaa707f74101"
	txib.HasGenesisHash = true
	_, ad, err := DecodeSignedTxn(b.BlockHeader, txib)
	require.Contains(t, err.Error(), "HasGenesisHash set to true but RequireGenesisHash obviates the flag")

	// gh not empty
	_, err = rand.Read(txib.Txn.GenesisHash[:])
	require.NoError(t, err)
	_, _, err = DecodeSignedTxn(b.BlockHeader, txib)
	require.Error(t, err)

	// if !proto.SupportSignedTxnInBlock, applyData is empty
	b.BlockHeader.CurrentProtocol = "v10"
	_, ad, err = DecodeSignedTxn(b.BlockHeader, txib)
	require.Equal(t, sdk.ApplyData{}, ad)

	// genesisID not empty
	txib.Txn.GenesisID = "foo"
	b.BlockHeader.CurrentProtocol = "future"
	_, _, err = DecodeSignedTxn(b.BlockHeader, txib)
	require.Contains(t, err.Error(), "GenesisID <foo> not empty")

	// consensus protocol not found
	b.BlockHeader.CurrentProtocol = "test"
	_, _, err = DecodeSignedTxn(b.BlockHeader, txib)
	require.Contains(t, err.Error(), "consensus protocol test not found")

}

func TestEncodeDecodeSignedTxn(t *testing.T) {

	var b sdk.Block
	b.BlockHeader.GenesisID = "foo"
	_, err := rand.Read(b.BlockHeader.GenesisHash[:])
	require.NoError(t, err)
	b.CurrentProtocol = "future"

	var tx sdk.SignedTxn
	tx.Txn.GenesisID = b.BlockHeader.GenesisID
	tx.Txn.GenesisHash = b.BlockHeader.GenesisHash

	txib, err := EncodeSignedTxn(b.BlockHeader, tx, sdk.ApplyData{})
	require.NoError(t, err)

	t2, _, err := DecodeSignedTxn(b.BlockHeader, txib)
	require.NoError(t, err)
	require.Equal(t, tx, t2)
}
