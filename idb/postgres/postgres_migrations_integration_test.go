package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/encoding/json"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
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

type oldImportState struct {
	AccountRound *int64 `codec:"account_round"`
}

func TestMaxRoundAccountedMigrationAccountRound0(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	round := int64(0)
	old := oldImportState{
		AccountRound: &round,
	}
	err = db.setMetastate(nil, stateMetastateKey, string(json.Encode(old)))
	require.NoError(t, err)

	migrationState := MigrationState{NextMigration: 4}
	err = MaxRoundAccountedMigration(db, &migrationState)
	require.NoError(t, err)

	importstate, err := db.getImportState(nil)
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
	db, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	round := int64(2)
	old := oldImportState{
		AccountRound: &round,
	}
	err = db.setMetastate(nil, stateMetastateKey, string(json.Encode(old)))
	require.NoError(t, err)

	migrationState := MigrationState{NextMigration: 4}
	err = MaxRoundAccountedMigration(db, &migrationState)
	require.NoError(t, err)

	importstate, err := db.getImportState(nil)
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
	db, _, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
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

func TestDeleteReverseAppDeltasMigration(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	require.NoError(t, err)

	_, err = db.db.Exec(
		`INSERT INTO txn (round, intra, typeenum, asset, txid, txnbytes, txn, extra) `+
			`VALUES(0, $1, 0, 0, '{}', '{}', '{}', '{"aca": 31}')`,
		1)
	require.NoError(t, err)

	_, err = db.db.Exec(
		`INSERT INTO txn (round, intra, typeenum, asset, txid, txnbytes, txn, extra) `+
			`VALUES(0, $1, 0, 0, '{}', '{}', '{}', '{"aca": 32, "agr": 42}')`,
		2)
	require.NoError(t, err)

	_, err = db.db.Exec(
		`INSERT INTO txn (round, intra, typeenum, asset, txid, txnbytes, txn, extra) `+
			`VALUES(0, $1, 0, 0, '{}', '{}', '{}', '{"aca": 33, "alr": 43}')`,
		3)
	require.NoError(t, err)

	migrationState := MigrationState{NextMigration: 4}
	err = DeleteReverseAppDeltasMigration(db, &migrationState)
	require.NoError(t, err)

	query := "SELECT extra FROM txn WHERE intra = $1"

	{
		row := db.db.QueryRow(query, 1)
		var extra string
		err = row.Scan(&extra)
		require.NoError(t, err)
		assert.Equal(t, `{"aca": 31}`, extra)
	}

	{
		row := db.db.QueryRow(query, 2)
		var extra string
		err = row.Scan(&extra)
		require.NoError(t, err)
		assert.Equal(t, `{"aca": 32}`, extra)
	}

	{
		row := db.db.QueryRow(query, 3)
		var extra string
		err = row.Scan(&extra)
		require.NoError(t, err)
		assert.Equal(t, `{"aca": 33}`, extra)
	}

	// Check the next migration number.
	assert.Equal(t, 5, migrationState.NextMigration)
	newNum := nextMigrationNum(t, db)
	assert.Equal(t, 5, newNum)
}
