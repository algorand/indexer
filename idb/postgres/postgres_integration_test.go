package postgres

import (
	"context"
	"database/sql"
	"math"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
	"github.com/algorand/indexer/util/test"
)

// getAccounting initializes the ac counting state for testing.
func getAccounting(round uint64, cache map[uint64]bool) *accounting.State {
	accountingState := accounting.New(cache)
	accountingState.InitRoundParts(round, test.FeeAddr, test.RewardAddr, 0)
	return accountingState
}

// TestMaxRoundOnUninitializedDB makes sure we return 0 when getting the max round on a new DB.
func TestMaxRoundOnUninitializedDB(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	round, err := db.GetNextRoundToAccount()
	assert.Equal(t, err, idb.ErrorNotInitialized)
	assert.Equal(t, uint64(0), round)

	round, err = db.getMaxRoundAccounted(nil)
	assert.Equal(t, err, idb.ErrorNotInitialized)
	assert.Equal(t, uint64(0), round)

	round, err = db.GetNextRoundToLoad()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)
}

// TestMaxRoundEmptyMetastate makes sure we return 0 when the metastate is empty.
func TestMaxRoundEmptyMetastate(t *testing.T) {
	pg, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	pg.Exec(`INSERT INTO metastate (k, v) values ('state', '{}')`)

	round, err := db.GetNextRoundToAccount()
	assert.Equal(t, err, idb.ErrorNotInitialized)
	assert.Equal(t, uint64(0), round)

	round, err = db.getMaxRoundAccounted(nil)
	assert.Equal(t, err, idb.ErrorNotInitialized)
	assert.Equal(t, uint64(0), round)
}

// TestMaxRound the happy path.
func TestMaxRound(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	pdb, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	db.Exec(
		`INSERT INTO metastate (k, v) values ($1, $2)`,
		"state",
		`{"next_account_round":123454322}`)
	db.Exec(
		`INSERT INTO block_header (round, realtime, rewardslevel, header) `+
			`VALUES ($1, NOW(), 0, '{}') ON CONFLICT DO NOTHING`,
		543212345)

	round, err := pdb.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(123454322), round)

	round, err = pdb.getMaxRoundAccounted(nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(123454321), round)

	round, err = pdb.GetNextRoundToLoad()
	require.NoError(t, err)
	assert.Equal(t, uint64(543212346), round)
}

func TestAccountedRoundNextRound0(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	pdb, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	db.Exec(
		`INSERT INTO metastate (k, v) values ($1, $2)`,
		"state",
		`{"next_account_round":0}`)

	round, err := pdb.GetNextRoundToAccount()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)

	round, err = pdb.getMaxRoundAccounted(nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), round)
}

func assertAccountAsset(t *testing.T, db *sql.DB, addr sdk_types.Address, assetid uint64, frozen bool, amount uint64) {
	var row *sql.Row
	var f bool
	var a uint64

	row = db.QueryRow(`SELECT frozen, amount FROM account_asset as a WHERE a.addr = $1 AND assetid = $2`, addr[:], assetid)
	err := row.Scan(&f, &a)
	assert.NoError(t, err, "failed looking up AccountA.")
	assert.Equal(t, frozen, f)
	assert.Equal(t, amount, a)
}

// TestAssetCloseReopenTransfer tests a scenario that requires asset subround accounting
func TestAssetCloseReopenTransfer(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64(2222)
	amt := uint64(10000)
	total := uint64(1000000)

	///////////
	// Given // A round scenario requiring subround accounting: AccountA is funded, closed, opts back, and funded again.
	///////////
	_, createAsset := test.MakeAssetConfigOrPanic(test.Round, 0, assetid, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com", test.AccountD)
	_, fundMain := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountD, test.AccountA, sdk_types.ZeroAddress)
	_, closeMain := test.MakeAssetTxnOrPanic(test.Round, assetid, 1000, test.AccountA, test.AccountB, test.AccountC)
	_, optinMain := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountA, test.AccountA, sdk_types.ZeroAddress)
	_, payMain := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountD, test.AccountA, sdk_types.ZeroAddress)

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(createAsset)
	state.AddTransaction(fundMain)
	state.AddTransaction(closeMain)
	state.AddTransaction(optinMain)
	state.AddTransaction(payMain)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

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

