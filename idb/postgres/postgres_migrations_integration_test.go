package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/types"
	"github.com/algorand/indexer/util/test"
)

func nextMigrationNum(t *testing.T, db *IndexerDb) int {
	j, err := db.getMetastate(nil, migrationMetastateKey)
	assert.NoError(t, err)

	assert.True(t, len(j) > 0)

	var state MigrationState
	err = encoding.DecodeJSON([]byte(j), &state)
	assert.NoError(t, err)

	return state.NextMigration
}

func TestFixFreezeLookupMigration(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	var sender types.Address
	var faddr types.Address

	sender[0] = 0x01
	faddr[0] = 0x02

	///////////
	// Given // A block containing an asset freeze txn has been imported.
	///////////
	freeze, _ := test.MakeAssetFreezeOrPanic(test.Round, 1234, true, sender, faddr)
	importTxns(t, db, test.Round, freeze)

	//////////
	// When // We truncate the txn_participation table and run our migration
	//////////
	db.db.Exec("TRUNCATE txn_participation")
	FixFreezeLookupMigration(db, &MigrationState{NextMigration: 12})

	//////////
	// Then // The sender is still deleted, but the freeze addr should be back.
	//////////
	senderCount := queryInt(db.db, "SELECT COUNT(*) FROM txn_participation WHERE addr = $1", sender[:])
	faddrCount := queryInt(db.db, "SELECT COUNT(*) FROM txn_participation WHERE addr = $1", faddr[:])
	assert.Equal(t, 0, senderCount)
	assert.Equal(t, 1, faddrCount)
}

// Test that ClearAccountDataMigration() clears account data for closed accounts.
func TestClearAccountDataMigrationClosedAccounts(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Rekey account A.
	{
		stxn, txnRow := test.MakePayTxnRowOrPanic(
			test.Round, 0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, sdk_types.ZeroAddress,
			test.AccountB)

		importTxns(t, db, test.Round, stxn)
		accountTxns(t, db, test.Round, txnRow)
	}

	// Close account A without deleting account data.
	{
		query := "UPDATE account SET deleted = true, closed_at = $1 WHERE addr = $2"
		_, err := db.db.Exec(query, test.Round+1, test.AccountA[:])
		assert.NoError(t, err)
	}

	// Run migration.
	err := ClearAccountDataMigration(db, &MigrationState{})
	assert.NoError(t, err)

	// Check that account A has no account data.
	opts := idb.AccountQueryOptions{
		EqualToAddress: test.AccountA[:],
		IncludeDeleted: true,
	}
	ch, _ := db.GetAccounts(context.Background(), opts)
	accountRow, ok := <-ch
	assert.True(t, ok)
	assert.NoError(t, accountRow.Error)
	account := accountRow.Account
	assert.Nil(t, account.AuthAddr)
}

// Test that ClearAccountDataMigration() clears account data that was set before account was closed.
func TestClearAccountDataMigrationClearsReopenedAccounts(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Create account A.
	{
		stxn, txnRow := test.MakePayTxnRowOrPanic(
			test.Round, 0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, sdk_types.ZeroAddress,
			sdk_types.ZeroAddress)

		importTxns(t, db, test.Round, stxn)
		accountTxns(t, db, test.Round, txnRow)
	}

	// Make rekey and keyreg transactions for account A.
	{
		stxn0, txnRow0 := test.MakePayTxnRowOrPanic(
			test.Round+1, 0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, sdk_types.ZeroAddress,
			test.AccountB)
		stxn1, txnRow1 := test.MakeSimpleKeyregOnlineTxn(test.Round+1, test.AccountA)

		importTxns(t, db, test.Round+1, stxn0, stxn1)
		accountTxns(t, db, test.Round+1, txnRow0, txnRow1)
	}

	// Check that account A is online, has auth-addr and participation info.
	opts := idb.AccountQueryOptions{
		EqualToAddress: test.AccountA[:],
	}
	{
		ch, _ := db.GetAccounts(context.Background(), opts)
		accountRow, ok := <-ch
		assert.True(t, ok)
		assert.NoError(t, accountRow.Error)
		account := accountRow.Account
		assert.NotNil(t, account.AuthAddr)
		assert.NotNil(t, account.Participation)
		assert.Equal(t, "Online", account.Status)
	}

	// Simulate closing and reopening of account A.
	{
		query := "UPDATE account SET deleted = false, closed_at = $1 WHERE addr = $2"
		_, err := db.db.Exec(query, test.Round+2, test.AccountA[:])
		assert.NoError(t, err)
	}

	// Run migration.
	err := ClearAccountDataMigration(db, &MigrationState{})
	assert.NoError(t, err)

	// Check that account A is offline and has no account data.
	{
		ch, _ := db.GetAccounts(context.Background(), opts)
		accountRow, ok := <-ch
		assert.True(t, ok)
		assert.NoError(t, accountRow.Error)
		account := accountRow.Account
		assert.Nil(t, account.AuthAddr)
		assert.Nil(t, account.Participation)
		assert.Equal(t, "Offline", account.Status)
	}
}

