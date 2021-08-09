package accounting

import (
	"errors"
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/mocks"
)

func TestBasic(t *testing.T) {
	var a basics.Address
	a[0] = 'a'

	account := models.Account{
		Address:                     a.String(),
		Amount:                      100,
		AmountWithoutPendingRewards: 100,
		Round:                       8,
	}

	txnBytes := protocol.Encode(&transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: protocol.PaymentTx,
				PaymentTxnFields: transactions.PaymentTxnFields{
					Receiver: a,
					Amount:   basics.MicroAlgos{Raw: 2},
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
	db.On("GetSpecialAccounts").Return(transactions.SpecialAddresses{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(8))

	account, err := AccountAtRound(account, 6, db)
	assert.NoError(t, err)

	assert.Equal(t, uint64(98), account.Amount)
}

// Test that when idb.Transactions() returns stale data the first time, we return an error.
func TestStaleTransactions1(t *testing.T) {
	var a basics.Address
	a[0] = 'a'

	account := models.Account{
		Address: a.String(),
		Round:   8,
	}

	var outCh <-chan idb.TxnRow

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts").Return(transactions.SpecialAddresses{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(7)).Once()

	account, err := AccountAtRound(account, 6, db)
	assert.True(t, errors.As(err, &ConsistencyError{}), "err: %v", err)
}

// Test that when idb.Transactions() returns stale data the second time, we return an error.
func TestStaleTransactions2(t *testing.T) {
	var a basics.Address
	a[0] = 'a'

	account := models.Account{
		Address:                     a.String(),
		Amount:                      100,
		AmountWithoutPendingRewards: 100,
		Round:                       8,
	}

	txnBytes := protocol.Encode(&transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: protocol.PaymentTx,
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
	db.On("GetSpecialAccounts").Return(transactions.SpecialAddresses{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(8)).Once()
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(5)).Once()

	account, err := AccountAtRound(account, 6, db)
	assert.True(t, errors.As(err, &ConsistencyError{}), "err: %v", err)
}
