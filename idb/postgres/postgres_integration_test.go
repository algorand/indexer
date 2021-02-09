package postgres

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/postgres"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/accounting"
	itypes "github.com/algorand/indexer/types"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util/test"
)

// getAccounting initializes the ac counting state for testing.
func getAccounting(round uint64, cache map[uint64]bool) *accounting.State {
	accountingState := accounting.New(cache)
	accountingState.InitRoundParts(round, test.FeeAddr, test.RewardAddr, 0)
	return accountingState
}

// setupPostgres starts a gnomock postgres DB then returns the connection string and a shutdown function.
func setupPostgres(t *testing.T) (*sql.DB, string, func()) {
	p := postgres.Preset(
		postgres.WithVersion("12.5"),
		postgres.WithUser("gnomock", "gnomick"),
		postgres.WithDatabase("mydb"),
		// 'IndexerDbByName' does this if necessary, so not needed here.
		//postgres.WithQueriesFile("setup_postgres.sql"),
	)
	container, err := gnomock.Start(p)
	assert.NoError(t, err, "Error starting gnomock")

	shutdownFunc := func() {
		err = gnomock.Stop(container)
		assert.NoError(t, err, "Error stoping gnomock")
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s  dbname=%s sslmode=disable",
		container.Host, container.DefaultPort(),
		"gnomock", "gnomick", "mydb",
	)

	db, err := sql.Open("postgres", connStr)
	assert.NoError(t, err, "Error opening pg connection")
	return db, connStr, shutdownFunc
}

// TestMaxRoundOnUninitializedDB makes sure we return 0 when getting the max round on a new DB.
func TestMaxRoundOnUninitializedDB(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	///////////
	// Given // A database that has not yet imported the genesis accounts.
	///////////
	db, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)

	//////////
	// When // We request the max round.
	//////////
	round, err := db.GetMaxRound()

	//////////
	// Then // There should be no error and we return that there are zero rounds.
	//////////
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), round)
}

func assertAccountAsset(t *testing.T, db *sql.DB, addr types.Address, assetid uint64, frozen bool, amount uint64) {
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
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	pdb, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)

	assetid := uint64(2222)
	amt := uint64(10000)
	total := uint64(1000000)

	///////////
	// Given // A round scenario requiring subround accounting: AccountA is funded, closed, opts back, and funded again.
	///////////
	_, createAsset := test.MakeAssetConfigOrPanic(test.Round, assetid, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com", test.AccountD)
	_, fundMain := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountD, test.AccountA, types.ZeroAddress)
	_, closeMain := test.MakeAssetTxnOrPanic(test.Round, assetid, 1000, test.AccountA, test.AccountB, test.AccountC)
	_, optinMain := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountA, test.AccountA, types.ZeroAddress)
	_, payMain := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountD, test.AccountA, types.ZeroAddress)

	cache, err := pdb.GetDefaultFrozen()
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
	err = pdb.CommitRoundAccounting(state.RoundUpdates, test.Round, &itypes.Block{})
	assert.NoError(t, err, "failed to commit")

	//////////
	// Then // Accounts A, B, C and D have the correct balances.
	//////////
	// A has the final payment after being closed out
	assertAccountAsset(t, db, test.AccountA, assetid, false, amt)
	// B has the closing transfer amount
	assertAccountAsset(t, db, test.AccountB, assetid, false, 1000)
	// C has the close-to remainder
	assertAccountAsset(t, db, test.AccountC, assetid, false, 9000)
	// D has the total minus both payments to A
	assertAccountAsset(t, db, test.AccountD, assetid, false, total - 2 * amt)
}

// TestDefaultFrozenAndCache checks that values are added to the default frozen cache, and that the cache is used when
// accounts optin to an asset.
func TestDefaultFrozenAndCache(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	pdb, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)

	assetid := uint64(2222)
	total := uint64(1000000)

	///////////
	// Given // A new asset with default-frozen = true, and AccountB opting into it.
	///////////
	_, createAssetFrozen := test.MakeAssetConfigOrPanic(test.Round, assetid, total, uint64(6), true, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)
	_, createAssetNotFrozen := test.MakeAssetConfigOrPanic(test.Round, assetid + 1, total, uint64(6), false, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)
	_, optinB1 := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountB, test.AccountB, types.ZeroAddress)
	_, optinB2 := test.MakeAssetTxnOrPanic(test.Round, assetid + 1, 0, test.AccountB, test.AccountB, types.ZeroAddress)


	cache, err := pdb.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(createAssetFrozen)
	state.AddTransaction(createAssetNotFrozen)
	state.AddTransaction(optinB1)
	state.AddTransaction(optinB2)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = pdb.CommitRoundAccounting(state.RoundUpdates, test.Round, &itypes.Block{})
	assert.NoError(t, err, "failed to commit")

	//////////
	// Then // Make sure the accounts have the correct default-frozen after create/optin
	//////////
	// default-frozen = true
	assertAccountAsset(t, db, test.AccountA, assetid, true, total)
	assertAccountAsset(t, db, test.AccountB, assetid, true, 0)

	// default-frozen = false
	assertAccountAsset(t, db, test.AccountA, assetid + 1, false, total)
	assertAccountAsset(t, db, test.AccountB, assetid + 1, false, 0)
}

