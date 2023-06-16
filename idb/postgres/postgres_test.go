package postgres

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/types"
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
	us_east_tz, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	us_west_tz, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	random_date_utc := time.Date(1000, time.December, 25, 1, 2, 3, 4, time.UTC)
	tests := []struct {
		name      string
		arg       idb.TransactionFilter
		whereArgs []interface{}
	}{
		{
			"BeforeTime UTC to UTC",
			idb.TransactionFilter{
				BeforeTime: random_date_utc,
			},
			[]interface{}{random_date_utc},
		},
		{
			"AfterTime UTC to UTC",
			idb.TransactionFilter{
				AfterTime: random_date_utc,
			},
			[]interface{}{random_date_utc},
		},
		{
			"BeforeTime AfterTime Conversion",
			idb.TransactionFilter{
				BeforeTime: time.Date(1000, time.December, 25, 1, 2, 3, 4, us_east_tz),
				AfterTime:  time.Date(1000, time.December, 25, 1, 2, 3, 4, us_west_tz),
			},
			[]interface{}{
				time.Date(1000, time.December, 25, 1, 2, 3, 4, us_east_tz).UTC(),
				time.Date(1000, time.December, 25, 1, 2, 3, 4, us_west_tz).UTC(),
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
