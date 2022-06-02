package localledger

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	algodConfig "github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util"
	"github.com/algorand/indexer/util/test"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestRunMigration(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	// /genesis
	httpmock.RegisterResponder("GET", "http://localhost/genesis",
		httpmock.NewStringResponder(200, string(json.Encode(test.MakeGenesis()))))
	// /v2/blocks/0 endpoint
	blockCert := rpcs.EncodedBlockCert{
		Block: test.MakeGenesisBlock(),
	}
	// /v2/blocks/0
	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/0?format=msgpack",
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert))))

	// /v2/blocks/1
	txn := test.MakePaymentTxn(0, 100, 0, 1, 1,
		0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block1, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	assert.Nil(t, err)
	blockCert1 := rpcs.EncodedBlockCert{
		Block: block1,
	}
	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/1?format=msgpack",
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert1))))

	// /v2/blocks/2
	block2, err := test.MakeBlockForTxns(block1.BlockHeader, &txn)
	assert.Nil(t, err)
	blockCert2 := rpcs.EncodedBlockCert{
		Block: block2,
	}
	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/2?format=msgpack",
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert2))))

	// /v2/blocks/3
	block3, err := test.MakeBlockForTxns(block2.BlockHeader, &txn)
	assert.Nil(t, err)
	blockCert3 := rpcs.EncodedBlockCert{
		Block: block3,
	}
	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/3?format=msgpack",
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert3))))

	// /v2/blocks/4
	block4, err := test.MakeBlockForTxns(block3.BlockHeader, &txn)
	assert.Nil(t, err)
	blockCert4 := rpcs.EncodedBlockCert{
		Block: block4,
	}
	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/4?format=msgpack",
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert4))))

	// /v2/blocks/5
	block5, err := test.MakeBlockForTxns(block4.BlockHeader, &txn)
	assert.Nil(t, err)
	blockCert5 := rpcs.EncodedBlockCert{
		Block: block5,
	}
	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/5?format=msgpack",
		httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert5))))

	// /v2/blocks/6
	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/6?format=msgpack",
		httpmock.NewStringResponder(404, string(json.Encode(algod.Status{}))))

	// /v2/status/wait-for-block-after/{round}
	httpmock.RegisterResponder("GET", `=~^http://localhost/v2/status/wait-for-block-after/\d+\z`,
		httpmock.NewStringResponder(200, string(json.Encode(algod.Status{}))))

	opts := idb.IndexerDbOptions{
		IndexerDatadir: "testdir",
		AlgodAddr:      "localhost",
		AlgodToken:     "AAAAA",
	}
	// migrate 3 rounds
	err = RunMigration(3, &opts)
	assert.NoError(t, err)
	genesis := test.MakeGenesis()
	genesisBlock := test.MakeGenesisBlock()
	initState, err := util.CreateInitState(&genesis, &genesisBlock)
	assert.NoError(t, err)
	l, err := ledger.OpenLedger(logging.NewLogger(), filepath.Join(path.Dir(opts.IndexerDatadir), "ledger"), false, initState, algodConfig.GetDefaultLocal())
	assert.NoError(t, err)
	// check 3 rounds written to ledger
	assert.Equal(t, uint64(3), uint64(l.Latest()))
	l.Close()

	// migration continues from last round
	err = RunMigration(5, &opts)
	assert.NoError(t, err)
	l, err = ledger.OpenLedger(logging.NewLogger(), filepath.Join(path.Dir(opts.IndexerDatadir), "ledger"), false, initState, algodConfig.GetDefaultLocal())
	assert.NoError(t, err)
	assert.Equal(t, uint64(5), uint64(l.Latest()))
	l.Close()

	// remove ledger files
	ledgerFiles := []string{"ledger.block.sqlite", "ledger.block.sqlite-shm", "ledger.block.sqlite-wal",
		"ledger.tracker.sqlite", "ledger.tracker.sqlite-shm", "ledger.tracker.sqlite-wal"}

	for _, fn := range ledgerFiles {
		if _, err = os.Stat(fn); err == nil {
			err = os.Remove(fn)
			assert.NoError(t, err)
		}
	}

}