// TestDefaultFrozenAndCache checks that values are added to the default frozen cache, and that the cache is used when
// accounts optin to an asset.
func TestDefaultFrozenAndCache(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64(2222)
	total := uint64(1000000)

	///////////
	// Given // A new asset with default-frozen = true, and AccountB opting into it.
	///////////
	_, createAssetFrozen := test.MakeAssetConfigOrPanic(test.Round, 0, assetid, total, uint64(6), true, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)
	_, createAssetNotFrozen := test.MakeAssetConfigOrPanic(test.Round, 0, assetid+1, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)
	_, optinB1 := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountB, test.AccountB, sdk_types.ZeroAddress)
	_, optinB2 := test.MakeAssetTxnOrPanic(test.Round, assetid+1, 0, test.AccountB, test.AccountB, sdk_types.ZeroAddress)

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(createAssetFrozen)
	state.AddTransaction(createAssetNotFrozen)
	state.AddTransaction(optinB1)
	state.AddTransaction(optinB2)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

	//////////
	// Then // Make sure the accounts have the correct default-frozen after create/optin
	//////////
	// default-frozen = true
	assertAccountAsset(t, db.db, test.AccountA, assetid, false, total) // the creator ignores default-frozen
	assertAccountAsset(t, db.db, test.AccountB, assetid, true, 0)

	// default-frozen = false
	assertAccountAsset(t, db.db, test.AccountA, assetid+1, false, total)
	assertAccountAsset(t, db.db, test.AccountB, assetid+1, false, 0)
}

// TestInitializeFrozenCache checks that the frozen cache is properly initialized on startup.
func TestInitializeFrozenCache(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	// Initialize DB by creating one of these things.
	_, err := idb.IndexerDbByName("postgres", connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	// Add some assets
	_, err = db.Exec(
		`INSERT INTO asset (index, creator_addr, params, deleted) values ($1, $2, $3, false)`,
		1, test.AccountA[:], `{"df":true}`)
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO asset (index, creator_addr, params, deleted) values ($1, $2, $3, false)`,
		2, test.AccountA[:], `{"df":false}`)
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO asset (index, creator_addr, params, deleted) values ($1, $2, $3, false)`,
		3, test.AccountA[:], `{}`)
	require.NoError(t, err)

	pdb, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)
	cache, err := pdb.GetDefaultFrozen()
	assert.NoError(t, err)

	assert.Len(t, cache, 1)
	assert.True(t, cache[1])
	assert.False(t, cache[2])
	assert.False(t, cache[3])
	assert.False(t, cache[300000])
}

// TestReCreateAssetHolding checks that the optin value of a defunct
func TestReCreateAssetHolding(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64(2222)
	total := uint64(1000000)

	tests := []struct {
		offset uint64
		frozen bool
	}{
		{
			offset: 0,
			frozen: true,
		},
		{
			offset: 1,
			frozen: false,
		},
	}

	for _, testcase := range tests {
		round := test.Round + testcase.offset
		aid := assetid + testcase.offset
		///////////
		// Given // A new asset with default-frozen, AccountB opts-in and has its frozen state toggled.
		/////////// Then AccountB opts-out then opts-in again.
		_, createAssetFrozen := test.MakeAssetConfigOrPanic(round, 0, aid, total, uint64(6), testcase.frozen, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)
		_, optinB := test.MakeAssetTxnOrPanic(round, aid, 0, test.AccountB, test.AccountB, sdk_types.ZeroAddress)
		_, unfreezeB := test.MakeAssetFreezeOrPanic(round, aid, !testcase.frozen, test.AccountB, test.AccountB)
		_, optoutB := test.MakeAssetTxnOrPanic(round, aid, 0, test.AccountB, test.AccountC, test.AccountD)

		cache, err := db.GetDefaultFrozen()
		assert.NoError(t, err)
		state := getAccounting(round, cache)
		state.AddTransaction(createAssetFrozen)
		state.AddTransaction(optinB)
		state.AddTransaction(unfreezeB)
		state.AddTransaction(optoutB)
		state.AddTransaction(optinB) // reuse optinB

		//////////
		// When // We commit the round accounting to the database.
		//////////
		err = db.CommitRoundAccounting(state.RoundUpdates, round, &types.BlockHeader{})
		assert.NoError(t, err, "failed to commit")

		//////////
		// Then // AccountB should have its frozen state set back to the default value
		//////////
		assertAccountAsset(t, db.db, test.AccountB, aid, testcase.frozen, 0)
	}
}

