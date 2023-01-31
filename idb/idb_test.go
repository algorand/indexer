package idb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util/test"
)

func TestTxnRowNext(t *testing.T) {
	// txn with 2 inner transactions and 2 nested inner transactions
	stxn := test.MakeAppCallWithInnerTxnV2(sdk.Address(test.AccountA), sdk.Address(test.AccountB), sdk.Address(test.AccountC), sdk.Address(test.AccountD), sdk.Address(test.AccountE))

	testcases := []struct {
		name string
		// input
		ascending bool
		txnRow    idb.TxnRow
		// expected
		round  uint64
		intra  uint32
		errMsg string
	}{
		{
			name:   "simple 1",
			txnRow: idb.TxnRow{Intra: 0, Round: 0},
			round:  0,
			intra:  0,
		},
		{
			name:   "simple 2",
			txnRow: idb.TxnRow{Intra: 500, Round: 1_234_567_890},
			round:  1_234_567_890,
			intra:  500,
		},
		{
			name:      "inner txns descending",
			ascending: false,
			txnRow: idb.TxnRow{
				RootTxn: &stxn,
				Extra: idb.TxnExtra{
					RootIntra: idb.OptionalUint{Present: true, Value: 50},
				},
				Intra: 51,
				Round: 1_234_567_890,
			},
			round: 1_234_567_890,
			intra: 50,
		},
		{
			name:      "inner txns ascending",
			ascending: true,
			txnRow: idb.TxnRow{
				RootTxn: &stxn,
				Extra: idb.TxnExtra{
					RootIntra: idb.OptionalUint{Present: true, Value: 50},
				},
				Intra: 51,
				Round: 1_234_567_890,
			},
			round: 1_234_567_890,
			intra: 54, // RootIntra + RootTxnBytes.numInnerTxns()
		},
		{
			name:      "root txn absent",
			ascending: true,
			txnRow: idb.TxnRow{
				Extra: idb.TxnExtra{
					RootIntra: idb.OptionalUint{Present: true, Value: 50},
				},
				Intra: 51,
				Round: 1_234_567_890,
			},
			errMsg: "was not given transaction",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			nextStr, err := tc.txnRow.Next(tc.ascending)
			if tc.errMsg != "" {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				return
			}
			require.NoError(t, err)

			round, intra, err := idb.DecodeTxnRowNext(nextStr)
			require.NoError(t, err)
			assert.Equal(t, tc.round, round)
			assert.Equal(t, tc.intra, intra)
		})
	}
}
