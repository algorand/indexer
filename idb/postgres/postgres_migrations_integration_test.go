package postgres

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
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

type oldImportState struct {
	AccountRound *int64 `codec:"account_round"`
}

func TestMaxRoundAccountedMigrationAccountRound0(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
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
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
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

// TestEmbeddedNullString make sure we're able to import cheeky assets.
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
	round := test.Round
	var creator types.Address

	for _, testcase := range tests {
		testcase := testcase
		name := testcase.AssetName
		unit := testcase.AssetUnit
		url := testcase.AssetURL
		creator[0] = byte(assetID)

		t.Run(testcase.Name, func(t *testing.T) {
			t.Parallel()
			db, shutdownFunc := setupIdb(t, test.MakeGenesis())
			defer shutdownFunc()

			txn, txnRow := test.MakeAssetConfigOrPanic(
				round, 0, assetID, math.MaxUint64, 0, false, unit, name, url, test.AccountA)

			// Test 1: import/accounting should work.
			importTxns(t, db, round, txn)
			accountTxns(t, db, round, txnRow)

			// Test 2: asset results properly serialized
			assets, _ := db.Assets(context.Background(), idb.AssetsQuery{AssetID: assetID})
			num := 0
			for asset := range assets {
				require.NoError(t, asset.Error)
				require.Equal(t, testcase.ExpectedAssetName, asset.Params.AssetName)
				require.Equal(t, testcase.ExpectedAssetUnit, asset.Params.UnitName)
				require.Equal(t, testcase.ExpectedAssetURL, asset.Params.URL)
				require.Equal(t, []byte(name), asset.Params.AssetNameBytes)
				require.Equal(t, []byte(unit), asset.Params.UnitNameBytes)
				require.Equal(t, []byte(url), asset.Params.URLBytes)
				num++
			}
			require.Equal(t, 1, num)

			// Test 3: transaction results properly serialized
			transactions, _ := db.Transactions(context.Background(), idb.TransactionFilter{})
			num = 0
			for tx := range transactions {
				require.NoError(t, tx.Error)
				// Note: These are created from the TxnBytes, so they have the exact name with embedded null.
				var txn sdk_types.SignedTxn
				require.NoError(t, msgpack.Decode(tx.TxnBytes, &txn))
				require.Equal(t, name, txn.Txn.AssetParams.AssetName)
				require.Equal(t, unit, txn.Txn.AssetParams.UnitName)
				require.Equal(t, url, txn.Txn.AssetParams.URL)
				num++
			}
			require.Equal(t, 1, num)

			requireNilOrEqual := func(t *testing.T, expected string, actual *string) {
				if expected == "" {
					require.Nil(t, actual)
				} else {
					require.NotNil(t, actual)
					require.Equal(t, expected, *actual)
				}
			}
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
