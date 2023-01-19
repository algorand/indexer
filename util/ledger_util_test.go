package util

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand/data/bookkeeping"
)

func TestReadGenesis(t *testing.T) {
	var reader io.Reader
	// nil reader
	_, err := ReadGenesis(reader)
	assert.Contains(t, err.Error(), "reader is nil")
	// no match struct field
	genesisStr := "{\"version\": 2}"
	reader = strings.NewReader(genesisStr)
	_, err = ReadGenesis(reader)
	assert.Contains(t, err.Error(), "json decode error")

	genesis := bookkeeping.Genesis{
		SchemaID:    "1",
		Network:     "test",
		Proto:       "test",
		RewardsPool: "AAAA",
		FeeSink:     "AAAA",
	}

	// read and decode genesis
	reader = strings.NewReader(string(json.Encode(genesis)))
	_, err = ReadGenesis(reader)
	assert.Nil(t, err)
	// read from empty reader
	_, err = ReadGenesis(reader)
	assert.Contains(t, err.Error(), "EOF")
}
