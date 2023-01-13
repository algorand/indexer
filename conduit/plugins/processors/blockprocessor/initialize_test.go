package blockprocessor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
	"github.com/sirupsen/logrus"
	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/conduit/plugins/processors/blockprocessor/internal"
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

	config := Config{
		LedgerDir:  t.TempDir(),
		AlgodAddr:  "localhost",
		AlgodToken: "AAAAA",
	}

	// migrate 3 rounds
	log, _ := test2.NewNullLogger()
	err := InitializeLedgerSimple(context.Background(), log, 3, &genesis, &config)
	assert.NoError(t, err)
	l, err := util.MakeLedger(log, false, &genesis, config.LedgerDir)
	assert.NoError(t, err)
	// check 3 rounds written to ledger
	assert.Equal(t, uint64(3), uint64(l.Latest()))
	l.Close()

	// migration continues from last round
	err = InitializeLedgerSimple(context.Background(), log, 6, &genesis, &config)
	assert.NoError(t, err)

	l, err = util.MakeLedger(log, false, &genesis, config.LedgerDir)
	assert.NoError(t, err)
	assert.Equal(t, uint64(6), uint64(l.Latest()))
	l.Close()
}

func TestInitializeLedgerFastCatchup_Errors(t *testing.T) {
	genesis := test.MakeGenesis()
	log, _ := test2.NewNullLogger()
	err := InitializeLedgerFastCatchup(context.Background(), log, "asdf", "", genesis)
	require.EqualError(t, err, "InitializeLedgerFastCatchup() err: indexer data directory missing")

	err = InitializeLedgerFastCatchup(context.Background(), log, "asdf", t.TempDir(), genesis)
	require.ErrorContains(t, err, "catchpoint parsing failed")

	tryToRun := func(ctx context.Context) {
		genesis := test.MakeGenesis()
		err = InitializeLedgerFastCatchup(
			ctx,
			logrus.New(),
			"21890000#BOGUSTCNVEDIBNRPNCKWRBQLJ7ILXIJBYKAHF67TLUOYRUGHW7ZA",
			t.TempDir(),
			genesis)
		require.ErrorContains(t, err, "context canceled")
	}

	// Run with an immediate cancel
	ctx, cf := context.WithCancel(context.Background())
	cf() // cancel immediately
	tryToRun(ctx)

	// This should hit a couple extra branches
	ctx, cf = context.WithCancel(context.Background())
	internal.Delay = 1 * time.Millisecond
	// cancel after a short delay
	go func() {
		time.Sleep(1 * time.Second)
		cf()
	}()
	tryToRun(ctx)
}
