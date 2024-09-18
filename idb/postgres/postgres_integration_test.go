package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-codec/codec"
	"github.com/algorand/indexer/v3/api/generated/v2"
	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/v3/idb/postgres/internal/schema"
	pgtest "github.com/algorand/indexer/v3/idb/postgres/internal/testing"
	pgutil "github.com/algorand/indexer/v3/idb/postgres/internal/util"
	"github.com/algorand/indexer/v3/types"
	"github.com/algorand/indexer/v3/util"
	"github.com/algorand/indexer/v3/util/test"

	crypto2 "github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/v2/protocol"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// TestMaxRoundOnUninitializedDB makes sure we return 0 when getting the max round on a new DB.
func TestMaxRoundOnUninitializedDB(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	db, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	defer db.Close()

	round, err := db.GetNextRoundToAccount()
	assert.Equal(t, idb.ErrorNotInitialized, err)
	assert.Equal(t, uint64(0), round)

	round, err = db.getMaxRoundAccounted(context.Background(), nil)
	assert.Equal(t, idb.ErrorNotInitialized, err)
	assert.Equal(t, uint64(0), round)
}

// TestMaxRound the happy path.
func TestMaxRound(t *testing.T) {
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	pdb, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	defer pdb.Close()

	_, err = db.Exec(
		context.Background(),
		`INSERT INTO metastate (k, v) values ($1, $2)`,
		"state",
		`{"next_account_round":123454322}`)
	assert.NoError(t, err)

	round, err := pdb.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(123454322), round)

	round, err = pdb.getMaxRoundAccounted(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(123454321), round)
}

func TestAccountedRoundNextRound0(t *testing.T) {
	db, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	pdb, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	defer pdb.Close()

	_, err = db.Exec(
		context.Background(),
		`INSERT INTO metastate (k, v) values ($1, $2)`,
		"state",
		`{"next_account_round":0}`)
	assert.NoError(t, err)

	round, err := pdb.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)

	round, err = pdb.getMaxRoundAccounted(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)
}

// TestMaxConnection tests that when setting the maximum connection to a value, that it is
// accurately set and that acquiring connections accurately depletes the pool
func TestMaxConnection(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	// Open Postgres with a maximum of 2 connections locally
	pdb, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{MaxConn: 2}, nil)
	assert.NoError(t, err)
	defer pdb.Close()

	s := pdb.db.Stat()

	assert.Equal(t, 2, int(s.MaxConns()))
	assert.Equal(t, 0, int(s.AcquiredConns()))
	assert.GreaterOrEqual(t, 1, int(s.IdleConns()))

	conn1, err := pdb.db.Acquire(context.Background())
	assert.NoError(t, err)
	defer conn1.Release()

	s = pdb.db.Stat()
	assert.Equal(t, 1, int(s.AcquiredConns()))

	conn2, err := pdb.db.Acquire(context.Background())
	assert.NoError(t, err)
	defer conn2.Release()

	s = pdb.db.Stat()

	// We have reached the total of 2 "total" max connections
	// Meaning we should have two acquired connections and 0 idle connections
	assert.Equal(t, 2, int(s.AcquiredConns()))
	assert.Equal(t, 0, int(s.IdleConns()))

}

func assertAccountAsset(t *testing.T, db *pgxpool.Pool, addr sdk.Address, assetid uint64, frozen bool, amount uint64) {
	var row pgx.Row
	var f bool
	var a uint64

	row = db.QueryRow(context.Background(), `SELECT frozen, amount FROM account_asset as a WHERE a.addr = $1 AND assetid = $2`, addr[:], assetid)
	err := row.Scan(&f, &a)
	assert.NoError(t, err, "failed looking up AccountA.")
	assert.Equal(t, frozen, f)
	assert.Equal(t, amount, a)
}

// TestAssetCloseReopenTransfer tests a scenario that requires asset subround accounting
func TestAssetCloseReopenTransfer(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64(1)
	amt := uint64(10000)
	total := uint64(1000000)

	///////////
	// Given // A round scenario requiring subround accounting: AccountA is funded, closed, opts back, and funded again.
	///////////
	// createAsset := test.MakeAssetConfigTxn(0, total, uint64(6), false, "mcn", "my coin", "http://antarctica.com", test.AccountD)
	// optInA1 := test.MakeAssetOptInTxn(assetid, test.AccountA)
	// optInA2 := test.MakeAssetOptInTxn(assetid, test.AccountA)
	// fundA := test.MakeAssetTransferTxn(assetid, amt, test.AccountD, test.AccountA, basics.Address{})
	// optInB := test.MakeAssetOptInTxn(assetid, test.AccountB)
	// optInC := test.MakeAssetOptInTxn(assetid, test.AccountC)
	// closeA := test.MakeAssetTransferTxn(assetid, 1000, test.AccountA, test.AccountB, test.AccountC)
	// payMain := test.MakeAssetTransferTxn(assetid, amt, test.AccountD, test.AccountA, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AssetCloseReopenTransfer.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// Then // Accounts A, B, C and D have the correct balances.
	//////////
	// A has the final payment after being closed out
	assertAccountAsset(t, db.db, test.AccountA, assetid, false, amt)
	// B has the closing transfer amount
	assertAccountAsset(t, db.db, test.AccountB, assetid, false, 1000)
	// C has the close-to remainder
	assertAccountAsset(t, db.db, test.AccountC, assetid, false, 9000)
	// D has the total minus both payments to A
	assertAccountAsset(t, db.db, test.AccountD, assetid, false, total-2*amt)
}

// TestReCreateAssetHolding checks the optin value of a defunct
func TestReCreateAssetHolding(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given //
	// A new asset with default-frozen, AccountB opts-in and has its frozen state
	// toggled.
	///////////Then AccountB opts-out then opts-in again.
	// createAssetFrozen := test.MakeAssetConfigTxn(
	//	0, total, uint64(6), frozen, "icicles", "frozen coin",
	//	"http://antarctica.com", test.AccountA)
	// optinB1 := test.MakeAssetOptInTxn(assetid, test.AccountB)
	// optinB2 := test.MakeAssetOptInTxn(assetid, test.AccountB)
	//unfreezeB := test.MakeAssetFreezeTxn(
	//	assetid, !frozen, test.AccountA, test.AccountB)
	// optoutB := test.MakeAssetTransferTxn(
	//	assetid, 0, test.AccountB, test.AccountC, test.AccountD)
	{
		// frozen:true
		vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/ReCreateAssetHoldingFrozen.vb")
		require.NoError(t, err)
		err = db.AddBlock(&vb)
		require.NoError(t, err)
		//////////
		// Then // AccountB should have its frozen state set back to the default value
		//////////
		assertAccountAsset(t, db.db, test.AccountB, 1, true, 0)
	}
	{
		// frozen:false
		vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/ReCreateAssetHolding.vb")
		require.NoError(t, err)
		err = db.AddBlock(&vb)
		require.NoError(t, err)
		//////////
		// Then // AccountB should have its frozen state set back to the default value
		//////////
		assertAccountAsset(t, db.db, test.AccountB, 6, false, 0)
	}
}

// TestMultipleAssetOptins make sure no-op transactions don't reset the default frozen value.
func TestNoopOptins(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given //
	// An asset with default-frozen = true, AccountB opts in, is unfrozen, then has a
	// no-op opt-in
	///////////
	assetid := uint64(1)
	// createAsset := test.MakeAssetConfigTxn(
	//	0, uint64(1000000), uint64(6), true, "icicles", "frozen coin",
	//	"http://antarctica.com", test.AccountD)
	// optinB1 := test.MakeAssetOptInTxn(assetid, test.AccountB)
	// optinB2 := test.MakeAssetOptInTxn(assetid, test.AccountB)
	// unfreezeB := test.MakeAssetFreezeTxn(assetid, false, test.AccountD, test.AccountB)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/NoopOptins.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// Then // AccountB should have its frozen state set back to the default value
	//////////
	assertAccountAsset(t, db.db, test.AccountB, assetid, false, 0)
}

// TestMultipleWriters tests that accounting cannot be double committed.
func TestMultipleWriters(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	amt := uint64(10000)

	///////////
	// Given // Send amt to AccountE
	///////////
	// payAccountE := test.MakePaymentTxn(
	//	1000, amt, 0, 0, 0, 0, test.AccountD, test.AccountE, basics.Address{},
	//	basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/MultipleWriters.vb")
	require.NoError(t, err)

	//////////
	// When // We attempt commit the round accounting multiple times.
	//////////
	start := make(chan struct{})
	commits := 10
	errors := make(chan error, commits)
	var wg sync.WaitGroup
	for i := 0; i < commits; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errors <- db.AddBlock(&vb)
		}()
	}
	close(start)

	wg.Wait()
	close(errors)

	//////////
	// Then // There should be num-1 errors, and AccountA should only be paid once.
	//////////
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}
	assert.Equal(t, commits-1, errorCount)

	// AccountE should contain the final payment.
	var balance uint64
	row := db.db.QueryRow(context.Background(), `SELECT microalgos FROM account WHERE account.addr = $1`, test.AccountE[:])
	err = row.Scan(&balance)
	assert.NoError(t, err, "checking balance")
	assert.Equal(t, amt, balance)
}

