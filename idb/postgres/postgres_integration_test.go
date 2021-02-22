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
	itypes "github.com/algorand/indexer/types"
	"github.com/algorand/indexer/util/test"
)

// getAccounting initializes the ac counting state for testing.
func getAccounting(round uint64, cache map[uint64]bool) *accounting.State {
	accountingState := accounting.New()
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
	roundA, err := db.GetMaxRoundAccounted()
	assert.NoError(t, err)
	roundL, err := db.GetMaxRoundLoaded()
	assert.NoError(t, err)

	//////////
	// Then // There should be no error and we return that there are zero rounds.
	//////////
	assert.Equal(t, uint64(0), roundA)
	assert.Equal(t, uint64(0), roundL)
}

// TestMaxRoundEmptyMetastate makes sure we return 0 when the metastate is empty.
func TestMaxRoundEmptyMetastate(t *testing.T) {
	pg, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	///////////
	// Given // The database has the metastate set but the account_round is missing.
	///////////
	db, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)
	pg.Exec(`INSERT INTO metastate (k, v) values ('state', '{}')`)

	//////////
	// When // We request the max round.
	//////////
	round, err := db.GetMaxRoundAccounted()
	assert.NoError(t, err)

	//////////
	// Then // There should be no error and we return that there are zero rounds.
	//////////
	assert.Equal(t, uint64(0), round)
}

// TestMaxRound the happy path.
func TestMaxRound(t *testing.T) {
	db, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	///////////
	// Given // The database has the metastate set normally.
	///////////
	pdb, err := idb.IndexerDbByName("postgres", connStr, nil, nil)
	assert.NoError(t, err)
	db.Exec(`INSERT INTO metastate (k, v) values ($1, $2)`, "state", "{\"account_round\":123454321}")
	db.Exec(`INSERT INTO block_header (round, realtime, rewardslevel, header) VALUES ($1, NOW(), 0, '{}') ON CONFLICT DO NOTHING`, 543212345)

	//////////
	// When // We request the max round.
	//////////
	roundA, err := pdb.GetMaxRoundAccounted()
	assert.NoError(t, err)
	roundL, err := pdb.GetMaxRoundLoaded()
	assert.NoError(t, err)

	//////////
	// Then // There should be no error and we return that there are zero rounds.
	//////////
	assert.Equal(t, uint64(123454321), roundA)
	assert.Equal(t, uint64(543212345), roundL)
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

	state := getAccounting(test.Round, nil)
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

