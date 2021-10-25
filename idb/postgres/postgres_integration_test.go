package postgres

import (
	"context"
	"database/sql"
	"math"
	"sync"
	"testing"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
	"github.com/algorand/indexer/util/test"
)

// TestMaxRoundOnUninitializedDB makes sure we return 0 when getting the max round on a new DB.
func TestMaxRoundOnUninitializedDB(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	db, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

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
	db.Exec(
		context.Background(),
		`INSERT INTO metastate (k, v) values ($1, $2)`,
		"state",
		`{"next_account_round":123454322}`)

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
	db.Exec(
		context.Background(),
		`INSERT INTO metastate (k, v) values ($1, $2)`,
		"state",
		`{"next_account_round":0}`)

	round, err := pdb.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)

	round, err = pdb.getMaxRoundAccounted(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)
}

func assertAccountAsset(t *testing.T, db *pgxpool.Pool, addr basics.Address, assetid uint64, frozen bool, amount uint64) {
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	assetid := uint64(1)
	amt := uint64(10000)
	total := uint64(1000000)

	///////////
	// Given // A round scenario requiring subround accounting: AccountA is funded, closed, opts back, and funded again.
	///////////
	createAsset := test.MakeAssetConfigTxn(
		0, total, uint64(6), false, "mcn", "my coin", "http://antarctica.com", test.AccountD)
	optInA := test.MakeAssetOptInTxn(assetid, test.AccountA)
	fundA := test.MakeAssetTransferTxn(
		assetid, amt, test.AccountD, test.AccountA, basics.Address{})
	optInB := test.MakeAssetOptInTxn(assetid, test.AccountB)
	optInC := test.MakeAssetOptInTxn(assetid, test.AccountC)
	closeA := test.MakeAssetTransferTxn(
		assetid, 1000, test.AccountA, test.AccountB, test.AccountC)
	payMain := test.MakeAssetTransferTxn(
		assetid, amt, test.AccountD, test.AccountA, basics.Address{})

	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createAsset, &optInA, &fundA, &optInB,
		&optInC, &closeA, &optInA, &payMain)
	require.NoError(t, err)

	//////////
	// When // We commit the block to the database
	//////////
	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	total := uint64(1000000)

	block := test.MakeGenesisBlock()
	for i, frozen := range []bool{true, false} {
		assetid := uint64(1 + 5*i)
		///////////
		// Given //
		// A new asset with default-frozen, AccountB opts-in and has its frozen state
		// toggled.
		/////////// Then AccountB opts-out then opts-in again.
		createAssetFrozen := test.MakeAssetConfigTxn(
			0, total, uint64(6), frozen, "icicles", "frozen coin",
			"http://antarctica.com", test.AccountA)
		optinB := test.MakeAssetOptInTxn(assetid, test.AccountB)
		unfreezeB := test.MakeAssetFreezeTxn(
			assetid, !frozen, test.AccountA, test.AccountB)
		optoutB := test.MakeAssetTransferTxn(
			assetid, 0, test.AccountB, test.AccountC, test.AccountD)

		var err error
		block, err = test.MakeBlockForTxns(
			block.BlockHeader, &createAssetFrozen, &optinB, &unfreezeB,
			&optoutB, &optinB)
		require.NoError(t, err)

		//////////
		// When // We commit the round accounting to the database.
		//////////
		err = db.AddBlock(&block)
		require.NoError(t, err)

		//////////
		// Then // AccountB should have its frozen state set back to the default value
		//////////
		assertAccountAsset(t, db.db, test.AccountB, assetid, frozen, 0)
	}
}

// TestMultipleAssetOptins make sure no-op transactions don't reset the default frozen value.
func TestNoopOptins(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given //
	// An asset with default-frozen = true, AccountB opts in, is unfrozen, then has a
	// no-op opt-in
	///////////
	assetid := uint64(1)

	createAsset := test.MakeAssetConfigTxn(
		0, uint64(1000000), uint64(6), true, "icicles", "frozen coin",
		"http://antarctica.com", test.AccountD)
	optinB := test.MakeAssetOptInTxn(assetid, test.AccountB)
	unfreezeB := test.MakeAssetFreezeTxn(assetid, false, test.AccountD, test.AccountB)

	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createAsset, &optinB, &unfreezeB, &optinB)
	require.NoError(t, err)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = db.AddBlock(&block)
	require.NoError(t, err)

	//////////
	// Then // AccountB should have its frozen state set back to the default value
	//////////
	assertAccountAsset(t, db.db, test.AccountB, assetid, false, 0)
}

