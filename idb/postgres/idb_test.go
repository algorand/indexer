package postgres

import (
	"github.com/algorand/indexer/idb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTxnRowNext(t *testing.T) {
	testcases := []struct{
		name string
		round uint64
		intra uint32
		txid string
		encodeError string
	} {
		{
			name: "simple 1",
			round: 0,
			intra: 0,
		},
		{
			name: "simple 2",
			round: 1_234_567_890,
			intra: 500,
		},
		{
			name: "txid 1",
			round: 1_234_567_890,
			intra: 500,
			txid: "S4T444EJOSHVZIN2EMWLFP6UYBTLDZZYTS2ZE3Q6GYUD53KZ5YUQ",
		},
		{
			name: "txid illegal base32",
			round: 1_234_567_890,
			intra: 500,
			txid: "S4T444EJOSHVZIN2EMWLFP6UYBTLDZ=YTS2ZE3Q6GYUD53KZ5YUQ",
			encodeError: "illegal base32 data",
		},
		{
			name: "txid wrong size",
			round: 1_234_567_890,
			intra: 500,
			txid: "BAFYBEICZSSCDSBS7FFQZ55ASQDF3SMV6KLCW3GOFSZVWLYARCI47BGF354",
			encodeError: "unexpected txid size",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			txnrow := idb.TxnRow {
				Round: tc.round,
				Intra: int(tc.intra),
				Extra: idb.TxnExtra{RootTxid: tc.txid},
			}
			nextStr, err := txnrow.Next()
			if tc.encodeError != "" {
				assert.Contains(t, err.Error(), tc.encodeError)
				return
			} else {
				require.NoError(t, err)
			}

			round, intra, txid, err := idb.DecodeTxnRowNext(nextStr)
			require.NoError(t, err)
			assert.Equal(t, tc.round, round)
			assert.Equal(t, tc.intra, intra)
			assert.Equal(t, tc.txid, txid)
		})
	}
}