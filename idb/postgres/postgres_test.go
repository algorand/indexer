package postgres

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
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