// TestMultipleWriters tests that accounting cannot be double committed.
func TestMultipleWriters(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	amt := uint64(10000)

	///////////
	// Given // Send amt to AccountE
	///////////
	payAccountE := test.MakePaymentTxn(
		1000, amt, 0, 0, 0, 0, test.AccountD, test.AccountE, basics.Address{},
		basics.Address{})

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &payAccountE)
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
			errors <- db.AddBlock(&block)
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

// TestBlockWithTransactions tests that the block with transactions endpoint works.
func TestBlockWithTransactions(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	round := uint64(1)
	assetid := uint64(1)
	amt := uint64(10000)
	total := uint64(1000000)

	///////////
	// Given // A block at round `round` with 5 transactions.
	///////////
	txn1 := test.MakeAssetConfigTxn(
		0, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com",
		test.AccountD)
	txn2 := test.MakeAssetOptInTxn(assetid, test.AccountA)
	txn3 := test.MakeAssetTransferTxn(
		assetid, amt, test.AccountD, test.AccountA, basics.Address{})
	txn4 := test.MakeAssetOptInTxn(assetid, test.AccountB)
	txn5 := test.MakeAssetOptInTxn(assetid, test.AccountC)
	txn6 := test.MakeAssetTransferTxn(
		assetid, 1000, test.AccountA, test.AccountB, test.AccountC)
	txn7 := test.MakeAssetTransferTxn(
		assetid, 0, test.AccountA, test.AccountA, basics.Address{})
	txn8 := test.MakeAssetTransferTxn(
		assetid, amt, test.AccountD, test.AccountA, basics.Address{})
	txns := []*transactions.SignedTxnWithAD{
		&txn1, &txn2, &txn3, &txn4, &txn5, &txn6, &txn7, &txn8}

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, txns...)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	//////////
	// When // We call GetBlock and Transactions
	//////////
	_, txnRows0, err := db.GetBlock(
		context.Background(), round, idb.GetBlockOptions{Transactions: true})
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
	assert.Len(t, txnRows0, len(txns))
	assert.Len(t, txnRows1, len(txns))
	for i := 0; i < len(txnRows0); i++ {
		expected := protocol.Encode(txns[i])
		assert.Equal(t, expected, txnRows0[i].TxnBytes)
		assert.Equal(t, expected, txnRows1[i].TxnBytes)
	}
}

func TestRekeyBasic(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	txn := test.MakePaymentTxn(
		1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, test.AccountB)
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err)

	//////////
	// Then // Account A is rekeyed to account B
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(context.Background(), `SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	ad, err := encoding.DecodeTrimmedAccountData(accountDataStr)
	require.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, test.AccountB, ad.AuthAddr)
}

func TestRekeyToItself(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transactions
	///////////
	txn := test.MakePaymentTxn(
		1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, test.AccountB)
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err)

	txn = test.MakePaymentTxn(
		1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{},
		test.AccountA)
	block, err = test.MakeBlockForTxns(block.BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err)

	//////////
	// Then // Account's A auth-address is not recorded
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(context.Background(), `SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	ad, err := encoding.DecodeTrimmedAccountData(accountDataStr)
	require.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, basics.Address{}, ad.AuthAddr)
}

