package accounting

import (
	"errors"
	"testing"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/mocks"
	"github.com/algorand/indexer/types"
)

func TestBasic(t *testing.T) {
	var a sdk_types.Address
	a[0] = 'a'

	account := models.Account{
		Address:                     a.String(),
		Amount:                      100,
		AmountWithoutPendingRewards: 100,
		Round:                       8,
	}

	txnBytes := msgpack.Encode(sdk_types.SignedTxnWithAD{
		SignedTxn: sdk_types.SignedTxn{
			Txn: sdk_types.Transaction{
				Type: sdk_types.PaymentTx,
				PaymentTxnFields: sdk_types.PaymentTxnFields{
					Receiver: a,
					Amount:   2,
				},
			},
		},
	})
	txnRow := idb.TxnRow{
		Round:    7,
		TxnBytes: txnBytes,
	}

	ch := make(chan idb.TxnRow, 1)
	ch <- txnRow
	close(ch)
	var outCh <-chan idb.TxnRow = ch

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts").Return(idb.SpecialAccounts{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(8))

	account, err := AccountAtRound(account, 6, db)
	assert.NoError(t, err)

	assert.Equal(t, uint64(98), account.Amount)
}

// Test that when idb.Transactions() returns stale data the first time, we return an error.
func TestStaleTransactions1(t *testing.T) {
	var a sdk_types.Address
	a[0] = 'a'

	account := models.Account{
		Address: a.String(),
		Round:   8,
	}

	var outCh <-chan idb.TxnRow

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts").Return(idb.SpecialAccounts{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(7)).Once()

	account, err := AccountAtRound(account, 6, db)
	assert.True(t, errors.As(err, &types.ConsistencyError{}), "err: %v", err)
}

// Test that when idb.Transactions() returns stale data the second time, we return an error.
func TestStaleTransactions2(t *testing.T) {
	var a sdk_types.Address
	a[0] = 'a'

	account := models.Account{
		Address:                     a.String(),
		Amount:                      100,
		AmountWithoutPendingRewards: 100,
		Round:                       8,
	}

	txnBytes := msgpack.Encode(sdk_types.SignedTxnWithAD{
		SignedTxn: sdk_types.SignedTxn{
			Txn: sdk_types.Transaction{
				Type: sdk_types.PaymentTx,
			},
		},
	})
	txnRow := idb.TxnRow{
		Round:    7,
		TxnBytes: txnBytes,
	}

	ch := make(chan idb.TxnRow, 1)
	ch <- txnRow
	close(ch)
	var outCh <-chan idb.TxnRow = ch

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts").Return(idb.SpecialAccounts{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(8)).Once()
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(5)).Once()

	account, err := AccountAtRound(account, 6, db)
	assert.True(t, errors.As(err, &types.ConsistencyError{}), "err: %v", err)
}