// Test that ClearAccountDataMigration() does not clear account data because is was updated after
// account was closed.
func TestClearAccountDataMigrationDoesNotClear(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Create account A.
	{
		stxn, txnRow := test.MakePayTxnRowOrPanic(
			test.Round, 0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, sdk_types.ZeroAddress,
			sdk_types.ZeroAddress)

		importTxns(t, db, test.Round, stxn)
		accountTxns(t, db, test.Round, txnRow)
	}

	// Simulate closing and reopening of account A.
	{
		query := "UPDATE account SET deleted = false, closed_at = $1 WHERE addr = $2"
		_, err := db.db.Exec(query, test.Round+1, test.AccountA[:])
		assert.NoError(t, err)
	}

	// Account A rekey and keyreg.
	{
		stxn0, txnRow0 := test.MakePayTxnRowOrPanic(
			test.Round+2, 0, 0, 0, 0, 0, 0, test.AccountA, test.AccountA, sdk_types.ZeroAddress,
			test.AccountB)
		stxn1, txnRow1 := test.MakeSimpleKeyregOnlineTxn(test.Round+2, test.AccountA)

		importTxns(t, db, test.Round+2, stxn0, stxn1)
		accountTxns(t, db, test.Round+2, txnRow0, txnRow1)
	}

	// A safety check.
	{
		accounts, err := getAccounts(db.db)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(accounts))
	}

	// Run migration.
	err := ClearAccountDataMigration(db, &MigrationState{})
	assert.NoError(t, err)

	// Check that account A is online and has auth addr and keyreg data.
	opts := idb.AccountQueryOptions{
		EqualToAddress: test.AccountA[:],
	}
	ch, _ := db.GetAccounts(context.Background(), opts)
	accountRow, ok := <-ch
	assert.True(t, ok)
	assert.NoError(t, accountRow.Error)
	account := accountRow.Account
	assert.NotNil(t, account.AuthAddr)
	assert.NotNil(t, account.Participation)
	assert.Equal(t, "Online", account.Status)
}

// Test that ClearAccountDataMigration() increments the next migration number.
func TestClearAccountDataMigrationIncMigrationNum(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Run migration.
	state := MigrationState{NextMigration: 13}
	err := ClearAccountDataMigration(db, &state)
	assert.NoError(t, err)

	assert.Equal(t, 14, state.NextMigration)
	newNum := nextMigrationNum(t, db)
	assert.Equal(t, 14, newNum)
}