func TestRekeyThreeTimesInSameRound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	txn0 := test.MakePaymentTxn(
		1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{},
		test.AccountB)
	txn1 := test.MakePaymentTxn(
		1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{},
		basics.Address{})
	txn2 := test.MakePaymentTxn(
		1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, test.AccountC)
	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &txn0, &txn1, &txn2)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err)

	//////////
	// Then // Account A is rekeyed to account C
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(context.Background(), `SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	ad, err := encoding.DecodeTrimmedAccountData(accountDataStr)
	require.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, test.AccountC, ad.AuthAddr)
}

func TestRekeyToItselfHasNotBeenRekeyed(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	txn := test.MakePaymentTxn(
		1000, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{},
		basics.Address{})
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)

	//////////
	// Then // No error when committing to the DB.
	//////////
	err = db.AddBlock(&block)
	require.NoError(t, err)
}

// TestIgnoreDefaultFrozenConfigUpdate the creator asset holding should ignore default-frozen = true.
func TestIgnoreDefaultFrozenConfigUpdate(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	assetid := uint64(1)
	total := uint64(1000000)

	///////////
	// Given // A new asset with default-frozen = true, and AccountB opting into it.
	///////////
	createAssetNotFrozen := test.MakeAssetConfigTxn(
		0, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com",
		test.AccountA)
	modifyAssetToFrozen := test.MakeAssetConfigTxn(
		assetid, total, uint64(6), true, "icicles", "frozen coin", "http://antarctica.com",
		test.AccountA)
	optin := test.MakeAssetOptInTxn(assetid, test.AccountB)

	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createAssetNotFrozen, &modifyAssetToFrozen,
		&optin)
	require.NoError(t, err)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	assetid := uint64(1)
	total := uint64(0)

	///////////
	// Given // A new asset with total = 0.
	///////////
	createAsset := test.MakeAssetConfigTxn(
		0, total, uint64(6), false, "mcn", "my coin", "http://antarctica.com",
		test.AccountA)
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &createAsset)
	require.NoError(t, err)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = db.AddBlock(&block)
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

func assertAssetHoldingDates(t *testing.T, db *pgxpool.Pool, address basics.Address, assetID uint64, deleted sql.NullBool, createdAt sql.NullInt64, closedAt sql.NullInt64) {
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	assetID := uint64(1)

	// Create an asset.
	txn := test.MakeAssetConfigTxn(0, 4, 0, false, "uu", "aa", "", test.AccountA)
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err)

	// Destroy an asset.
	txn = test.MakeAssetDestroyTxn(assetID, test.AccountA)
	block, err = test.MakeBlockForTxns(block.BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	assetID := uint64(1)

	// Create an asset. Set total supply to 0.
	txn0 := test.MakeAssetConfigTxn(0, 0, 0, false, "uu", "aa", "", test.AccountA)
	txn1 := test.MakeAssetDestroyTxn(assetID, test.AccountA)
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0, &txn1)
	require.NoError(t, err)

	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	assetID := uint64(1)

	// Create an asset. Create a transaction where all special addresses are different
	// from creator's address.
	txn0 := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "acfg",
				Header: transactions.Header{
					Sender:      test.AccountA,
					GenesisHash: test.GenesisHash,
				},
				AssetConfigTxnFields: transactions.AssetConfigTxnFields{
					AssetParams: basics.AssetParams{
						Manager:  test.AccountB,
						Reserve:  test.AccountB,
						Freeze:   test.AccountB,
						Clawback: test.AccountB,
					},
				},
			},
			Sig: test.Signature,
		},
	}

	// Another account opts in.
	txn1 := test.MakeAssetOptInTxn(assetID, test.AccountC)

	// Destroy an asset.
	txn2 := test.MakeAssetDestroyTxn(assetID, test.AccountB)

	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &txn0, &txn1, &txn2)
	require.NoError(t, err)
	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // A block containing an asset freeze txn
	///////////

	// Create a block with freeze txn
	assetid := uint64(1)

	createAsset := test.MakeAssetConfigTxn(
		0, uint64(1000000), uint64(6), false, "mcn", "my coin", "http://antarctica.com",
		test.AccountA)
	optinB := test.MakeAssetOptInTxn(assetid, test.AccountB)
	freeze := test.MakeAssetFreezeTxn(assetid, true, test.AccountA, test.AccountB)

	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createAsset, &optinB, &freeze)
	require.NoError(t, err)

	//////////
	// When // We import the block.
	//////////
	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	///////////
	// Given // A block containing an app call txn with inners
	///////////

	// In order to simplify the test,
	// since db.AddBlock uses ApplyData from the block and not from the evaluator,
	// fake ApplyData to have inner txn
	// otherwise it requires funding the app account and other special setup
	var appAddr basics.Address
	appAddr[1] = 99
	createApp := test.MakeAppCallWithInnerTxn(test.AccountA, appAddr, test.AccountB, appAddr, test.AccountC)

	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createApp)
	require.NoError(t, err)

	//////////
	// When // We import the block.
	//////////
	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	// Create an app.

	// Create a transaction with ExtraProgramPages field set to 1
	const extraPages = 1
	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      test.AccountA,
					GenesisHash: test.GenesisHash,
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					ExtraProgramPages: extraPages,
				},
			},
			Sig: test.Signature,
		},
	}

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err, "failed to commit")

	row := db.db.QueryRow(context.Background(), "SELECT index, params FROM app WHERE creator = $1", test.AccountA[:])

	var index uint64
	var paramsStr []byte
	err = row.Scan(&index, &paramsStr)
	require.NoError(t, err)
	require.NotZero(t, index)

	ap, err := encoding.DecodeAppParams(paramsStr)
	require.NoError(t, err)
	require.Equal(t, uint32(1), ap.ExtraProgramPages)

	var filter generated.SearchForApplicationsParams
	var aidx uint64 = uint64(index)
	filter.ApplicationId = &aidx
	appRows, _ := db.Applications(context.Background(), &filter)
	num := 0
	for row := range appRows {
		require.NoError(t, row.Error)
		num++
		require.NotNil(t, row.Application.Params.ExtraProgramPages, "we should have this field")
		require.Equal(t, uint64(1), *row.Application.Params.ExtraProgramPages)
	}
	require.Equal(t, 1, num)

	rows, _ := db.GetAccounts(context.Background(), idb.AccountQueryOptions{EqualToAddress: test.AccountA[:]})
	num = 0
	var createdApps *[]generated.Application
	for row := range rows {
		require.NoError(t, row.Error)
		num++
		require.NotNil(t, row.Account.AppsTotalExtraPages, "we should have this field")
		require.Equal(t, uint64(1), *row.Account.AppsTotalExtraPages)
		createdApps = row.Account.CreatedApps
	}
	require.Equal(t, 1, num)

	require.NotNil(t, createdApps)
	require.Equal(t, 1, len(*createdApps))
	app := (*createdApps)[0]
	require.NotNil(t, app.Params.ExtraProgramPages)
	require.Equal(t, uint64(extraPages), *app.Params.ExtraProgramPages)
}

func assertKeytype(t *testing.T, db *IndexerDb, address basics.Address, keytype *string) {
	opts := idb.AccountQueryOptions{
		EqualToAddress: address[:],
	}
	rowsCh, _ := db.GetAccounts(context.Background(), opts)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	assert.Equal(t, keytype, row.Account.SigType)
}

func TestKeytypeBasic(t *testing.T) {
	block := test.MakeGenesisBlock()
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), block)
	defer shutdownFunc()

	assertKeytype(t, db, test.AccountA, nil)

	// Sig
	txn := test.MakePaymentTxn(
		0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})

	block, err := test.MakeBlockForTxns(block.BlockHeader, &txn)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	keytype := "sig"
	assertKeytype(t, db, test.AccountA, &keytype)

	// Msig
	txn = test.MakePaymentTxn(
		0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	txn.Sig = crypto.Signature{}
	txn.Msig.Subsigs = append(txn.Msig.Subsigs, crypto.MultisigSubsig{})

	block, err = test.MakeBlockForTxns(block.BlockHeader, &txn)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	keytype = "msig"
	assertKeytype(t, db, test.AccountA, &keytype)
}

// Test that asset amount >= 2^63 is handled correctly. Due to the specifics of
// postgres it might be a problem.
func TestLargeAssetAmount(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	assetid := uint64(1)
	txn := test.MakeAssetConfigTxn(
		0, math.MaxUint64, 0, false, "mc", "mycoin", "", test.AccountA)
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
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

	_, ok := <-availableCh
	assert.False(t, ok)

	state, err := db.getMigrationState(nil)
	require.NoError(t, err)

	assert.Equal(t, len(migrations), state.NextMigration)
}

// Test that opening the database the second time (after initializing) is successful.
func TestOpenDbAgain(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()

	_, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	_, _, err = OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)
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
		Name              string
		AssetName         string
		AssetUnit         string
		AssetURL          string
		ExpectedAssetName string
		ExpectedAssetUnit string
		ExpectedAssetURL  string
	}{
		{
			Name:              "Normal",
			AssetName:         "asset-name",
			AssetUnit:         "au",
			AssetURL:          "https://algorand.com",
			ExpectedAssetName: "asset-name",
			ExpectedAssetUnit: "au",
			ExpectedAssetURL:  "https://algorand.com",
		},
		{
			Name:              "Embedded Null",
			AssetName:         "asset\000name",
			AssetUnit:         "a\000u",
			AssetURL:          "https:\000//algorand.com",
			ExpectedAssetName: "",
			ExpectedAssetUnit: "",
			ExpectedAssetURL:  "",
		},
		{
			Name:              "Invalid UTF8",
			AssetName:         "asset\x8cname",
			AssetUnit:         "a\x8cu",
			AssetURL:          "https:\x8c//algorand.com",
			ExpectedAssetName: "",
			ExpectedAssetUnit: "",
			ExpectedAssetURL:  "",
		},
		{
			Name:              "Emoji",
			AssetName:         "💩",
			AssetUnit:         "💰",
			AssetURL:          "🌐",
			ExpectedAssetName: "💩",
			ExpectedAssetUnit: "💰",
			ExpectedAssetURL:  "🌐",
		},
	}

	assetID := uint64(1)

	for _, testcase := range tests {
		testcase := testcase
		name := testcase.AssetName
		unit := testcase.AssetUnit
		url := testcase.AssetURL

		t.Run(testcase.Name, func(t *testing.T) {
			db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
			defer shutdownFunc()

			txn := test.MakeAssetConfigTxn(
				0, math.MaxUint64, 0, false, unit, name, url, test.AccountA)
			block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
			require.NoError(t, err)

			// Test 1: import/accounting should work.
			err = db.AddBlock(&block)
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
			txnRows, _ := db.Transactions(context.Background(), idb.TransactionFilter{})
			num = 0
			for row := range txnRows {
				require.NoError(t, row.Error)
				// Note: These are created from the TxnBytes, so they have the exact name with embedded null.
				var txn transactions.SignedTxn
				require.NoError(t, protocol.Decode(row.TxnBytes, &txn))
				require.Equal(t, name, txn.Txn.AssetParams.AssetName)
				require.Equal(t, unit, txn.Txn.AssetParams.UnitName)
				require.Equal(t, url, txn.Txn.AssetParams.URL)
				num++
			}
			require.Equal(t, 1, num)

			// Test 4: account results should have the correct asset
			accounts, _ := db.GetAccounts(context.Background(), idb.AccountQueryOptions{EqualToAddress: test.AccountA[:], IncludeAssetParams: true})
			num = 0
			for acct := range accounts {
				require.NoError(t, acct.Error)
				require.NotNil(t, acct.Account.CreatedAssets)
				require.Len(t, *acct.Account.CreatedAssets, 1)

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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	unit := "co\000in"
	name := "algo"
	url := "https://algorand.com"
	assetID := uint64(1)

	txn := test.MakeAssetConfigTxn(
		0, math.MaxUint64, 0, false, unit, name, url, test.AccountA)
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	txn = transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "acfg",
				Header: transactions.Header{
					Sender:      test.AccountA,
					Fee:         basics.MicroAlgos{Raw: 1000},
					GenesisHash: test.GenesisHash,
				},
				AssetConfigTxnFields: transactions.AssetConfigTxnFields{
					ConfigAsset: basics.AssetIndex(assetID),
					AssetParams: basics.AssetParams{
						Freeze:   test.AccountB,
						Clawback: test.AccountC,
					},
				},
			},
			Sig: test.Signature,
		},
	}
	block, err = test.MakeBlockForTxns(block.BlockHeader, &txn)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	// Test 2: asset results properly serialized
	assets, _ := db.Assets(context.Background(), idb.AssetsQuery{AssetID: assetID})
	num := 0
	for asset := range assets {
		require.NoError(t, asset.Error)
		require.Equal(t, name, asset.Params.AssetName)
		require.Equal(t, unit, asset.Params.UnitName)
		require.Equal(t, url, asset.Params.URL)

		require.Equal(t, basics.Address{}, asset.Params.Manager, "Manager should have been cleared.")
		require.Equal(t, basics.Address{}, asset.Params.Reserve, "Reserve should have been cleared.")
		// These were updated
		require.Equal(t, test.AccountB, asset.Params.Freeze)
		require.Equal(t, test.AccountC, asset.Params.Clawback)
		num++
	}
	require.Equal(t, 1, num)
}

func TestKeytypeResetsOnRekey(t *testing.T) {
	block := test.MakeGenesisBlock()
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), block)
	defer shutdownFunc()

	// Sig
	txn := test.MakePaymentTxn(
		0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})

	block, err := test.MakeBlockForTxns(block.BlockHeader, &txn)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	keytype := "sig"
	assertKeytype(t, db, test.AccountA, &keytype)

	// Rekey.
	txn = test.MakePaymentTxn(
		0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, test.AccountB)

	block, err = test.MakeBlockForTxns(block.BlockHeader, &txn)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	assertKeytype(t, db, test.AccountA, nil)

	// Msig
	txn = test.MakePaymentTxn(
		0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	txn.Sig = crypto.Signature{}
	txn.Msig.Subsigs = append(txn.Msig.Subsigs, crypto.MultisigSubsig{})
	txn.AuthAddr = test.AccountB

	block, err = test.MakeBlockForTxns(block.BlockHeader, &txn)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	keytype = "msig"
	assertKeytype(t, db, test.AccountA, &keytype)
}

// TestAddBlockGenesis tests that adding block 0 is successful.
func TestAddBlockGenesis(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	opts := idb.GetBlockOptions{
		Transactions: true,
	}
	blockHeaderRet, txns, err := db.GetBlock(context.Background(), 0, opts)
	require.NoError(t, err)
	assert.Empty(t, txns)
	assert.Equal(t, test.MakeGenesisBlock().BlockHeader, blockHeaderRet)

	nextRound, err := db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), nextRound)
}

// TestAddBlockAssetCloseAmountInTxnExtra tests that we set the correct asset close
// amount in `txn.extra` column.
func TestAddBlockAssetCloseAmountInTxnExtra(t *testing.T) {
	// Use an old version of consensus parameters that have AssetCloseAmount = false.
	genesis := test.MakeGenesis()
	genesis.Proto = protocol.ConsensusV24
	block := test.MakeGenesisBlock()
	block.UpgradeState.CurrentProtocol = protocol.ConsensusV24

	db, shutdownFunc := setupIdb(t, genesis, block)
	defer shutdownFunc()

	assetid := uint64(1)

	createAsset := test.MakeAssetConfigTxn(
		0, uint64(1000000), uint64(6), false, "mcn", "my coin", "http://antarctica.com",
		test.AccountA)
	optinB := test.MakeAssetOptInTxn(assetid, test.AccountB)
	transferAB := test.MakeAssetTransferTxn(
		assetid, 100, test.AccountA, test.AccountB, basics.Address{})
	optinC := test.MakeAssetOptInTxn(assetid, test.AccountC)
	// Close B to C.
	closeB := test.MakeAssetTransferTxn(
		assetid, 30, test.AccountB, test.AccountA, test.AccountC)

	block, err := test.MakeBlockForTxns(
		block.BlockHeader, &createAsset, &optinB, &transferAB,
		&optinC, &closeB)
	require.NoError(t, err)

	err = db.AddBlock(&block)
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

	err = db.LoadGenesis(test.MakeGenesis())
	require.NoError(t, err)

	round, err := db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)

	block := test.MakeGenesisBlock()
	err = db.AddBlock(&block)
	require.NoError(t, err)

	round, err = db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), round)

	block, err = test.MakeBlockForTxns(block.BlockHeader)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	round, err = db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(2), round)

	block, err = test.MakeBlockForTxns(block.BlockHeader)
	require.NoError(t, err)
	err = db.AddBlock(&block)
	require.NoError(t, err)

	round, err = db.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(3), round)
}

// Test that AddBlock makes a record of an account that gets created and deleted in
// the same round.
func TestAddBlockCreateDeleteAccountSameRound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	createTxn := test.MakePaymentTxn(
		0, 5, 0, 0, 0, 0, test.AccountA, test.AccountE, basics.Address{}, basics.Address{})
	deleteTxn := test.MakePaymentTxn(
		0, 2, 3, 0, 0, 0, test.AccountE, test.AccountB, test.AccountC, basics.Address{})
	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createTxn, &deleteTxn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	assetid := uint64(1)
	createTxn := test.MakeAssetConfigTxn(0, 3, 0, false, "", "", "", test.AccountA)
	deleteTxn := test.MakeAssetDestroyTxn(assetid, test.AccountA)
	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createTxn, &deleteTxn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	appid := uint64(1)
	createTxn := test.MakeCreateAppTxn(test.AccountA)
	deleteTxn := test.MakeAppDestroyTxn(appid, test.AccountA)
	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createTxn, &deleteTxn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err)

	yes := true
	opts := generated.SearchForApplicationsParams{
		ApplicationId: &appid,
		IncludeAll:    &yes,
	}
	rowsCh, _ := db.Applications(context.Background(), &opts)

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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	appid := uint64(1)
	createTxn := test.MakeCreateAppTxn(test.AccountA)
	optInTxn := test.MakeAppOptInTxn(appid, test.AccountB)
	optOutTxn := test.MakeAppOptOutTxn(appid, test.AccountB)
	block, err := test.MakeBlockForTxns(
		test.MakeGenesisBlock().BlockHeader, &createTxn, &optInTxn, &optOutTxn)
	require.NoError(t, err)

	err = db.AddBlock(&block)
	require.NoError(t, err)

	opts := idb.AccountQueryOptions{
		EqualToAddress: test.AccountB[:],
		IncludeDeleted: true,
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
}

// TestNonUTF8Logs makes sure we're able to import cheeky logs.
func TestNonUTF8Logs(t *testing.T) {
	tests := []struct {
		Name string
		Logs []string
	}{
		{
			Name: "Normal",
			Logs: []string{"Test log1", "Test log2", "Test log3"},
		},
		{
			Name: "Embedded Null",
			Logs: []string{"\000", "\x00\x00\x00\x00\x00\x00\x00\x00", string([]byte{00, 00})},
		},
		{
			Name: "Invalid UTF8",
			Logs: []string{"\x8c", "\xff", "\xf8"},
		},
		{
			Name: "Emoji",
			Logs: []string{"💩", "💰", "🌐"},
		},
	}

	for _, testcase := range tests {
		testcase := testcase

		t.Run(testcase.Name, func(t *testing.T) {
			db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
			defer shutdownFunc()

			createAppTxn := test.MakeCreateAppTxn(test.AccountA)
			createAppTxn.ApplyData.EvalDelta = transactions.EvalDelta{
				Logs: testcase.Logs,
			}

			block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &createAppTxn)
			require.NoError(t, err)

			// Test 1: import/accounting should work.
			err = db.AddBlock(&block)
			require.NoError(t, err)

			// Test 2: transaction results properly serialized
			txnRows, _ := db.Transactions(context.Background(), idb.TransactionFilter{})
			for row := range txnRows {
				require.NoError(t, row.Error)
				var txn transactions.SignedTxnWithAD
				require.NoError(t, protocol.Decode(row.TxnBytes, &txn))
				require.Equal(t, testcase.Logs, txn.ApplyData.EvalDelta.Logs)
			}
		})
	}
}

// Test that LoadGenesis writes account totals.
func TestLoadGenesisAccountTotals(t *testing.T) {
	_, connStr, shutdownFunc := pgtest.SetupPostgres(t)
	defer shutdownFunc()
	db, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	err = db.LoadGenesis(test.MakeGenesis())
	require.NoError(t, err)

	json, err := db.getMetastate(context.Background(), nil, schema.AccountTotals)
	require.NoError(t, err)

	ret, err := encoding.DecodeAccountTotals([]byte(json))
	require.NoError(t, err)

	assert.Equal(
		t, basics.MicroAlgos{Raw: 4 * 1000 * 1000 * 1000 * 1000}, ret.Offline.Money)
}