// TestMultipleAssetOptins make sure no-op transactions don't reset the default frozen value.
func TestNoopOptins(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // An asset with default-frozen = true, AccountB opt's in, is unfrozen, then has a no-op opt-in
	///////////

	assetid := uint64(2222)
	// create asst
	//db.Exec(`INSERT INTO asset (index, creator_addr, params) values ($1, $2, $3)`, assetid, test.AccountA[:], `{"df":true}`)

	_, createAsset := test.MakeAssetConfigOrPanic(test.Round, 0, assetid, uint64(1000000), uint64(6), true, "icicles", "frozen coin", "http://antarctica.com", test.AccountD)
	_, optinB := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountB, test.AccountB, sdk_types.ZeroAddress)
	_, unfreezeB := test.MakeAssetFreezeOrPanic(test.Round, assetid, false, test.AccountB, test.AccountB)

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(createAsset)
	state.AddTransaction(optinB)
	state.AddTransaction(unfreezeB)
	state.AddTransaction(optinB)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

	//////////
	// Then // AccountB should have its frozen state set back to the default value
	//////////
	// TODO: This isn't working yet
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
	_, payAccountE := test.MakePayTxnRowOrPanic(test.Round, 1000, amt, 0, 0, 0, 0, test.AccountD,
		test.AccountE, sdk_types.ZeroAddress, sdk_types.ZeroAddress)

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(payAccountE)

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
			errors <- db.CommitRoundAccounting(
				state.RoundUpdates, test.Round, &types.BlockHeader{})
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
	row := db.db.QueryRow(`SELECT microalgos FROM account WHERE account.addr = $1`, test.AccountE[:])
	err = row.Scan(&balance)
	assert.NoError(t, err, "checking balance")
	assert.Equal(t, amt, balance)
}

// TestBlockWithTransactions tests that the block with transactions endpoint works.
// TestBlockWithTransactions tests that the block with transactions endpoint works.
func TestBlockWithTransactions(t *testing.T) {
	var err error

	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	pdb, err := idb.IndexerDbByName("postgres", connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	assetid := uint64(2222)
	amt := uint64(10000)
	total := uint64(1000000)

	///////////
	// Given // A block at round test.Round with 5 transactions.
	///////////
	tx1, row1 := test.MakeAssetConfigOrPanic(test.Round, 0, assetid, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com", test.AccountD)
	tx2, row2 := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountD, test.AccountA, sdk_types.ZeroAddress)
	tx3, row3 := test.MakeAssetTxnOrPanic(test.Round, assetid, 1000, test.AccountA, test.AccountB, test.AccountC)
	tx4, row4 := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountA, test.AccountA, sdk_types.ZeroAddress)
	tx5, row5 := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountD, test.AccountA, sdk_types.ZeroAddress)
	txns := []*sdk_types.SignedTxnWithAD{tx1, tx2, tx3, tx4, tx5}
	txnRows := []*idb.TxnRow{row1, row2, row3, row4, row5}

	_, err = db.Exec(
		`INSERT INTO metastate (k, v) values ($1, $2)`, "state",
		`{"next_account_round": 12}`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO block_header (round, realtime, rewardslevel, header) VALUES ($1, NOW(), 0, '{}') ON CONFLICT DO NOTHING`, test.Round)
	require.NoError(t, err)
	for i := range txns {
		_, err = db.Exec(`INSERT INTO txn (round, intra, typeenum, asset, txid, txnbytes, txn) VALUES ($1, $2, $3, $4, $5, $6, $7)`, test.Round, i, 0, 0, crypto.TransactionID(txns[i].Txn), txnRows[i].TxnBytes, "{}")
		require.NoError(t, err)
	}

	//////////
	// When // We call GetBlock and Transactions
	//////////
	_, blockTxn, err := pdb.GetBlock(context.Background(), test.Round, idb.GetBlockOptions{Transactions: true})
	require.NoError(t, err)
	round := test.Round
	txnRow, _ := pdb.Transactions(context.Background(), idb.TransactionFilter{Round: &round})
	transactionsTxn := make([]idb.TxnRow, 0)
	for row := range txnRow {
		require.NoError(t, row.Error)
		transactionsTxn = append(transactionsTxn, row)
	}

	//////////
	// Then // They should have the same transactions
	//////////
	assert.Len(t, blockTxn, 5)
	assert.Len(t, transactionsTxn, 5)
	for i := 0; i < len(blockTxn); i++ {
		assert.Equal(t, txnRows[i].TxnBytes, blockTxn[i].TxnBytes)
		assert.Equal(t, txnRows[i].TxnBytes, transactionsTxn[i].TxnBytes)
	}
}

func TestRekeyBasic(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	_, txnRow := test.MakePayTxnRowOrPanic(test.Round, 1000, 0, 0, 0, 0, 0, test.AccountA,
		test.AccountA, sdk_types.ZeroAddress, test.AccountB)

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(txnRow)

	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

	//////////
	// Then // Account A is rekeyed to account B
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(`SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	var ad types.AccountData
	err = encoding.DecodeJSON(accountDataStr, &ad)
	assert.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, test.AccountB, ad.SpendingKey)
}