// TestTransactionsTimestamps tests that the transactions endpoint responds to times properly.
func TestTransactionsTimestamps(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	round := uint64(1)
	///////////
	// Given // A block with 8 transactions at ts 1671036853.
	///////////
	usEastTz, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	usWestTz, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BlockWithTransactions.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We call Transactions with timestamp filters
	//////////
	blkTime := vb.Block.BlockHeader.TimeStamp
	fullRowsCh, _ := db.Transactions(
		context.Background(),
		idb.TransactionFilter{
			Round:      &round,
			BeforeTime: time.Unix(blkTime+1, 0).In(usEastTz),
			AfterTime:  time.Unix(blkTime-1, 0)})
	var txnRows0 []idb.TxnRow
	for row := range fullRowsCh {
		require.NoError(t, row.Error)
		txnRows0 = append(txnRows0, row)
	}

	emptyRowsCh, _ := db.Transactions(
		context.Background(),
		idb.TransactionFilter{
			Round:     &round,
			AfterTime: time.Unix(blkTime, 0).In(usWestTz)})
	txnRows1 := make([]idb.TxnRow, 0)
	for row := range emptyRowsCh {
		require.NoError(t, row.Error)
		txnRows1 = append(txnRows1, row)
	}

	//////////
	// Then // They should have the correct number of transactions
	//////////
	assert.Len(t, txnRows0, len(vb.Block.Payset))
	assert.Len(t, txnRows1, 0)
}

// TestBlockWithTransactions tests that the block with transactions endpoint works.
func TestBlockWithTransactions(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	round := uint64(1)
	///////////
	// Given // A block at round `round` with 5 transactions.
	///////////
	// txn1 := test.MakeAssetConfigTxn(
	//	0, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com",
	//	test.AccountD)
	// txn2 := test.MakeAssetOptInTxn(assetid, test.AccountA)
	// txn3 := test.MakeAssetTransferTxn(
	//	assetid, amt, test.AccountD, test.AccountA, basics.Address{})
	// txn4 := test.MakeAssetOptInTxn(assetid, test.AccountB)
	// txn5 := test.MakeAssetOptInTxn(assetid, test.AccountC)
	// txn6 := test.MakeAssetTransferTxn(
	//	assetid, 1000, test.AccountA, test.AccountB, test.AccountC)
	// txn7 := test.MakeAssetTransferTxn(
	//	assetid, 0, test.AccountA, test.AccountA, basics.Address{})
	// txn8 := test.MakeAssetTransferTxn(
	//	assetid, amt, test.AccountD, test.AccountA, basics.Address{})
	// txns := []*transactions.SignedTxnWithAD{
	//	&txn1, &txn2, &txn3, &txn4, &txn5, &txn6, &txn7, &txn8}
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BlockWithTransactions.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// When // We call GetBlock and Transactions
	//////////
	_, txnRows0, err := db.GetBlock(context.Background(), round, idb.GetBlockOptions{Transactions: true, MaxTransactionsLimit: 100})
	require.NoError(t, err)

	rowsCh, _ := db.Transactions(context.Background(), idb.TransactionFilter{Round: &round})
	txnRows1 := make([]idb.TxnRow, 0)
	for row := range rowsCh {
		require.NoError(t, row.Error)
		txnRows1 = append(txnRows1, row)
	}

	//////////
	// Then // They should have the correct transactions
	//////////
	var txns []sdk.SignedTxn
	for _, stxib := range vb.Block.Payset {
		stxn, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, stxib)
		require.NoError(t, err)
		txns = append(txns, stxn)
	}
	assert.Len(t, txnRows0, len(txns))
	assert.Len(t, txnRows1, len(txns))
	for i := 0; i < len(txnRows0); i++ {
		expected := base64.StdEncoding.EncodeToString(msgpack.Encode(txns[i]))
		actual := base64.StdEncoding.EncodeToString(msgpack.Encode(txnRows0[i].Txn))
		assert.Equal(t, expected, actual)

		actual = base64.StdEncoding.EncodeToString(msgpack.Encode(txnRows1[i].Txn))
		assert.Equal(t, expected, actual)
	}
}

func TestRekeyBasic(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	// txn := test.MakePaymentTxn(
	//	1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, test.AccountB)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/RekeyBasic.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)
	//////////
	// Then // Account A is rekeyed to account B
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(context.Background(), `SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	ad, err := encoding.DecodeTrimmedLcAccountData(accountDataStr)
	require.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, sdk.Address(test.AccountB), ad.AuthAddr)
}

func TestRekeyToItself(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transactions
	///////////
	// txn := test.MakePaymentTxn(
	//	1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, test.AccountB)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/RekeyToItself.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// Then // Account's A auth-address is not recorded
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(context.Background(), `SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	ad, err := encoding.DecodeTrimmedLcAccountData(accountDataStr)
	require.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, sdk.Address{}, ad.AuthAddr)
}

