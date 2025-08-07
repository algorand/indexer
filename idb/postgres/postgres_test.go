package postgres

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/types"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

func Test_txnFilterOptimization(t *testing.T) {
	tests := []struct {
		name     string
		arg      idb.TransactionFilter
		rootOnly bool
	}{
		{
			name:     "basic",
			arg:      idb.TransactionFilter{},
			rootOnly: true,
		},
		{
			name:     "rounds",
			arg:      idb.TransactionFilter{MinRound: 100, MaxRound: 101, Limit: 100},
			rootOnly: true,
		},
		{
			name:     "date",
			arg:      idb.TransactionFilter{AfterTime: time.Unix(100000, 100), Limit: 100},
			rootOnly: true,
		},
		{
			name:     "token",
			arg:      idb.TransactionFilter{NextToken: "test", Limit: 100},
			rootOnly: true,
		},
		{
			name:     "address",
			arg:      idb.TransactionFilter{Address: []byte{0x10, 0x11, 0x12}, Limit: 100},
			rootOnly: false,
		},
		{
			name:     "type",
			arg:      idb.TransactionFilter{TypeEnum: idb.TypeEnumPay, Limit: 100},
			rootOnly: false,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s(%t)", tt.name, tt.rootOnly), func(t *testing.T) {
			optimized := txnFilterOptimization(tt.arg)
			assert.Equal(t, tt.rootOnly, optimized.SkipInnerTransactions)
		})
	}
}

func Test_UnknownProtocol(t *testing.T) {
	db := IndexerDb{}
	protocol := "zzzzzzz"
	err := db.AddBlock(&types.ValidatedBlock{
		Block: sdk.Block{
			BlockHeader: sdk.BlockHeader{
				UpgradeState: sdk.UpgradeState{
					CurrentProtocol: protocol,
				},
			},
		},
	})
	require.ErrorContains(t, err, protocol)
	require.ErrorContains(t, err, "you need to upgrade")
}

func Test_buildTransactionQueryTime(t *testing.T) {
	usEastTZ, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	usWestTZ, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	randomDateUTC := time.Date(1000, time.December, 25, 1, 2, 3, 4, time.UTC)
	tests := []struct {
		name      string
		arg       idb.TransactionFilter
		whereArgs []interface{}
	}{
		{
			"BeforeTime UTC to UTC",
			idb.TransactionFilter{
				BeforeTime: randomDateUTC,
			},
			[]interface{}{randomDateUTC},
		},
		{
			"AfterTime UTC to UTC",
			idb.TransactionFilter{
				AfterTime: randomDateUTC,
			},
			[]interface{}{randomDateUTC},
		},
		{
			"BeforeTime AfterTime Conversion",
			idb.TransactionFilter{
				BeforeTime: time.Date(1000, time.December, 25, 1, 2, 3, 4, usEastTZ),
				AfterTime:  time.Date(1000, time.December, 25, 1, 2, 3, 4, usWestTZ),
			},
			[]interface{}{
				time.Date(1000, time.December, 25, 1, 2, 3, 4, usEastTZ).UTC(),
				time.Date(1000, time.December, 25, 1, 2, 3, 4, usWestTZ).UTC(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, whereArgs, _ := buildTransactionQuery(tt.arg)
			require.Equal(t, whereArgs, tt.whereArgs)
		})
	}
}

func Test_buildTransactionQueryApplicationLogs(t *testing.T) {
	tests := []struct {
		name                   string
		requireApplicationLogs bool
		expectedInQuery        bool
	}{
		{
			name:                   "RequireApplicationLogs true",
			requireApplicationLogs: true,
			expectedInQuery:        true,
		},
		{
			name:                   "RequireApplicationLogs false",
			requireApplicationLogs: false,
			expectedInQuery:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := idb.TransactionFilter{
				RequireApplicationLogs: tt.requireApplicationLogs,
				ApplicationID:          uint64Ptr(123),
				Limit:                  10,
			}

			query, _, err := buildTransactionQuery(filter)
			require.NoError(t, err)

			if tt.expectedInQuery {
				assert.Contains(t, query, "t.txn -> 'dt' -> 'lg' IS NOT NULL", "Query should contain application logs filter")
			} else {
				assert.NotContains(t, query, "t.txn -> 'dt' -> 'lg' IS NOT NULL", "Query should not contain application logs filter")
			}
		})
	}
}