func TestRekeyToItself(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	{
		_, txnRow := test.MakePayTxnRowOrPanic(test.Round, 1000, 0, 0, 0, 0, 0, test.AccountA,
			test.AccountA, sdk_types.ZeroAddress, test.AccountB)

		cache, err := db.GetDefaultFrozen()
		assert.NoError(t, err)
		state := getAccounting(test.Round, cache)
		state.AddTransaction(txnRow)

		err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
		assert.NoError(t, err, "failed to commit")
	}
	{
		_, txnRow := test.MakePayTxnRowOrPanic(test.Round+1, 1000, 0, 0, 0, 0, 0, test.AccountA,
			test.AccountA, sdk_types.ZeroAddress, test.AccountA)

		cache, err := db.GetDefaultFrozen()
		assert.NoError(t, err)
		state := getAccounting(test.Round+1, cache)
		state.AddTransaction(txnRow)

		err = db.CommitRoundAccounting(
			state.RoundUpdates, test.Round+1, &types.BlockHeader{})
		assert.NoError(t, err, "failed to commit")
	}

	//////////
	// Then // Account's A auth-address is not recorded
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(`SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err := row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	var ad types.AccountData
	err = encoding.DecodeJSON(accountDataStr, &ad)
	assert.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, sdk_types.ZeroAddress, ad.SpendingKey)
}

func TestRekeyThreeTimesInSameRound(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)

	{
		_, txnRow := test.MakePayTxnRowOrPanic(test.Round, 1000, 0, 0, 0, 0, 0, test.AccountA,
			test.AccountA, sdk_types.ZeroAddress, test.AccountB)
		state.AddTransaction(txnRow)
	}
	{
		_, txnRow := test.MakePayTxnRowOrPanic(test.Round, 1000, 0, 0, 0, 0, 0, test.AccountA,
			test.AccountA, sdk_types.ZeroAddress, sdk_types.ZeroAddress)
		state.AddTransaction(txnRow)
	}
	{
		_, txnRow := test.MakePayTxnRowOrPanic(test.Round, 1000, 0, 0, 0, 0, 0, test.AccountA,
			test.AccountA, sdk_types.ZeroAddress, test.AccountC)
		state.AddTransaction(txnRow)
	}

	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

	//////////
	// Then // Account A is rekeyed to account C
	//////////
	var accountDataStr []byte
	row := db.db.QueryRow(`SELECT account_data FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&accountDataStr)
	assert.NoError(t, err, "querying account data")

	var ad types.AccountData
	err = encoding.DecodeJSON(accountDataStr, &ad)
	assert.NoError(t, err, "failed to parse account data json")
	assert.Equal(t, test.AccountC, ad.SpendingKey)
}

