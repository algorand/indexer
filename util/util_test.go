package util

import (
	"encoding/base64"
	"io/ioutil"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
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
		err = DecodeFromFile(pretty, &testDecode)
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
		err = DecodeFromFile(small, &testDecode)
		require.Equal(t, data, testDecode)
	}

	// gzip test
	{
		small := path.Join(tempdir, "small.json.gz")
		err := EncodeToFile(small, data, false)
		require.NoError(t, err)
		require.FileExists(t, small)
		var testDecode test
		err = DecodeFromFile(small, &testDecode)
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