func TestMakeDeletedNotNullMigration(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	// Make deleted columns nullable.
	queries := []string{
		"ALTER TABLE account ALTER COLUMN deleted DROP NOT NULL",
		"ALTER TABLE account_asset ALTER COLUMN deleted DROP NOT NULL",
		"ALTER TABLE asset ALTER COLUMN deleted DROP NOT NULL",
		"ALTER TABLE app ALTER COLUMN deleted DROP NOT NULL",
		"ALTER TABLE account_app ALTER COLUMN deleted DROP NOT NULL",
	}
	for _, query := range queries {
		_, err := db.db.Exec(query)
		require.NoError(t, err)
	}

	// Insert data.
	var address sdk_types.Address
	queries = []string{
		"INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted) " +
			"VALUES ($1, 0, 0, 0, NULL)",
		"INSERT INTO account_asset (addr, assetid, amount, frozen, deleted) " +
			"VALUES ($1, 0, 0, false, NULL)",
		"INSERT INTO asset (index, creator_addr, params, deleted) " +
			"VALUES (0, $1, '{}', NULL)",
		"INSERT INTO app (index, creator, params, deleted) " +
			"VALUES (0, $1, '{}', NULL)",
		"INSERT INTO account_app (addr, app, deleted) " +
			"VALUES ($1, 0, NULL)",
	}
	for _, query := range queries {
		_, err := db.db.Exec(query, address[:])
		require.NoError(t, err)
	}

	// Run migration.
	state := MigrationState{NextMigration: 98}
	err := MakeDeletedNotNullMigration(db, &state)
	require.NoError(t, err)

	// Check that next migration number is incremented.
	assert.Equal(t, 99, state.NextMigration)
	newNum := nextMigrationNum(t, db)
	assert.Equal(t, 99, newNum)

	// Check that deleted columns are set to false.
	queries = []string{
		"SELECT deleted FROM account WHERE addr = $1",
		"SELECT deleted FROM account_asset WHERE addr = $1",
		"SELECT deleted FROM asset WHERE creator_addr = $1",
		"SELECT deleted FROM app WHERE creator = $1",
		"SELECT deleted FROM account_app WHERE addr = $1",
	}
	for _, query := range queries {
		row := db.db.QueryRow(query, address[:])

		var deleted *bool
		err := row.Scan(&deleted)
		require.NoError(t, err)

		require.NotNil(t, deleted)
		assert.Equal(t, false, *deleted)
	}
}

func TestMaxRoundAccountedMigrationAccountRound0(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	round := int64(0)
	importstate := importState{
		AccountRound: &round,
	}
	err = db.setImportState(nil, importstate)
	require.NoError(t, err)

	migrationState := MigrationState{NextMigration: 4}
	err = MaxRoundAccountedMigration(db, &migrationState)
	require.NoError(t, err)

	importstate, err = db.getImportState(nil)
	require.NoError(t, err)

	nextRound := uint64(0)
	importstateExpected := importState{
		NextRoundToAccount: &nextRound,
	}
	assert.Equal(t, importstateExpected, importstate)

	// Check the next migration number.
	assert.Equal(t, 5, migrationState.NextMigration)
	newNum := nextMigrationNum(t, db)
	assert.Equal(t, 5, newNum)
}

func TestMaxRoundAccountedMigrationAccountRoundPositive(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	round := int64(2)
	importstate := importState{
		AccountRound: &round,
	}
	err = db.setImportState(nil, importstate)
	require.NoError(t, err)

	migrationState := MigrationState{NextMigration: 4}
	err = MaxRoundAccountedMigration(db, &migrationState)
	require.NoError(t, err)

	importstate, err = db.getImportState(nil)
	require.NoError(t, err)

	nextRound := uint64(3)
	importstateExpected := importState{
		NextRoundToAccount: &nextRound,
	}
	assert.Equal(t, importstateExpected, importstate)

	// Check the next migration number.
	assert.Equal(t, 5, migrationState.NextMigration)
	newNum := nextMigrationNum(t, db)
	assert.Equal(t, 5, newNum)
}

func TestMaxRoundAccountedMigrationUninitialized(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	migrationState := MigrationState{NextMigration: 4}
	err = MaxRoundAccountedMigration(db, &migrationState)
	require.NoError(t, err)

	_, err = db.getImportState(nil)
	assert.Equal(t, idb.ErrorNotInitialized, err)

	// Check the next migration number.
	assert.Equal(t, 5, migrationState.NextMigration)
	newNum := nextMigrationNum(t, db)
	assert.Equal(t, 5, newNum)
}
