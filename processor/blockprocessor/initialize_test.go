package blockprocessor

import (
	"fmt"
	"os"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/sirupsen/logrus"
	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util"
	"github.com/algorand/indexer/util/test"
)

func TestRunMigration(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	// /genesis
	genesis := test.MakeGenesis()
	httpmock.RegisterResponder("GET", "http://localhost/genesis",
		httpmock.NewStringResponder(200, string(json.Encode(genesis))))
	// /v2/blocks/0 endpoint
	genesisBlock := test.MakeGenesisBlock()

	// responder for rounds 1 to 6
	txn := test.MakePaymentTxn(0, 100, 0, 1, 1,
		0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	prevHeader := genesisBlock.BlockHeader
	txn.Txn.GenesisHash = genesis.Hash()
	for i := 1; i < 7; i++ {
		block, err := test.MakeBlockForTxns(prevHeader, &txn)
		assert.Nil(t, err)
		blockCert := rpcs.EncodedBlockCert{
			Block: block,
		}
		url := fmt.Sprintf("http://localhost/v2/blocks/%d?format=msgpack", i)
		httpmock.RegisterResponder("GET", url,
			httpmock.NewStringResponder(200, string(msgpack.Encode(blockCert))))
		prevHeader = block.BlockHeader
	}

	httpmock.RegisterResponder("GET", "http://localhost/v2/blocks/7?format=msgpack",
		httpmock.NewStringResponder(404, string(json.Encode(algod.Status{}))))

	// /v2/status/wait-for-block-after/{round}
	httpmock.RegisterResponder("GET", `=~^http://localhost/v2/status/wait-for-block-after/\d+\z`,
		httpmock.NewStringResponder(200, string(json.Encode(algod.Status{}))))

	dname, _ := os.MkdirTemp("", "indexer")
	defer os.RemoveAll(dname)
	opts := idb.IndexerDbOptions{
		IndexerDatadir: dname,
		AlgodAddr:      "localhost",
		AlgodToken:     "AAAAA",
	}

	// migrate 3 rounds
	err := InitializeLedgerSimple(logrus.New(), 3, &opts)
	assert.NoError(t, err)
	log, _ := test2.NewNullLogger()
	l, err := util.MakeLedger(log, false, &genesis, opts.IndexerDatadir)
	assert.NoError(t, err)
	// check 3 rounds written to ledger
	assert.Equal(t, uint64(3), uint64(l.Latest()))
	l.Close()

	// migration continues from last round
	err = InitializeLedgerSimple(logrus.New(), 6, &opts)
	assert.NoError(t, err)

	l, err = util.MakeLedger(log, false, &genesis, opts.IndexerDatadir)
	assert.NoError(t, err)
	assert.Equal(t, uint64(6), uint64(l.Latest()))
	l.Close()
}