// TestInitializeFrozenCache checks that the frozen cache is properly initialized on startup.
func TestInitializeFrozenCache(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	// Initialize DB by creating one of these things.
	_, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)

	// Add some assets
	db.Exec(`INSERT INTO asset (index, creator_addr, params) values ($1, $2, $3)`, 1, test.AccountA[:], `{"df":true}`)
	db.Exec(`INSERT INTO asset (index, creator_addr, params) values ($1, $2, $3)`, 2, test.AccountA[:], `{"df":false}`)
	db.Exec(`INSERT INTO asset (index, creator_addr, params) values ($1, $2, $3)`, 3, test.AccountA[:], `{}`)

	pdb, err := OpenPostgres(connStr, nil, nil)
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
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	pdb, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)

	assetid := uint64(2222)
	total := uint64(1000000)

	tests := []struct{
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
		_, createAssetFrozen := test.MakeAssetConfigOrPanic(round, aid, total, uint64(6), testcase.frozen, "icicles", "frozen coin", "http://antarctica.com", test.AccountA)
		_, optinB := test.MakeAssetTxnOrPanic(round, aid, 0, test.AccountB, test.AccountB, types.ZeroAddress)
		_, unfreezeB := test.MakeAssetFreezeOrPanic(round, aid, !testcase.frozen, test.AccountB)
		_, optoutB := test.MakeAssetTxnOrPanic(round, aid, 0, test.AccountB, test.AccountC, test.AccountD)

		cache, err := pdb.GetDefaultFrozen()
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
		err = pdb.CommitRoundAccounting(state.RoundUpdates, round, &itypes.Block{})
		assert.NoError(t, err, "failed to commit")

		//////////
		// Then // AccountB should have its frozen state set back to the default value
		//////////
		assertAccountAsset(t, db, test.AccountB, aid, testcase.frozen, 0)
	}
}

// TestMultipleAssetOptins make sure no-op transactions don't reset the default frozen value.
func TestNoopOptins(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	///////////
	// Given // An asset with default-frozen = true, AccountB opt's in, is unfrozen, then has a no-op opt-in
	///////////

	assetid := uint64(2222)
	// create asst
	//db.Exec(`INSERT INTO asset (index, creator_addr, params) values ($1, $2, $3)`, assetid, test.AccountA[:], `{"df":true}`)

	pdb, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)

	_, createAsset := test.MakeAssetConfigOrPanic(test.Round, assetid, uint64(1000000), uint64(6), true, "icicles", "frozen coin", "http://antarctica.com", test.AccountD)
	_, optinB := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountB, test.AccountB, types.ZeroAddress)
	_, unfreezeB := test.MakeAssetFreezeOrPanic(test.Round, assetid, false, test.AccountB)

	cache, err := pdb.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(createAsset)
	state.AddTransaction(optinB)
	state.AddTransaction(unfreezeB)
	state.AddTransaction(optinB)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	err = pdb.CommitRoundAccounting(state.RoundUpdates, test.Round, &itypes.Block{})
	assert.NoError(t, err, "failed to commit")

	//////////
	// Then // AccountB should have its frozen state set back to the default value
	//////////
	// TODO: This isn't working yet
	assertAccountAsset(t, db, test.AccountB, assetid, false, 0)
}




// TestMultipleWriters tests that accounting cannot be double committed.
func TestMultipleWriters(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	pdb, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)

	amt := uint64(10000)

	///////////
	// Given // Send amt to AccountA
	///////////
	_, payAccountA := test.MakePayTxnRowOrPanic(test.Round, 1000, amt, 0, 0, 0, 0, test.AccountD, test.AccountA, types.ZeroAddress)

	cache, err := pdb.GetDefaultFrozen()
	assert.NoError(t, err)
	state := getAccounting(test.Round, cache)
	state.AddTransaction(payAccountA)

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
			<- start
			errors <- pdb.CommitRoundAccounting(state.RoundUpdates, test.Round, &itypes.Block{})
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

	// AccountA should contain the final payment.
	var balance uint64
	row := db.QueryRow(`SELECT microalgos FROM account WHERE account.addr = $1`, test.AccountA[:])
	err = row.Scan(&balance)
	assert.NoError(t, err, "checking balance")
	assert.Equal(t, amt, balance)
}