func TestRekeyToItselfHasNotBeenRekeyed(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	///////////
	// Given // Send rekey transaction
	///////////
	_, txnRow := test.MakePayTxnRowOrPanic(test.Round, 1000, 0, 0, 0, 0, 0, test.AccountA,
		test.AccountA, sdk_types.ZeroAddress, sdk_types.ZeroAddress)

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(txnRow)

	//////////
	// Then // No error when committing to the DB.
	//////////
	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")
}

// TestIgnoreDefaultFrozenConfigUpdate the creator asset holding should ignore default-frozen = true.
func TestIgnoreDefaultFrozenConfigUpdate(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64(2222)
	total := uint64(1000000)

	///////////
	// Given // A new asset with default-frozen = true, and AccountB opting into it.
	///////////
	_, createAssetNotFrozen := test.MakeAssetConfigOrPanic(test.Round, 0, assetid, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)
	_, modifyAssetToFrozen := test.MakeAssetConfigOrPanic(test.Round, assetid, assetid, total, uint64(6), true, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)
	_, optin := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountB, test.AccountB, sdk_types.ZeroAddress)

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(createAssetNotFrozen)
	state.AddTransaction(modifyAssetToFrozen)
	state.AddTransaction(optin)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

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

	assetid := uint64(2222)
	total := uint64(0)

	///////////
	// Given // A new asset with total = 0.
	///////////
	_, createAsset := test.MakeAssetConfigOrPanic(test.Round, 0, assetid, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(createAsset)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

	//////////
	// Then // Make sure the creator has an asset holding with amount = 0.
	//////////
	assertAccountAsset(t, db.db, test.AccountA, assetid, false, 0)
}

func assertAssetDates(t *testing.T, db *sql.DB, assetID uint64, deleted sql.NullBool, createdAt sql.NullInt64, closedAt sql.NullInt64) {
	row := db.QueryRow(
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

func assertAssetHoldingDates(t *testing.T, db *sql.DB, address sdk_types.Address, assetID uint64, deleted sql.NullBool, createdAt sql.NullInt64, closedAt sql.NullInt64) {
	row := db.QueryRow(
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

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)

	assetID := uint64(3)

	// Create an asset.
	{
		_, txnRow := test.MakeAssetConfigOrPanic(test.Round, 0, assetID, 4, 0, false, "uu", "aa", "",
			test.AccountA)

		state := getAccounting(test.Round, cache)
		err := state.AddTransaction(txnRow)
		assert.NoError(t, err)

		err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
		assert.NoError(t, err, "failed to commit")
	}
	// Destroy an asset.
	{
		_, txnRow := test.MakeAssetDestroyTxn(test.Round+1, assetID)

		state := getAccounting(test.Round+1, cache)
		err := state.AddTransaction(txnRow)
		assert.NoError(t, err)

		err = db.CommitRoundAccounting(state.RoundUpdates, test.Round+1, &types.BlockHeader{})
		assert.NoError(t, err, "failed to commit")
	}

	// Check that the asset is deleted.
	assertAssetDates(t, db.db, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)},
		sql.NullInt64{Valid: true, Int64: int64(test.Round + 1)})

	// Check that the account's asset holding is deleted.
	assertAssetHoldingDates(t, db.db, test.AccountA, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)},
		sql.NullInt64{Valid: true, Int64: int64(test.Round + 1)})
}

func TestDestroyAssetZeroSupply(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)

	assetID := uint64(3)

	state := getAccounting(test.Round, cache)

	// Create an asset.
	{
		// Set total supply to 0.
		_, txnRow := test.MakeAssetConfigOrPanic(test.Round, 0, assetID, 0, 0, false, "uu", "aa", "",
			test.AccountA)

		err := state.AddTransaction(txnRow)
		assert.NoError(t, err)
	}
	// Destroy an asset.
	{
		_, txnRow := test.MakeAssetDestroyTxn(test.Round, assetID)

		err := state.AddTransaction(txnRow)
		assert.NoError(t, err)
	}

	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

	// Check that the asset is deleted.
	assertAssetDates(t, db.db, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)})

	// Check that the account's asset holding is deleted.
	assertAssetHoldingDates(t, db.db, test.AccountA, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)})
}