func TestRekeyThreeTimesInSameRound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	// txn0 := test.MakePaymentTxn(
	//	1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{},
	//	test.AccountB)
	// txn1 := test.MakePaymentTxn(
	//	1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{},
	//	basics.Address{})
	// txn2 := test.MakePaymentTxn(
	//	1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, test.AccountC)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/RekeyThreeTimesInSameRound.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// Then // Account A is rekeyed to account C
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(context.Background(), `SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	ad, err := encoding.DecodeTrimmedLcAccountData(accountDataStr)
	require.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, sdk.Address(test.AccountC), ad.AuthAddr)
}

func TestRekeyToItselfHasNotBeenRekeyed(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	// txn := test.MakePaymentTxn(
	//	1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{},
	//	basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/RekeyToItselfHasNotBeenRekeyed.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)
}

// TestIgnoreDefaultFrozenConfigUpdate the creator asset holding should ignore default-frozen = true.
func TestIgnoreDefaultFrozenConfigUpdate(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64(1)
	total := uint64(1000000)

	///////////
	// Given // A new asset with default-frozen = true, and AccountB opting into it.
	///////////
	// createAssetNotFrozen := test.MakeAssetConfigTxn(
	//	0, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com",
	//	test.AccountA)
	// modifyAssetToFrozen := test.MakeAssetConfigTxn(
	//	assetid, total, uint64(6), true, "icicles", "frozen coin", "http://antarctica.com",
	//	test.AccountA)
	// optin := test.MakeAssetOptInTxn(assetid, test.AccountB)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/IgnoreDefaultFrozenConfigUpdate.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// Then // Make sure the accounts have the correct default-frozen after create/optin
	//////////
	// default-frozen = true
	assertAccountAsset(t, db.db, test.AccountA, assetid, false, total)
	assertAccountAsset(t, db.db, test.AccountB, assetid, false, 0)
}

// TestZeroTotalAssetCreate tests that the asset holding with total of 0 is created.
func TestZeroTotalAssetCreate(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64(1)

	///////////
	// Given // A new asset with total = 0.
	///////////
	// createAsset := test.MakeAssetConfigTxn(
	//	0, total, uint64(6), false, "mcn", "my coin", "http://antarctica.com",
	//	test.AccountA)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/ZeroTotalAssetCreate.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// Then // Make sure the creator has an asset holding with amount = 0.
	//////////
	assertAccountAsset(t, db.db, test.AccountA, assetid, false, 0)
}

func assertAssetDates(t *testing.T, db *pgxpool.Pool, assetID uint64, deleted sql.NullBool, createdAt sql.NullInt64, closedAt sql.NullInt64) {
	row := db.QueryRow(
		context.Background(),
		"SELECT deleted, created_at, closed_at FROM asset WHERE index = $1", int64(assetID))

	var retDeleted sql.NullBool
	var retCreatedAt sql.NullInt64
	var retClosedAt sql.NullInt64
	err := row.Scan(&retDeleted, &retCreatedAt, &retClosedAt)
	assert.NoError(t, err)

	assert.Equal(t, deleted, retDeleted)
	assert.Equal(t, createdAt, retCreatedAt)
	assert.Equal(t, closedAt, retClosedAt)
}

func assertAssetHoldingDates(t *testing.T, db *pgxpool.Pool, address sdk.Address, assetID uint64, deleted sql.NullBool, createdAt sql.NullInt64, closedAt sql.NullInt64) {
	row := db.QueryRow(
		context.Background(),
		"SELECT deleted, created_at, closed_at FROM account_asset WHERE "+
			"addr = $1 AND assetid = $2",
		address[:], assetID)

	var retDeleted sql.NullBool
	var retCreatedAt sql.NullInt64
	var retClosedAt sql.NullInt64
	err := row.Scan(&retDeleted, &retCreatedAt, &retClosedAt)
	assert.NoError(t, err)

	assert.Equal(t, deleted, retDeleted)
	assert.Equal(t, createdAt, retCreatedAt)
	assert.Equal(t, closedAt, retClosedAt)
}

func TestDestroyAssetBasic(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetID := uint64(1)

	// Create an asset.
	// txn := test.MakeAssetConfigTxn(0, 4, 0, false, "uu", "aa", "", test.AccountA)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/DestroyAssetBasicCreate.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// Destroy an asset.
	// txn0: AssetDestroyTxn
	vb2, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/DestroyAssetBasicDelete.vb")
	require.NoError(t, err)
	vb2.Block.BlockHeader.Round = sdk.Round(2)
	err = db.AddBlock(&vb2)
	require.NoError(t, err)

	// Check that the asset is deleted.
	assertAssetDates(t, db.db, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: 1},
		sql.NullInt64{Valid: true, Int64: 2})

	// Check that the account's asset holding is deleted.
	assertAssetHoldingDates(t, db.db, test.AccountA, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: 1},
		sql.NullInt64{Valid: true, Int64: 2})
}

func TestDestroyAssetZeroSupply(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetID := uint64(1)

	// Create an asset. Set total supply to 0.
	//txn0 := test.MakeAssetConfigTxn(0, 0, 0, false, "uu", "aa", "", test.AccountA)
	//txn1 := test.MakeAssetDestroyTxn(assetID, test.AccountA)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/DestroyAssetZeroSupply.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// Check that the asset is deleted.
	assertAssetDates(t, db.db, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: 1},
		sql.NullInt64{Valid: true, Int64: 1})

	// Check that the account's asset holding is deleted.
	assertAssetHoldingDates(t, db.db, test.AccountA, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: 1},
		sql.NullInt64{Valid: true, Int64: 1})
}

func TestDestroyAssetDeleteCreatorsHolding(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetID := uint64(1)
	//// Create an asset. Create a transaction where all special addresses are different
	//// from creator's address.
	//txn0 := transactions.SignedTxnWithAD{
	//	SignedTxn: transactions.SignedTxn{
	//		Txn: transactions.Transaction{
	//			Type: "acfg",
	//			Header: transactions.Header{
	//				Sender:      test.AccountA,
	//				GenesisHash: test.GenesisHash,
	//			},
	//			AssetConfigTxnFields: transactions.AssetConfigTxnFields{
	//				AssetParams: basics.AssetParams{
	//					Manager:  test.AccountB,
	//					Reserve:  test.AccountB,
	//					Freeze:   test.AccountB,
	//					Clawback: test.AccountB,
	//				},
	//			},
	//		},
	//		Sig: test.Signature,
	//	},
	//}
	//
	//// Another account opts in.
	//txn1 := test.MakeAssetOptInTxn(assetID, test.AccountC)
	//
	//// Destroy an asset.
	//txn2 := test.MakeAssetDestroyTxn(assetID, test.AccountB)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/DestroyAssetDeleteCreatorsHolding.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// Check that the creator's asset holding is deleted.
	assertAssetHoldingDates(t, db.db, test.AccountA, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: 1},
		sql.NullInt64{Valid: true, Int64: 1})

	// Check that other account's asset holding was not deleted.
	assertAssetHoldingDates(t, db.db, test.AccountC, assetID,
		sql.NullBool{Valid: true, Bool: false},
		sql.NullInt64{Valid: true, Int64: 1},
		sql.NullInt64{Valid: false, Int64: 0})

	// Check that the manager does not have an asset holding.
	count := queryInt(
		db.db, "SELECT COUNT(*) FROM account_asset WHERE addr = $1", test.AccountB[:])
	assert.Equal(t, 0, count)
}

// Test that block import adds the freeze/sender accounts to txn_participation.
func TestAssetFreezeTxnParticipation(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // A block containing an asset freeze txn
	///////////

	// Create a block with freeze txn
	//createAsset := test.MakeAssetConfigTxn(
	//	0, uint64(1000000), uint64(6), false, "mcn", "my coin", "http://antarctica.com",
	//	test.AccountA)
	//optinB := test.MakeAssetOptInTxn(assetid, test.AccountB)
	//freeze := test.MakeAssetFreezeTxn(assetid, true, test.AccountA, test.AccountB)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AssetFreezeTxnParticipation.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// Then // Both accounts should have an entry in the txn_participation table.
	//////////
	round := uint64(1)
	intra := uint64(2)

	query :=
		"SELECT COUNT(*) FROM txn_participation WHERE addr = $1 AND round = $2 AND " +
			"intra = $3"
	acctACount := queryInt(db.db, query, test.AccountA[:], round, intra)
	acctBCount := queryInt(db.db, query, test.AccountB[:], round, intra)
	assert.Equal(t, 1, acctACount)
	assert.Equal(t, 1, acctBCount)
}

// Test that block import adds accounts from inner txns to txn_participation.
func TestInnerTxnParticipation(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // A block containing an app call txn with inners
	///////////

	// In order to simplify the test,
	// since db.AddBlock uses ApplyData from the block and not from the evaluator,
	// fake ApplyData to have inner txn
	// otherwise it requires funding the app account and other special setup
	var appAddr sdk.Address
	appAddr[1] = 99
	// createApp := test.MakeAppCallWithInnerTxn(test.AccountA, appAddr, test.AccountB, appAddr, test.AccountC)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/InnerTxnParticipation.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	//////////
	// Then // All accounts should have an entry in the txn_participation table.
	//////////
	round := uint64(1)
	intra := uint64(0) // the only one txn in the block

	query :=
		"SELECT COUNT(*) FROM txn_participation WHERE addr = $1 AND round = $2 AND " +
			"intra = $3"
	acctACount := queryInt(db.db, query, test.AccountA[:], round, intra)
	acctBCount := queryInt(db.db, query, test.AccountB[:], round, intra)
	acctCCount := queryInt(db.db, query, test.AccountC[:], round, intra)
	acctAppCount := queryInt(db.db, query, appAddr[:], round, intra)
	assert.Equal(t, 1, acctACount)
	assert.Equal(t, 1, acctBCount)
	assert.Equal(t, 1, acctCCount)
	assert.Equal(t, 1, acctAppCount)
}

func TestAppExtraPages(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Create an app.

	// Create a transaction with ExtraProgramPages field set to 1
	// txn := transactions.SignedTxnWithAD{
	//	SignedTxn: transactions.SignedTxn{
	//		Txn: transactions.Transaction{
	//			Type: "appl",
	//			Header: transactions.Header{
	//				Sender:      test.AccountA,
	//				GenesisHash: test.GenesisHash,
	//			},
	//			ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
	//				ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
	//				ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
	//				ExtraProgramPages: extraPages,
	//			},
	//		},
	//		Sig: test.Signature,
	//	},
	//}
	const extraPages = 1
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AppExtraPages.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	row := db.db.QueryRow(context.Background(), "SELECT index, params FROM app WHERE creator = $1", test.AccountA[:])

	var index uint64
	var paramsStr []byte
	err = row.Scan(&index, &paramsStr)
	require.NoError(t, err)
	require.NotZero(t, index)

	ap, err := encoding.DecodeAppParams(paramsStr)
	require.NoError(t, err)
	require.Equal(t, uint32(1), ap.ExtraProgramPages)

	var filter idb.ApplicationQuery
	var aidx = index
	filter.ApplicationID = uint64Ptr(aidx)
	appRows, _ := db.Applications(context.Background(), filter)
	num := 0
	for row := range appRows {
		require.NoError(t, row.Error)
		num++
		require.NotNil(t, row.Application.Params.ExtraProgramPages, "we should have this field")
		require.Equal(t, uint64(1), *row.Application.Params.ExtraProgramPages)
	}
	require.Equal(t, 1, num)

	rows, _ := db.GetAccounts(context.Background(), idb.AccountQueryOptions{EqualToAddress: test.AccountA[:], IncludeAppParams: true})
	num = 0
	var createdApps *[]generated.Application
	for row := range rows {
		require.NoError(t, row.Error)
		num++
		require.NotNil(t, row.Account.AppsTotalExtraPages, "we should have this field")
		require.Equal(t, uint64(1), *row.Account.AppsTotalExtraPages)
		require.Equal(t, uint64(1), row.Account.TotalCreatedApps)
		createdApps = row.Account.CreatedApps
	}
	require.Equal(t, 1, num)

	require.NotNil(t, createdApps)
	require.Equal(t, 1, len(*createdApps))
	app := (*createdApps)[0]
	require.NotNil(t, app.Params.ExtraProgramPages)
	require.Equal(t, uint64(extraPages), *app.Params.ExtraProgramPages)
}

func assertKeytype(t *testing.T, db *IndexerDb, address sdk.Address, keytype *generated.AccountSigType) {
	opts := idb.AccountQueryOptions{
		EqualToAddress: address[:],
		IncludeDeleted: true,
	}
	rowsCh, _ := db.GetAccounts(context.Background(), opts)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	assert.Equal(t, keytype, row.Account.SigType)
}

func TestKeytypeBasic(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assertKeytype(t, db, test.AccountA, nil)

	// Sig
	// txn := test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeytypeBasicSig.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	keytype := generated.AccountSigTypeSig
	assertKeytype(t, db, test.AccountA, &keytype)

	// Msig
	// txn = test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	vb2, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeytypeBasicMsig.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb2)
	require.NoError(t, err)

	keytype = "msig"
	assertKeytype(t, db, test.AccountA, &keytype)
}

// Test that asset amount >= 2^63 is handled correctly. Due to the specifics of
// postgres it might be a problem.
func TestLargeAssetAmount(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// txn := test.MakeAssetConfigTxn(
	//	0, math.MaxUint64, 0, false, "mc", "mycoin", "", test.AccountA)
	assetid := uint64Ptr(uint64(1))
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LargeAssetAmount.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	{
		opts := idb.AssetBalanceQuery{
			AssetID: assetid,
		}
		rowsCh, _ := db.AssetBalances(context.Background(), opts)

		row, ok := <-rowsCh
		require.True(t, ok)
		require.NoError(t, row.Error)
		assert.Equal(t, uint64(math.MaxUint64), row.Amount)
	}

	{
		opts := idb.AccountQueryOptions{
			EqualToAddress:       test.AccountA[:],
			IncludeAssetHoldings: true,
		}
		rowsCh, _ := db.GetAccounts(context.Background(), opts)

		row, ok := <-rowsCh
		require.True(t, ok)
		require.NoError(t, row.Error)
		require.NotNil(t, row.Account.Assets)
		require.Equal(t, 1, len(*row.Account.Assets))
		require.Equal(t, uint64(1), row.Account.TotalAssetsOptedIn)
		assert.Equal(t, uint64(math.MaxUint64), (*row.Account.Assets)[0].Amount)
	}
}

// Test that initializing a new database sets the correct migration number and
// that the database is available.
func TestInitializationNewDatabase(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	db, availableCh, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)
	defer db.Close()

	_, ok := <-availableCh
	assert.False(t, ok)

	state, err := db.getMigrationState(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, len(migrations), state.NextMigration)
}

// Test that opening the database the second time (after initializing) is successful.
func TestOpenDbAgain(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	for i := 0; i < 2; i++ {
		db, availableCh, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
		require.NoError(t, err)
		<-availableCh
		db.Close()
	}
}

func requireNilOrEqual(t *testing.T, expected string, actual *string) {
	if expected == "" {
		require.Nil(t, actual)
	} else {
		require.NotNil(t, actual)
		require.Equal(t, expected, *actual)
	}
}

// TestNonDisplayableUTF8 make sure we're able to import cheeky assets.
func TestNonDisplayableUTF8(t *testing.T) {
	tests := []struct {
		Name                   string
		AssetName              string
		AssetUnit              string
		AssetURL               string
		ExpectedAssetName      string
		ExpectedAssetUnit      string
		ExpectedAssetURL       string
		ValidatedBlockFilePath string
	}{
		{
			Name:                   "Normal",
			AssetName:              "asset-name",
			AssetUnit:              "au",
			AssetURL:               "https://algorand.com",
			ExpectedAssetName:      "asset-name",
			ExpectedAssetUnit:      "au",
			ExpectedAssetURL:       "https://algorand.com",
			ValidatedBlockFilePath: "test_resources/validated_blocks/NonDisplayableUTF8Normal.vb",
		},
		{
			Name:                   "Embedded Null",
			AssetName:              "asset\000name",
			AssetUnit:              "a\000u",
			AssetURL:               "https:\000//algorand.com",
			ExpectedAssetName:      "",
			ExpectedAssetUnit:      "",
			ExpectedAssetURL:       "",
			ValidatedBlockFilePath: "test_resources/validated_blocks/NonDisplayableUTF8EmbeddedNull.vb",
		},
		{
			Name:                   "Invalid UTF8",
			AssetName:              "asset\x8cname",
			AssetUnit:              "a\x8cu",
			AssetURL:               "https:\x8c//algorand.com",
			ExpectedAssetName:      "",
			ExpectedAssetUnit:      "",
			ExpectedAssetURL:       "",
			ValidatedBlockFilePath: "test_resources/validated_blocks/NonDisplayableUTF8InvalidUTF8.vb",
		},
		{
			Name:                   "Emoji",
			AssetName:              "💩",
			AssetUnit:              "💰",
			AssetURL:               "🌐",
			ExpectedAssetName:      "💩",
			ExpectedAssetUnit:      "💰",
			ExpectedAssetURL:       "🌐",
			ValidatedBlockFilePath: "test_resources/validated_blocks/NonDisplayableUTF8Emoji.vb",
		},
	}

	assetID := uint64Ptr(uint64(1))

	for _, testcase := range tests {
		testcase := testcase
		name := testcase.AssetName
		unit := testcase.AssetUnit
		url := testcase.AssetURL

		t.Run(testcase.Name, func(t *testing.T) {
			db, shutdownFunc := setupIdb(t, test.MakeGenesis())
			defer shutdownFunc()

			// txn := test.MakeAssetConfigTxn(
			//	0, math.MaxUint64, 0, false, unit, name, url, test.AccountA)
			//// Try to add cheeky inner txns lazily by adding an AD to the acfg txn
			// txn.ApplyData.EvalDelta.InnerTxns = []transactions.SignedTxnWithAD{
			//	test.MakeAssetConfigTxn(
			//		0, math.MaxUint64, 0, false, unit, name, url, test.AccountA),
			//}
			// txn.ApplyData.EvalDelta.InnerTxns[0].ConfigAsset = basics.AssetIndex(innerAssetID)

			// Test 1: import/accounting should work.
			vb, err := test.ReadValidatedBlockFromFile(testcase.ValidatedBlockFilePath)
			require.NoError(t, err)
			err = db.AddBlock(&vb)
			require.NoError(t, err)

			// Test 2: asset results properly serialized
			assets, _ := db.Assets(context.Background(), idb.AssetsQuery{AssetID: assetID})
			num := 0
			for asset := range assets {
				require.NoError(t, asset.Error)
				require.Equal(t, name, asset.Params.AssetName)
				require.Equal(t, unit, asset.Params.UnitName)
				require.Equal(t, url, asset.Params.URL)
				num++
			}
			require.Equal(t, 1, num)

			// Test 3: transaction results properly serialized
			// Transaction results also return the inner txn acfg
			txnRows, _ := db.Transactions(context.Background(), idb.TransactionFilter{SkipOptimization: true})
			num = 0
			for row := range txnRows {
				require.NoError(t, row.Error)
				// The inner txns will have a RootTxn instead of a Txn row
				var rowTxn *sdk.SignedTxnWithAD
				if row.Txn != nil {
					rowTxn = row.Txn
				} else {
					rowTxn = row.RootTxn
				}

				// Note: These are created from the TxnBytes, so they have the exact name with embedded null.
				require.NotNil(t, rowTxn)
				require.Equal(t, name, rowTxn.Txn.AssetParams.AssetName)
				require.Equal(t, unit, rowTxn.Txn.AssetParams.UnitName)
				require.Equal(t, url, rowTxn.Txn.AssetParams.URL)
				num++
			}
			// Check that the root and inner asset is matched
			require.Equal(t, 2, num)

			// Test 4: account results should have the correct asset
			accounts, _ := db.GetAccounts(context.Background(), idb.AccountQueryOptions{EqualToAddress: test.AccountA[:], IncludeAssetParams: true})
			num = 0
			for acct := range accounts {
				require.NoError(t, acct.Error)
				require.NotNil(t, acct.Account.CreatedAssets)
				require.Len(t, *acct.Account.CreatedAssets, 1)
				require.Equal(t, uint64(1), acct.Account.TotalCreatedAssets)

				asset := (*acct.Account.CreatedAssets)[0]
				if testcase.ExpectedAssetName == "" {
					require.Nil(t, asset.Params.Name)
				}
				requireNilOrEqual(t, testcase.ExpectedAssetName, asset.Params.Name)
				requireNilOrEqual(t, testcase.ExpectedAssetUnit, asset.Params.UnitName)
				requireNilOrEqual(t, testcase.ExpectedAssetURL, asset.Params.Url)
				require.Equal(t, []byte(name), *asset.Params.NameB64)
				require.Equal(t, []byte(unit), *asset.Params.UnitNameB64)
				require.Equal(t, []byte(url), *asset.Params.UrlB64)
				num++
			}
			require.Equal(t, 1, num)
		})
	}
}

// TestReconfigAsset make sure we properly handle asset param merges.
func TestReconfigAsset(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	unit := "co\000in"
	name := "algo"
	url := "https://algorand.com"
	assetID := uint64Ptr(uint64(1))

	// txn := test.MakeAssetConfigTxn(
	//	0, math.MaxUint64, 0, false, unit, name, url, test.AccountA)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/ReconfigAsset.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// Test 2: asset results properly serialized
	assets, _ := db.Assets(context.Background(), idb.AssetsQuery{AssetID: assetID})
	num := 0
	for asset := range assets {
		require.NoError(t, asset.Error)
		require.Equal(t, name, asset.Params.AssetName)
		require.Equal(t, unit, asset.Params.UnitName)
		require.Equal(t, url, asset.Params.URL)

		require.Equal(t, sdk.Address{}, asset.Params.Manager, "Manager should have been cleared.")
		require.Equal(t, sdk.Address{}, asset.Params.Reserve, "Reserve should have been cleared.")
		// These were updated
		require.Equal(t, sdk.Address(test.AccountB), asset.Params.Freeze)
		require.Equal(t, sdk.Address(test.AccountC), asset.Params.Clawback)
		num++
	}
	require.Equal(t, 1, num)
}

func TestKeytypeResetsOnRekey(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Sig
	// txn := test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeytypeResetsOnRekeySig.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	keytype := generated.AccountSigTypeSig
	assertKeytype(t, db, test.AccountA, &keytype)

	// Rekey.
	// txn = test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, test.AccountB)
	vb2, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeytypeResetsOnRekeyRekey.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb2)
	require.NoError(t, err)

	assertKeytype(t, db, test.AccountA, nil)

	// Msig
	// txn = test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	vb3, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeytypeResetsOnRekeyMsig.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb3)
	require.NoError(t, err)

	keytype = generated.AccountSigTypeMsig
	assertKeytype(t, db, test.AccountA, &keytype)
}

// Test that after closing the account, keytype will be correctly set.
func TestKeytypeDeletedAccount(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assertKeytype(t, db, test.AccountA, nil)

	// closeTxn := test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, test.AccountB, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeytypeDeletedAccount.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	keytype := generated.AccountSigTypeSig
	assertKeytype(t, db, test.AccountA, &keytype)
}

// TestAddBlockGenesis tests that adding block 0 is successful.
func TestAddBlockGenesis(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	opts := idb.GetBlockOptions{
		Transactions: true,
	}
	blockHeaderRet, txns, err := db.GetBlock(context.Background(), 0, opts)
	require.NoError(t, err)
	assert.Empty(t, txns)
	genesisBlock := test.MakeGenesisBlock()
	require.NoError(t, err)
	assert.Equal(t, genesisBlock.BlockHeader, blockHeaderRet)

	nextRound, err := db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), nextRound)
}

// TestAddBlockAssetCloseAmountInTxnExtra tests that we set the correct asset close
// amount in `txn.extra` column.
func TestAddBlockAssetCloseAmountInTxnExtra(t *testing.T) {
	// Use an old version of consensus parameters that have AssetCloseAmount = false.
	genesis := test.MakeGenesis()
	genesis.Proto = string(protocol.ConsensusV24)

	db, shutdownFunc := setupIdb(t, genesis)
	defer shutdownFunc()

	// createAsset := test.MakeAssetConfigTxn(
	//	0, uint64(1000000), uint64(6), false, "mcn", "my coin", "http://antarctica.com",
	//	test.AccountA)
	// optinB := test.MakeAssetOptInTxn(assetid, test.AccountB)
	// transferAB := test.MakeAssetTransferTxn(
	//	assetid, 100, test.AccountA, test.AccountB, basics.Address{})
	// optinC := test.MakeAssetOptInTxn(assetid, test.AccountC)
	//// Close B to C.
	// closeB := test.MakeAssetTransferTxn(
	//	assetid, 30, test.AccountB, test.AccountA, test.AccountC)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AddBlockAssetCloseAmountInTxnExtra.vb")
	require.NoError(t, err)

	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// Check asset close amount in the `closeB` transaction.
	round := uint64(1)
	intra := uint64(4)

	tf := idb.TransactionFilter{
		Round:  &round,
		Offset: &intra,
	}
	rowsCh, _ := db.Transactions(context.Background(), tf)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	assert.Equal(t, uint64(70), row.Extra.AssetCloseAmount)

	row, ok = <-rowsCh
	require.False(t, ok)
}

func TestAddBlockIncrementsMaxRoundAccounted(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	db, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)
	defer db.Close()

	err = db.LoadGenesis(test.MakeGenesis())
	require.NoError(t, err)

	round, err := db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)

	// add genesis block
	block := test.MakeGenesisBlock()
	vb := types.ValidatedBlock{
		Block: block,
		Delta: sdk.LedgerStateDelta{},
	}
	require.NoError(t, db.AddBlock(&vb))

	round, err = db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), round)

	block, err = test.MakeBlockForTxns(block.BlockHeader)
	require.NoError(t, err)
	block.BlockHeader.Round = 1
	vb = types.ValidatedBlock{
		Block: block,
		Delta: sdk.LedgerStateDelta{},
	}
	require.NoError(t, db.AddBlock(&vb))
	require.NoError(t, err)

	round, err = db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(2), round)

	block, err = test.MakeBlockForTxns(block.BlockHeader)
	require.NoError(t, err)
	block.BlockHeader.Round = 2
	vb = types.ValidatedBlock{
		Block: block,
		Delta: sdk.LedgerStateDelta{},
	}
	require.NoError(t, db.AddBlock(&vb))

	round, err = db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(3), round)
}

// Test that AddBlock makes a record of an account that gets created and deleted in
// the same round.
func TestAddBlockCreateDeleteAccountSameRound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// createTxn := test.MakePaymentTxn(
	//	0, 5, 0, 0, 0, 0, test.AccountA, test.AccountE, basics.Address{}, basics.Address{})
	// deleteTxn := test.MakePaymentTxn(
	//	0, 2, 3, 0, 0, 0, test.AccountE, test.AccountB, test.AccountC, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AddBlockCreateDeleteAccountSameRound.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	opts := idb.AccountQueryOptions{
		EqualToAddress: test.AccountE[:],
		IncludeDeleted: true,
	}
	rowsCh, _ := db.GetAccounts(context.Background(), opts)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	require.NotNil(t, row.Account.Deleted)
	assert.True(t, *row.Account.Deleted)
	require.NotNil(t, row.Account.CreatedAtRound)
	assert.Equal(t, uint64(1), *row.Account.CreatedAtRound)
	require.NotNil(t, row.Account.ClosedAtRound)
	assert.Equal(t, uint64(1), *row.Account.ClosedAtRound)
}

// Test that AddBlock makes a record of an asset that is created and deleted in
// the same round.
func TestAddBlockCreateDeleteAssetSameRound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64Ptr(uint64(1))
	// createTxn := test.MakeAssetConfigTxn(0, 3, 0, false, "", "", "", test.AccountA)
	// deleteTxn := test.MakeAssetDestroyTxn(assetid, test.AccountA)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AddBlockCreateDeleteAssetSameRound.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// Asset global state.
	{
		opts := idb.AssetsQuery{
			AssetID:        assetid,
			IncludeDeleted: true,
		}
		rowsCh, _ := db.Assets(context.Background(), opts)

		row, ok := <-rowsCh
		require.True(t, ok)
		require.NoError(t, row.Error)
		require.NotNil(t, row.Deleted)
		assert.True(t, *row.Deleted)
		require.NotNil(t, row.CreatedRound)
		assert.Equal(t, uint64(1), *row.CreatedRound)
		require.NotNil(t, row.ClosedRound)
		assert.Equal(t, uint64(1), *row.ClosedRound)
	}

	// Asset local state.
	{
		opts := idb.AssetBalanceQuery{
			AssetID:        assetid,
			IncludeDeleted: true,
		}
		rowsCh, _ := db.AssetBalances(context.Background(), opts)

		row, ok := <-rowsCh
		require.True(t, ok)
		require.NoError(t, row.Error)
		require.NotNil(t, row.Deleted)
		assert.True(t, *row.Deleted)
		require.NotNil(t, row.CreatedRound)
		assert.Equal(t, uint64(1), *row.CreatedRound)
		require.NotNil(t, row.ClosedRound)
		assert.Equal(t, uint64(1), *row.ClosedRound)
	}
}

// Test that AddBlock makes a record of an app that is created and deleted in
// the same round.
func TestAddBlockCreateDeleteAppSameRound(t *testing.T) {

	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	appid := uint64Ptr(uint64(1))

	// createTxn := test.MakeCreateAppTxn(test.AccountA)
	// deleteTxn := test.MakeAppDestroyTxn(appid, test.AccountA)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AddBlockCreateDeleteAppSameRound.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	opts := idb.ApplicationQuery{
		ApplicationID:  appid,
		IncludeDeleted: true,
	}
	rowsCh, _ := db.Applications(context.Background(), opts)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	require.NotNil(t, row.Application.Deleted)
	assert.True(t, *row.Application.Deleted)
	require.NotNil(t, row.Application.CreatedAtRound)
	assert.Equal(t, uint64(1), *row.Application.CreatedAtRound)
	require.NotNil(t, row.Application.DeletedAtRound)
	assert.Equal(t, uint64(1), *row.Application.DeletedAtRound)
}

// Test that AddBlock makes a record of an app that is created and deleted in
// the same round.
func TestAddBlockAppOptInOutSameRound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// createTxn := test.MakeCreateAppTxn(test.AccountA)
	// optInTxn := test.MakeAppOptInTxn(appid, test.AccountB)
	// optOutTxn := test.MakeAppOptOutTxn(appid, test.AccountB)
	appid := uint64Ptr(uint64(1))
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AddBlockAppOptInOutSameRound.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	opts := idb.AccountQueryOptions{
		EqualToAddress:       test.AccountB[:],
		IncludeDeleted:       true,
		IncludeAppLocalState: true,
	}
	rowsCh, _ := db.GetAccounts(context.Background(), opts)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)

	require.NotNil(t, row.Account.AppsLocalState)
	require.Equal(t, 1, len(*row.Account.AppsLocalState))

	localState := (*row.Account.AppsLocalState)[0]
	require.NotNil(t, localState.Deleted)
	assert.True(t, *localState.Deleted)
	require.NotNil(t, localState.OptedInAtRound)
	assert.Equal(t, uint64(1), *localState.OptedInAtRound)
	require.NotNil(t, localState.ClosedOutAtRound)
	assert.Equal(t, uint64(1), *localState.ClosedOutAtRound)
	require.Equal(t, uint64(0), row.Account.TotalAppsOptedIn)

	q := idb.ApplicationQuery{
		ApplicationID:  appid,
		IncludeDeleted: true,
	}
	lsRows, _ := db.AppLocalState(context.Background(), q)
	lsRow, ok := <-lsRows
	require.True(t, ok)
	require.NoError(t, lsRow.Error)
	ls := lsRow.AppLocalState
	require.Equal(t, appid, uint64Ptr(ls.Id))
	require.NotNil(t, ls.Deleted)
	assert.True(t, *ls.Deleted)
	require.NotNil(t, ls.OptedInAtRound)
	assert.Equal(t, uint64(1), *ls.OptedInAtRound)
	require.NotNil(t, ls.ClosedOutAtRound)
	assert.Equal(t, uint64(1), *ls.ClosedOutAtRound)
}

// TestSearchForInnerTransactionReturnsRootTransaction checks that the parent
// transaction is returned when matching on inner transactions if the
// ReturnInnerTxnFlag is false. If the ReturnInnerTxnFlag is true, it should
// return the inner txn instead.
func TestSearchForInnerTransactionReturnsRootTransaction(t *testing.T) {
	var appAddr sdk.Address
	appAddr[1] = 99

	tests := []struct {
		name        string
		matches     int
		returnInner bool
		filter      idb.TransactionFilter
	}{
		{
			name:        "match on root, inner, and inner-inners, return root",
			matches:     3,
			returnInner: false,
			filter:      idb.TransactionFilter{Address: appAddr[:], TypeEnum: idb.TypeEnumApplication},
		},
		{
			name:        "match on inner, return root",
			matches:     1,
			returnInner: false,
			filter:      idb.TransactionFilter{Address: appAddr[:], TypeEnum: idb.TypeEnumPay},
		},
		{
			name:        "match on inner-inner, return root",
			matches:     1,
			returnInner: false,
			filter:      idb.TransactionFilter{Address: appAddr[:], TypeEnum: idb.TypeEnumAssetTransfer},
		},
		{
			name:        "match all, return root",
			matches:     5,
			returnInner: false,
			filter:      idb.TransactionFilter{Address: appAddr[:]},
		},
		{
			name:        "match on root, inner, and inner-inners, return inners",
			matches:     3,
			returnInner: true,
			filter:      idb.TransactionFilter{Address: appAddr[:], TypeEnum: idb.TypeEnumApplication, SkipInnerTransactionConversion: true},
		},
		{
			name:        "match on inner, return inners",
			matches:     1,
			returnInner: true,
			filter:      idb.TransactionFilter{Address: appAddr[:], TypeEnum: idb.TypeEnumPay, SkipInnerTransactionConversion: true},
		},
		{
			name:        "match on inner-inner, return inners",
			matches:     1,
			returnInner: true,
			filter:      idb.TransactionFilter{Address: appAddr[:], TypeEnum: idb.TypeEnumAssetTransfer, SkipInnerTransactionConversion: true},
		},
		{
			name:        "match all, return inners",
			matches:     5,
			returnInner: true,
			filter:      idb.TransactionFilter{Address: appAddr[:], SkipInnerTransactionConversion: true},
		},
	}

	// Given: A DB with one transaction containing inner transactions [app -> pay -> xfer]
	pdb, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	db := setupIdbWithConnectionString(t, connStr, test.MakeGenesis())
	defer db.Close()

	// appCall := test.MakeAppCallWithInnerTxn(test.AccountA, appAddr, test.AccountB, appAddr, test.AccountC)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/SearchForInnerTransactionReturnsRootTransaction.vb")
	require.NoError(t, err)

	stxn, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])
	require.NoError(t, err)
	rootTxid := crypto2.TransactionIDString(stxn.Txn)

	err = pgutil.TxWithRetry(pdb, serializable, func(tx pgx.Tx) error {
		genblk := types.ValidatedBlock{
			Block: test.MakeGenesisBlock(),
			Delta: sdk.LedgerStateDelta{},
		}
		require.NoError(t, db.AddBlock(&genblk))
		return db.AddBlock(&vb)
	}, nil)
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// When: searching for a transaction that matches part of the transaction.
			results, _ := db.Transactions(context.Background(), tc.filter)

			// Then: only the root transaction should be returned if the SkipInnerTransactionConversion flag is true.
			// Else if SkipInnerTransactionConversion is false, then the inner txn should be returned.
			num := 0
			for result := range results {
				num++
				require.NoError(t, result.Error)

				if tc.returnInner {
					// Make sure that only the inner txn is returned
					require.True(t, (result.Txn != nil) && (result.RootTxn == nil))
				} else {
					// Make sure the root txn is returned.
					var stxn *sdk.SignedTxnWithAD

					// Exactly one of Txn and RootTxn must be present.
					require.True(t, (result.Txn == nil) != (result.RootTxn == nil))

					// Get Txn or RootTxn
					if result.Txn != nil {
						stxn = result.Txn
					}
					if result.RootTxn != nil {
						stxn = result.RootTxn
					}
					require.Equal(t, rootTxid, crypto2.TransactionIDString(stxn.Txn))
				}
			}

			// There can be multiple matches because deduplication happens in REST API.
			require.Equal(t, tc.matches, num)
		})
	}
}

// TestNonUTF8Logs makes sure we're able to import cheeky logs
// for both the root and inner transactions.
func TestNonUTF8Logs(t *testing.T) {
	tests := []struct {
		Name                   string
		Logs                   []string
		ValidatedBlockFilePath string
	}{
		{
			Name:                   "Normal",
			Logs:                   []string{"Test log1", "Test log2", "Test log3"},
			ValidatedBlockFilePath: "./test_resources/validated_blocks/NonUTF8LogsNormal.vb",
		},
		{
			Name:                   "Embedded Null",
			Logs:                   []string{"\000", "\x00\x00\x00\x00\x00\x00\x00\x00", string([]byte{00, 00})},
			ValidatedBlockFilePath: "./test_resources/validated_blocks/NonUTF8LogsEmbeddedNull.vb",
		},
		{
			Name:                   "Invalid UTF8",
			Logs:                   []string{"\x8c", "\xff", "\xf8"},
			ValidatedBlockFilePath: "./test_resources/validated_blocks/NonUTF8LogsInvalidUTF8.vb",
		},
		{
			Name:                   "Emoji",
			Logs:                   []string{"💩", "💰", "🌐"},
			ValidatedBlockFilePath: "./test_resources/validated_blocks/NonUTF8LogsEmoji.vb",
		},
	}

	for _, testcase := range tests {
		testcase := testcase

		t.Run(testcase.Name, func(t *testing.T) {
			db, shutdownFunc := setupIdb(t, test.MakeGenesis())
			defer shutdownFunc()

			// createAppTxn := test.MakeCreateAppTxn(test.AccountA)
			// createAppTxn.ApplyData.EvalDelta = transactions.EvalDelta{
			//	Logs: testcase.Logs,
			//	InnerTxns: []transactions.SignedTxnWithAD{
			//		// Inner application call with nested cheeky logs
			//		{
			//			SignedTxn: transactions.SignedTxn{
			//				Txn: transactions.Transaction{
			//					Type: protocol2.ApplicationCallTx,
			//					Header: transactions.Header{
			//						Sender: test.AccountA,
			//					},
			//					ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
			//						ApplicationID: 789,
			//						OnCompletion:  transactions.NoOpOC,
			//					},
			//				},
			//			},
			//			ApplyData: transactions.ApplyData{
			//				EvalDelta: transactions.EvalDelta{
			//					Logs: testcase.Logs,
			//				},
			//			},
			//		},
			//	},
			//}

			vb, err := test.ReadValidatedBlockFromFile(testcase.ValidatedBlockFilePath)
			require.NoError(t, err)

			// Test 1: import/accounting should work.
			err = db.AddBlock(&vb)
			require.NoError(t, err)

			// Test 2: transaction results properly serialized
			txnRows, _ := db.Transactions(context.Background(), idb.TransactionFilter{})
			for row := range txnRows {
				var rowTxn *sdk.SignedTxnWithAD
				if row.Txn != nil {
					rowTxn = row.Txn
				} else {
					rowTxn = row.RootTxn
				}
				require.NoError(t, row.Error)
				require.NotNil(t, rowTxn)
				require.Equal(t, testcase.Logs, rowTxn.ApplyData.EvalDelta.Logs)
			}
		})
	}
}

func TestTxnAssetID(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// createAssetTxn := test.MakeAssetConfigTxn(
	//	0, 0, 0, false, "myasset", "ma", "", test.AccountA)
	// configAssetTxn := test.MakeAssetConfigTxn(
	//	assetid, 0, 0, false, "myasset", "ma", "", test.AccountA)
	// appid := uint64(3)
	// createAppTxn := test.MakeCreateAppTxn(test.AccountA)
	// destroyAppTxn := test.MakeAppDestroyTxn(appid, test.AccountA)

	assetid := uint64(1)
	appid := uint64(3)
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/TxnAssetID.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	txnRowsCh, _ := db.Transactions(context.Background(), idb.TransactionFilter{})
	for i := 0; i < 2; i++ {
		row, ok := <-txnRowsCh
		require.True(t, ok)
		require.NoError(t, row.Error)
		assert.Equal(t, assetid, row.AssetID)
	}
	for i := 0; i < 2; i++ {
		row, ok := <-txnRowsCh
		require.True(t, ok)
		require.NoError(t, row.Error)
		assert.Equal(t, appid, row.AssetID)
	}
}

func TestBadTxnJsonEncoding(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Need to import a block header because the transactions query joins on it.
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BadTxnJsonEncoding.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	rootTxid := "abc"
	rootIntra := uint(4)
	badJSON := `{"aaaaaaaa": 0}`

	query := `INSERT INTO txn (round, intra, typeenum, asset, txid, txn, extra)
    VALUES (1, $1, 0, 0, $2, $3, $4)`

	_, err = db.db.Exec(
		context.Background(), query, rootIntra, rootTxid, badJSON,
		encoding.EncodeTxnExtra(&idb.TxnExtra{}))
	require.NoError(t, err)

	{
		extra := idb.TxnExtra{
			RootIntra: idb.OptionalUint{Present: true, Value: rootIntra},
			RootTxid:  rootTxid,
		}
		_, err = db.db.Exec(
			context.Background(), query, rootIntra+1, nil, badJSON,
			encoding.EncodeTxnExtra(&extra))
		require.NoError(t, err)
	}

	{
		offset := uint64(rootIntra)
		tf := idb.TransactionFilter{
			SkipOptimization: true,
			Offset:           &offset,
		}
		rowsCh, _ := db.Transactions(context.Background(), tf)

		row, ok := <-rowsCh
		require.True(t, ok)

		require.Error(t, row.Error)
		assert.Contains(t, row.Error.Error(), "error decoding txn")
	}

	{
		offset := uint64(rootIntra) + 1
		tf := idb.TransactionFilter{
			SkipOptimization: true,
			Offset:           &offset,
		}
		rowsCh, _ := db.Transactions(context.Background(), tf)

		row, ok := <-rowsCh
		require.True(t, ok)

		require.Error(t, row.Error)
		assert.Contains(t, row.Error.Error(), "error decoding roottxn")
	}
}

func TestKeytypeDoNotResetReceiver(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assertKeytype(t, db, test.AccountA, nil)

	// Sigtype of account B becomes "sig".
	// txn := test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountB, test.AccountB, basics.Address{}, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeytypeDoNotResetReceiver1.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// Sigtype of account A becomes "sig" and B remains the same.
	vb2, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/KeytypeDoNotResetReceiver2.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb2)
	require.NoError(t, err)

	keytype := generated.AccountSigTypeSig
	assertKeytype(t, db, test.AccountA, &keytype)
	assertKeytype(t, db, test.AccountB, &keytype)
}

// Test that if information in `txn` and `txn_participation` tables is ahead of
// the current round, AddBlock() still runs successfully.
func TestAddBlockTxnTxnParticipationAhead(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	{
		query := `INSERT INTO txn (round, intra, typeenum, asset, txn, extra)
			VALUES (1, 0, 0, 0, 'null'::jsonb, 'null'::jsonb)`
		_, err := db.db.Exec(context.Background(), query)
		require.NoError(t, err)
	}
	{
		query := `INSERT INTO txn_participation (addr, round, intra)
			VALUES ($1, 1, 0)`
		_, err := db.db.Exec(context.Background(), query, test.AccountA[:])
		require.NoError(t, err)
	}

	// txn := test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AddBlockTxnTxnParticipationAhead.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)
}

// Test that AddBlock() writes to `txn_participation` table.
func TestAddBlockTxnParticipationAdded(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// txn := test.MakePaymentTxn(
	//	0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/AddBlockTxnParticipationAdded.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	tf := idb.TransactionFilter{
		Address: test.AccountA[:],
	}
	rowsCh, _ := db.Transactions(context.Background(), tf)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	require.NotNil(t, row.Txn)

	stxn, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])
	require.NoError(t, err)
	expected := base64.StdEncoding.EncodeToString(msgpack.Encode(stxn))
	actual := base64.StdEncoding.EncodeToString(msgpack.Encode(*row.Txn))
	assert.Equal(t, expected, actual)
}

// Test that if information in the `txn` table is ahead of the current round,
// Transactions() doesn't return the rows ahead of the state.
func TestTransactionsTxnAhead(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Insert a transaction row at round 1 and check that Transactions() does not return
	// it.
	{
		query := `INSERT INTO txn (round, intra, typeenum, asset, txn, extra)
			VALUES (1, 0, 0, 0, 'null'::jsonb, 'null'::jsonb)`
		_, err := db.db.Exec(context.Background(), query)
		require.NoError(t, err)
	}
	{
		rowsCh, _ := db.Transactions(context.Background(), idb.TransactionFilter{SkipOptimization: true})
		_, ok := <-rowsCh
		assert.False(t, ok)
	}

	// Now add an empty round 1 block, and verify that Transactions() returns the
	// fake transaction.
	{
		vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/TransactionsTxnAhead.vb")
		require.NoError(t, err)
		err = db.AddBlock(&vb)
		require.NoError(t, err)
	}
	{
		rowsCh, _ := db.Transactions(context.Background(), idb.TransactionFilter{SkipOptimization: true})
		row, ok := <-rowsCh
		require.True(t, ok)
		require.NoError(t, row.Error)
	}
}

// Test that if genesis hash is different from what is in db metastate
// indexer does not start.
func TestGenesisHashCheckAtDBSetup(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	genesis := test.MakeGenesis()
	db := setupIdbWithConnectionString(t, connStr, genesis)
	defer db.Close()
	genesisHash := genesis.Hash()
	network, err := db.getMetastate(context.Background(), nil, schema.NetworkMetaStateKey)
	assert.NoError(t, err)
	networkState, err := encoding.DecodeNetworkState([]byte(network))
	assert.NoError(t, err)
	assert.Equal(t, genesisHash, networkState.GenesisHash)
	// connect with different genesis configs
	genesis.Network = "testnest"
	// different genesisHash, should fail
	idbImpl, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	err = idbImpl.LoadGenesis(genesis)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "genesis hash not matching")
}

type ImportState struct {
	NextRoundToAccount uint64 `codec:"next_account_round"`
}

// Test that if genesis hash at initial import is different from what is in db metastate
// indexer does not start.
func TestGenesisHashCheckAtInitialImport(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	genesis := test.MakeGenesis()
	db, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)
	defer db.Close()
	// test db upgrade
	// set next round to account
	state := ImportState{NextRoundToAccount: 1}
	var buf []byte
	jsonCodecHandle := new(codec.JsonHandle)
	enc := codec.NewEncoderBytes(&buf, jsonCodecHandle)
	enc.MustEncode(state)
	err = db.setMetastate(nil, schema.StateMetastateKey, string(buf))
	assert.NoError(t, err)
	// network state not initialized
	networkState, err := db.getNetworkState(context.Background(), nil)
	require.ErrorIs(t, err, idb.ErrorNotInitialized)
	genesisReader := bytes.NewReader(json.Encode(genesis))
	gen, err := util.ReadGenesis(genesisReader)
	require.NoError(t, err)
	imported, err := util.EnsureInitialImport(db, gen)
	require.NoError(t, err)
	require.True(t, true, imported)
	// network state should be set
	networkState, err = db.getNetworkState(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, networkState.GenesisHash, genesis.Hash())

	// change genesis value
	genesis.Network = "testnest"
	genesisReader = bytes.NewReader(json.Encode(genesis))
	gen, err = util.ReadGenesis(genesisReader)
	require.NoError(t, err)
	// different genesisHash, should fail
	_, err = util.EnsureInitialImport(db, gen)
	require.Error(t, err)
	require.Contains(t, err.Error(), "genesis hash not matching")

}

func getResults(ctx context.Context, rows <-chan idb.AccountRow) (result []idb.AccountRow) {
	for {
		select {
		case row, ok := <-rows:
			if !ok {
				return
			}
			result = append(result, row)
		case <-ctx.Done():
			return
		}
	}
}

func TestIndexerDb_GetAccounts(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	testcases := []struct {
		gt *uint64
		lt *uint64
		id uint64
	}{
		{
			gt: uint64Ptr(1),
			lt: nil,
		},
		{
			gt: nil,
			lt: uint64Ptr(1),
		},
		{
			gt: uint64Ptr(1),
			lt: uint64Ptr(1),
		},
		{
			gt: uint64Ptr(1),
			lt: nil,
			id: 1,
		},
		{
			gt: nil,
			lt: uint64Ptr(1),
			id: 1,
		},
		{
			gt: uint64Ptr(1),
			lt: uint64Ptr(1),
			id: 1,
		},
	}

	for i, testcase := range testcases {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			resultCh, count := db.GetAccounts(ctx, idb.AccountQueryOptions{
				AssetGT:    testcase.gt,
				AssetLT:    testcase.lt,
				HasAssetID: testcase.id,
			})
			assert.Equal(t, uint64(0), count)
			results := getResults(ctx, resultCh)

			if testcase.id == 0 {
				// When the id is 0, there should be an error
				assert.NotNil(t, results)
				assert.Len(t, results, 1)
				assert.Error(t, results[0].Error)
				expected := fmt.Sprintf("AssetGT=%d, AssetLT=%d, but HasAssetID=0", uintOrDefault(testcase.gt), uintOrDefault(testcase.lt))
				assert.Equal(t, expected, results[0].Error.Error())
			} else {
				// Otherwise, the empty DB should simply return no results.
				assert.Nil(t, results)
			}
		})
	}
}

// Test transaction filter asset amount
func TestTransactionFilterAssetAmount(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// AssetAmountLT
	// txnA := test.MakeAssetConfigTxn(0, 100, 0, false, "test", "test", "", test.AccountA)
	// txnB := test.MakeAssetOptInTxn(1, test.AccountB)
	// txnC := test.MakeAssetTransferTxn(1, 10, test.AccountA, test.AccountB, basics.Address{})
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/TransactionFilterAssetAmount1.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// query
	filter := idb.TransactionFilter{AssetAmountLT: uint64Ptr(uint64(math.MaxInt64 + 1))}
	rowsCh, _ := db.Transactions(context.Background(), filter)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	require.NotNil(t, row.Txn)
	txnC, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[2])
	require.NoError(t, err)
	expected := base64.StdEncoding.EncodeToString(msgpack.Encode(txnC))
	actual := base64.StdEncoding.EncodeToString(msgpack.Encode(*row.Txn))
	assert.Equal(t, expected, actual)

	// AssetAmountGT
	// txnD := test.MakeAssetConfigTxn(0, math.MaxUint64, 0, false, "test2", "test2", "", test.AccountA)
	// txnE := test.MakeAssetOptInTxn(4, test.AccountB)
	// txnF := test.MakeAssetTransferTxn(4, math.MaxUint64, test.AccountA, test.AccountB, basics.Address{})
	vb2, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/TransactionFilterAssetAmount2.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb2)
	require.NoError(t, err)

	// query
	filter = idb.TransactionFilter{AssetAmountGT: uint64Ptr(uint64(math.MaxInt64 + 1))}
	rowsCh, _ = db.Transactions(context.Background(), filter)

	row, ok = <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	require.NotNil(t, row.Txn)

	txnF, _, err := util.DecodeSignedTxn(vb2.Block.BlockHeader, vb2.Block.Payset[2])
	require.NoError(t, err)
	expected = base64.StdEncoding.EncodeToString(msgpack.Encode(txnF))
	actual = base64.StdEncoding.EncodeToString(msgpack.Encode(*row.Txn))
	assert.Equal(t, expected, actual)
}

func TestDeleteTransactions(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	txns := []sdk.SignedTxn{}

	// add 4 rounds of txns
	// txnA := test.MakeCreateAppTxn(test.AccountA)
	// txnB := test.MakeCreateAppTxn(test.AccountB)
	// txnC := test.MakeCreateAppTxn(test.AccountC)
	// txnD := test.MakeCreateAppTxn(test.AccountD)
	for i := 1; i <= 4; i++ {
		vb, err := test.ReadValidatedBlockFromFile(fmt.Sprintf("test_resources/validated_blocks/DeleteTransactionsAddRound%d.vb", i))
		require.NoError(t, err)

		err = db.AddBlock(&vb)
		require.NoError(t, err)
		stxn, _, err := util.DecodeSignedTxn(vb.Block.BlockHeader, vb.Block.Payset[0])
		require.NoError(t, err)
		txns = append(txns, stxn)
	}

	// keep rounds >= 2
	err := db.DeleteTransactions(context.Background(), 2)
	assert.NoError(t, err)

	// query txns
	rowsCh, _ := db.Transactions(context.Background(), idb.TransactionFilter{})

	// check remaining transactions are correct
	i := 1
	for row := range rowsCh {
		require.NoError(t, row.Error)
		require.NotNil(t, row.Txn)
		expected := base64.StdEncoding.EncodeToString(msgpack.Encode(txns[i]))
		actual := base64.StdEncoding.EncodeToString(msgpack.Encode(*row.Txn))
		assert.Equal(t, expected, actual)
		i++
	}

	// verify metastate
	deleteStatus, err := db.getMetastate(context.Background(), nil, schema.DeleteStatusKey)
	assert.NoError(t, err)
	status, err := encoding.DecodeDeleteStatus([]byte(deleteStatus))
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), status.OldestRound)

	// add 2 txns for round 5
	vb, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/DeleteTransactions.vb")
	require.NoError(t, err)
	err = db.AddBlock(&vb)
	require.NoError(t, err)

	// keep round 5
	err = db.DeleteTransactions(context.Background(), 5)
	assert.NoError(t, err)

	// 2 txns in round 5
	rowsCh, _ = db.Transactions(context.Background(), idb.TransactionFilter{})
	i = 0
	for row := range rowsCh {
		require.NoError(t, row.Error)
		require.NotNil(t, row.Txn)
		expected := base64.StdEncoding.EncodeToString(msgpack.Encode(txns[i]))
		actual := base64.StdEncoding.EncodeToString(msgpack.Encode(*row.Txn))
		assert.Equal(t, expected, actual)
		i++
	}

	deleteStatus, err = db.getMetastate(context.Background(), nil, schema.DeleteStatusKey)
	assert.NoError(t, err)
	status, err = encoding.DecodeDeleteStatus([]byte(deleteStatus))
	assert.NoError(t, err)
	assert.Equal(t, uint64(5), status.OldestRound)
}
