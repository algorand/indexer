package filewriter

import (
	"io/ioutil"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeToAndFromFile(t *testing.T) {
	tempdir := t.TempDir()

	type test struct {
		One    string         `json:"one"`
		Two    int            `json:"two"`
		Strict map[int]string `json:"strict"`
	}
	data := test{
		One: "one",
		Two: 2,
		Strict: map[int]string{
			0: "int-key",
		},
	}

	{
		pretty := path.Join(tempdir, "pretty.json")
		err := EncodeJSONToFile(pretty, data, true)
		require.NoError(t, err)
		require.FileExists(t, pretty)
		var testDecode test
		err = DecodeJSONFromFile(pretty, &testDecode, false)
		require.Equal(t, data, testDecode)

		// Check the pretty printing
		bytes, err := ioutil.ReadFile(pretty)
		require.NoError(t, err)
		require.Contains(t, string(bytes), "  \"one\": \"one\",\n")
		require.Contains(t, string(bytes), `"0": "int-key"`)
	}

	{
		small := path.Join(tempdir, "small.json")
		err := EncodeJSONToFile(small, data, false)
		require.NoError(t, err)
		require.FileExists(t, small)
		var testDecode test
		err = DecodeJSONFromFile(small, &testDecode, false)
		require.Equal(t, data, testDecode)
	}

	// gzip test
	{
		small := path.Join(tempdir, "small.json.gz")
		err := EncodeJSONToFile(small, data, false)
		require.NoError(t, err)
		require.FileExists(t, small)
		var testDecode test
		err = DecodeJSONFromFile(small, &testDecode, false)
		require.Equal(t, data, testDecode)
	}
}