func TestDestroyAssetDeleteCreatorsHolding(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	cache, err := db.GetDefaultFrozen()
	assert.NoError(t, err)

	assetID := uint64(3)

	state := getAccounting(test.Round, cache)

	// Create an asset.
	{
		// Create a transaction where all special addresses are different from creator's address.
		txn := sdk_types.SignedTxnWithAD{
			SignedTxn: sdk_types.SignedTxn{
				Txn: sdk_types.Transaction{
					Type: "acfg",
					Header: sdk_types.Header{
						Sender: test.AccountA,
					},
					AssetConfigTxnFields: sdk_types.AssetConfigTxnFields{
						AssetParams: sdk_types.AssetParams{
							Manager:  test.AccountB,
							Reserve:  test.AccountB,
							Freeze:   test.AccountB,
							Clawback: test.AccountB,
						},
					},
				},
			},
		}
		txnRow := idb.TxnRow{
			Round:    uint64(test.Round),
			TxnBytes: msgpack.Encode(txn),
			AssetID:  assetID,
		}

		err := state.AddTransaction(&txnRow)
		assert.NoError(t, err)
	}
	// Another account opts in.
	{
		_, txnRow := test.MakeAssetTxnOrPanic(test.Round, assetID, 0, test.AccountC,
			test.AccountC, sdk_types.ZeroAddress)
		state.AddTransaction(txnRow)
	}
	// Destroy an asset.
	{
		_, txnRow := test.MakeAssetDestroyTxn(test.Round, assetID)
		state.AddTransaction(txnRow)
	}

	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	assert.NoError(t, err, "failed to commit")

	// Check that the creator's asset holding is deleted.
	assertAssetHoldingDates(t, db.db, test.AccountA, assetID,
		sql.NullBool{Valid: true, Bool: true},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)})

	// Check that other account's asset holding was not deleted.
	assertAssetHoldingDates(t, db.db, test.AccountC, assetID,
		sql.NullBool{Valid: true, Bool: false},
		sql.NullInt64{Valid: true, Int64: int64(test.Round)},
		sql.NullInt64{Valid: false, Int64: 0})

	// Check that the manager does not have an asset holding.
	{
		count := queryInt(db.db, "SELECT COUNT(*) FROM account_asset WHERE addr = $1", test.AccountB[:])
		assert.Equal(t, 0, count)
	}
}

// Test that block import adds the freeze/sender accounts to txn_participation.
func TestAssetFreezeTxnParticipation(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	blockImporter := importer.NewDBImporter(db)

	///////////
	// Given // A block containing an asset freeze txn
	///////////

	// Create a block with freeze txn
	freeze, _ := test.MakeAssetFreezeOrPanic(test.Round, 1234, true, test.AccountA, test.AccountB)
	block := test.MakeBlockForTxns(test.Round, freeze)

	//////////
	// When // We import the block.
	//////////
	txnCount, err := blockImporter.ImportDecodedBlock(&block)
	assert.NoError(t, err, "failed to import")
	assert.Equal(t, 1, txnCount)

	//////////
	// Then // Both accounts should have an entry in the txn_participation table.
	//////////
	acctACount := queryInt(db.db, "SELECT COUNT(*) FROM txn_participation WHERE addr = $1", test.AccountA[:])
	acctBCount := queryInt(db.db, "SELECT COUNT(*) FROM txn_participation WHERE addr = $1", test.AccountB[:])
	assert.Equal(t, 1, acctACount)
	assert.Equal(t, 1, acctBCount)
}