// Test_buildTransactionQueryZeroValues tests the SQL generation for zero-value filters.
// This test verifies the custom behavior for application-id=0 and asset-id=0 filtering.
func Test_buildTransactionQueryZeroValues(t *testing.T) {
	tests := []struct {
		name           string
		filter         idb.TransactionFilter
		expectedSQL    []string // SQL fragments that should be present
		notExpectedSQL []string // SQL fragments that should NOT be present
		expectedArgs   []interface{}
		description    string
	}{
		{
			name: "ApplicationID zero - query original JSON field for app creation",
			filter: idb.TransactionFilter{
				ApplicationID: uint64Ptr(0),
				Limit:         10,
			},
			expectedSQL:    []string{"t.typeenum = ", "t.txn -> 'txn' -> 'apid'", "IS NULL"},
			notExpectedSQL: []string{},
			expectedArgs:   []interface{}{int(idb.TypeEnumApplication), uint64(0)},
			description:    "should query original JSON field for application creation transactions",
		},
		{
			name: "ApplicationID non-zero - should query t.asset",
			filter: idb.TransactionFilter{
				ApplicationID: uint64Ptr(123),
				Limit:         10,
			},
			expectedSQL:    []string{"t.asset = "},
			notExpectedSQL: []string{"t.txn -> 'txn' -> 'apid'"},
			expectedArgs:   []interface{}{uint64(123)},
			description:    "Non-zero ApplicationID should use t.asset column",
		},
		{
			name: "AssetID zero - should work for both transfers and creation",
			filter: idb.TransactionFilter{
				AssetID: uint64Ptr(0),
				Limit:   10,
			},
			expectedSQL:    []string{"t.asset = ", "t.typeenum = ", "t.txn -> 'txn' -> 'caid'", "IS NULL"},
			notExpectedSQL: []string{},
			expectedArgs:   []interface{}{uint64(0), int(idb.TypeEnumAssetConfig), uint64(0)},
			description:    "AssetID=0 should work for both Algo transfers (t.asset=0) and asset creation (original JSON field)",
		},
		{
			name: "AssetID non-zero - should query t.asset",
			filter: idb.TransactionFilter{
				AssetID: uint64Ptr(456),
				Limit:   10,
			},
			expectedSQL:    []string{"t.asset = "},
			notExpectedSQL: []string{},
			expectedArgs:   []interface{}{uint64(456)},
			description:    "Non-zero AssetID should use t.asset column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, whereArgs, err := buildTransactionQuery(tt.filter)
			require.NoError(t, err, "buildTransactionQuery should not error: %s", tt.description)

			// Check that expected SQL fragments are present
			for _, expectedFragment := range tt.expectedSQL {
				assert.Contains(t, query, expectedFragment,
					"Query should contain '%s': %s", expectedFragment, tt.description)
			}

			// Check that unexpected SQL fragments are NOT present
			for _, notExpectedFragment := range tt.notExpectedSQL {
				assert.NotContains(t, query, notExpectedFragment,
					"Query should NOT contain '%s': %s", notExpectedFragment, tt.description)
			}

			// Check that the arguments match expected values
			if len(tt.expectedArgs) > 0 {
				// Note: whereArgs may have more arguments than we're testing,
				// so we just check that our expected args are somewhere in there
				foundExpectedArgs := 0
				for _, expectedArg := range tt.expectedArgs {
					for _, actualArg := range whereArgs {
						if actualArg == expectedArg {
							foundExpectedArgs++
							break
						}
					}
				}
				assert.Equal(t, len(tt.expectedArgs), foundExpectedArgs,
					"Should find all expected arguments in whereArgs: %s", tt.description)
			}

		})
	}
}
