package localledger

import (
	"testing"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util/test"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestRunMigration(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	// mocked /genesis endpoint
	httpmock.RegisterResponder("GET", "http://localhost/genesis",
		httpmock.NewStringResponder(200, string(json.Encode(test.MakeGenesis()))))
	// mocked /v2/blocks/0 endpoint
	blockCert := rpcs.EncodedBlockCert{
		Block: test.MakeGenesisBlock(),
	}
	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/0?format=msgpack",
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert))))

	// mocked /v2/blocks/0 endpoint
	httpmock.RegisterResponder("GET", `=~^http://localhost/v2/blocks/\\d+?format=msgpack\\z`,
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert))))

	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/1?format=msgpack",
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert))))

	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/2?format=msgpack",
		httpmock.NewStringResponder(404, string(json.Encode(algod.Status{}))))

	httpmock.RegisterResponder("GET", "http://localhost/v2/status/wait-for-block-after/1",
		httpmock.NewStringResponder(200, string(json.Encode(algod.Status{}))))

	opts := idb.IndexerDbOptions{
		IndexerDatadir: "testdir",
		AlgodAddr:      "localhost",
		AlgodToken:     "AAAAA",
	}
	err := RunMigration(1, &opts)
	assert.NoError(t, err)
}
