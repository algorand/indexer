package blockprocessor

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
	"github.com/sirupsen/logrus"
	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
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

	dname, err := os.MkdirTemp("", "indexer")
	defer os.RemoveAll(dname)
	opts := idb.IndexerDbOptions{
		IndexerDatadir: dname,
		AlgodAddr:      "localhost",
		AlgodToken:     "AAAAA",
	}

	// migrate 3 rounds
	err = InitializeLedgerSimple(context.Background(), logrus.New(), 3, &opts)
	assert.NoError(t, err)
	log, _ := test2.NewNullLogger()
	l, err := util.MakeLedger(log, false, &genesis, opts.IndexerDatadir)
	assert.NoError(t, err)
	// check 3 rounds written to ledger
	assert.Equal(t, uint64(3), uint64(l.Latest()))
	l.Close()

	// migration continues from last round
	err = InitializeLedgerSimple(context.Background(), logrus.New(), 6, &opts)
	assert.NoError(t, err)

	l, err = util.MakeLedger(log, false, &genesis, opts.IndexerDatadir)
	assert.NoError(t, err)
	assert.Equal(t, uint64(6), uint64(l.Latest()))
	l.Close()
}

func TestInitializeLedgerFastCatchup_Errors(t *testing.T) {
	log, _ := test2.NewNullLogger()
	err := InitializeLedgerFastCatchup(context.Background(), log, "asdf", "", bookkeeping.Genesis{})
	require.EqualError(t, err, "InitializeLedgerFastCatchup() err: indexer data directory missing")

	err = InitializeLedgerFastCatchup(context.Background(), log, "asdf", t.TempDir(), bookkeeping.Genesis{})
	require.EqualError(t, err, "InitializeLedgerFastCatchup() err: catchpoint parsing failed")

	tryToRun := func(ctx context.Context) {
		var addr basics.Address
		genesis := bookkeeping.Genesis{
			SchemaID:    "1",
			Network:     "test",
			Proto:       "future",
			Allocation:  nil,
			RewardsPool: addr.String(),
			FeeSink:     addr.String(),
			Timestamp:   0,
			Comment:     "",
			DevMode:     false,
		}
		err = InitializeLedgerFastCatchup(
			ctx,
			logrus.New(),
			"21890000#BOGUSTCNVEDIBNRPNCKWRBQLJ7ILXIJBYKAHF67TLUOYRUGHW7ZA",
			t.TempDir(),
			genesis)
		require.EqualError(t, err, "InitializeLedgerFastCatchup() err: context canceled")
	}

	// Run with an immediate cancel
	ctx, cf := context.WithCancel(context.Background())
	cf() // cancel immediately
	tryToRun(ctx)

	// This should hit a couple extra branches
	ctx, cf = context.WithCancel(context.Background())
	go func() {
		time.Sleep(11 * time.Second)
		cf() // cancel immediately
	}()
	tryToRun(ctx)
}
