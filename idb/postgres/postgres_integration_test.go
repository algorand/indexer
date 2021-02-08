package postgres

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/lib/pq"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/postgres"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util/test"
)

// getAccounting initializes the ac counting state for testing.
func getAccounting() *accounting.State {
	accountingState := accounting.New()
	accountingState.InitRoundParts(test.Round, test.FeeAddr, test.RewardAddr, 0)
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

// TestAssetCloseReopenTransfer tests a scenario that requires asset subround accounting
func TestAssetCloseReopenTransfer(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	pdb, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)

	assetid := uint64(2222)
	amt := uint64(10000)

	///////////
	// Given // A round scenario requiring subround accounting: AccountA is funded, closed, opts back, and funded again.
	///////////
	_, fundMain := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountD, test.AccountA, types.ZeroAddress)
	_, closeMain := test.MakeAssetTxnOrPanic(test.Round, assetid, 1000, test.AccountA, test.AccountB, test.AccountC)
	_, optinMain := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountA, test.AccountA, types.ZeroAddress)
	_, payMain := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountD, test.AccountA, types.ZeroAddress)

	state := getAccounting()
	state.AddTransaction(fundMain)
	state.AddTransaction(closeMain)
	state.AddTransaction(optinMain)
	state.AddTransaction(payMain)

	//////////
	// When // We commit the round accounting to the database.
	//////////
	pdb.CommitRoundAccounting(state.RoundUpdates, test.Round, 0)

	//////////
	// Then // Accounts A, B, C and D have the correct balances.
	//////////
	var resultBalance int
	var row *sql.Row

	// AccountA should contain the final payment.
	row = db.QueryRow(`SELECT amount FROM account_asset WHERE account_asset.assetid = $1 AND account_asset.addr = $2`, assetid, test.AccountA[:])
	err = row.Scan(&resultBalance)
	assert.NoError(t, err, "checking balance")
	assert.Equal(t, int(amt), resultBalance)

	// AccountB should have the asset close amount of 1000
	row = db.QueryRow(`SELECT amount FROM account_asset WHERE account_asset.assetid = $1 AND account_asset.addr = $2`, assetid, test.AccountB[:])
	err = row.Scan(&resultBalance)
	assert.NoError(t, err, "checking balance")
	assert.Equal(t, 1000, resultBalance)

	// AccountC should have the remaining 9000
	row = db.QueryRow(`SELECT amount FROM account_asset WHERE account_asset.assetid = $1 AND account_asset.addr = $2`, assetid, test.AccountC[:])
	err = row.Scan(&resultBalance)
	assert.NoError(t, err, "checking balance")
	assert.Equal(t, 9000, resultBalance)

	// The funding account, AccountD, should have -20000, it sent funds without ever being funded.
	row = db.QueryRow(`SELECT amount FROM account_asset WHERE account_asset.assetid = $1 AND account_asset.addr = $2`, assetid, test.AccountD[:])
	err = row.Scan(&resultBalance)
	assert.NoError(t, err, "checking balance")
	assert.Equal(t, int(amt) * -2, resultBalance)
}