func TestAppExtraPages(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	cache, err := db.GetDefaultFrozen()
	require.NoError(t, err)

	assetID := uint64(3)

	state := getAccounting(test.Round, cache)

	// Create an app.
	{
		// Create a transaction with ExtraProgramPages field set to 1
		txn := sdk_types.SignedTxnWithAD{
			SignedTxn: sdk_types.SignedTxn{
				Txn: sdk_types.Transaction{
					Type: "appl",
					Header: sdk_types.Header{
						Sender: test.AccountA,
					},
					ApplicationFields: sdk_types.ApplicationFields{
						ApplicationCallTxnFields: sdk_types.ApplicationCallTxnFields{
							ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
							ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
							ExtraProgramPages: 1,
						},
					},
				},
			},
		}
		txnRow := idb.TxnRow{
			Round:    uint64(test.Round),
			TxnBytes: msgpack.Encode(txn),
			AssetID:  assetID,
		}

		err := state.AddTransaction(&txnRow)
		require.NoError(t, err)

		block := test.MakeBlockForTxns(test.Round, &txn)
		blockImporter := importer.NewDBImporter(db)
		txnCount, err := blockImporter.ImportDecodedBlock(&block)
		require.NoError(t, err, "failed to import")
		require.Equal(t, 1, txnCount)
	}

	err = db.CommitRoundAccounting(state.RoundUpdates, test.Round, &types.BlockHeader{})
	require.NoError(t, err, "failed to commit")

	row := db.db.QueryRow("SELECT index, params FROM app WHERE creator = $1", test.AccountA[:])

	var index uint64
	var paramsStr []byte
	err = row.Scan(&index, &paramsStr)
	require.NoError(t, err)
	require.NotZero(t, index)

	var ap AppParams
	err = encoding.DecodeJSON(paramsStr, &ap)
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
	for row := range rows {
		require.NoError(t, row.Error)
		num++
		require.NotNil(t, row.Account.AppsTotalExtraPages, "we should have this field")
		require.Equal(t, uint64(1), *row.Account.AppsTotalExtraPages)
	}
	require.Equal(t, 1, num)
}

func assertKeytype(t *testing.T, db *IndexerDb, address sdk_types.Address, keytype *string) {
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Make an empty block so `GetAccounts()` does not fail.
	importTxns(t, db, test.Round)
	accountTxns(t, db, test.Round)
	assertKeytype(t, db, test.AccountA, nil)

	{
		txn, txnRow := test.MakePayTxnRowOrPanic(
			test.Round+1, 0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA,
			sdk_types.ZeroAddress, sdk_types.ZeroAddress)
		txn.Sig[0] = 3
		txnRow.TxnBytes = msgpack.Encode(txn)
		importTxns(t, db, test.Round+1, txn)
		accountTxns(t, db, test.Round+1, txnRow)
		keytype := "sig"
		assertKeytype(t, db, test.AccountA, &keytype)
	}

	{
		txn, txnRow := test.MakePayTxnRowOrPanic(
			test.Round+2, 0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA,
			sdk_types.ZeroAddress, sdk_types.ZeroAddress)
		txn.Msig.Subsigs = append(txn.Msig.Subsigs, sdk_types.MultisigSubsig{})
		txnRow.TxnBytes = msgpack.Encode(txn)
		importTxns(t, db, test.Round+2, txn)
		accountTxns(t, db, test.Round+2, txnRow)
		keytype := "msig"
		assertKeytype(t, db, test.AccountA, &keytype)
	}
}

// Test that asset amount >= 2^63 is handled correctly. Due to the specifics of
// postgres it might be a problem.
func TestLargeAssetAmount(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	assetid := uint64(1)
	txn, txnRow := test.MakeAssetConfigOrPanic(
		test.Round, 0, assetid, math.MaxUint64, 0, false, "mc", "mycoin", "", test.AccountA)
	importTxns(t, db, test.Round, txn)
	accountTxns(t, db, test.Round, txnRow)

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

// Test that initializing a new database sets the correct migration number.
func TestInitializationNewDatabase(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	state, err := db.getMigrationState()
	require.NoError(t, err)

	assert.Equal(t, len(migrations), state.NextMigration)
}

// Test that opening the database the second time (after initializing) is successful.
func TestOpenDbAgain(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	_, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	_, err = OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)
}
