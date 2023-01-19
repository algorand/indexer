package accounting

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/indexer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/mocks"
)

func TestBasic(t *testing.T) {
	var a sdk.Address
	a[0] = 'a'

	account := models.Account{
		Address:                     a.String(),
		Amount:                      100,
		AmountWithoutPendingRewards: 100,
		Round:                       8,
	}

	txnRow := idb.TxnRow{
		Round: 7,
		Txn: &sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{
					Type: sdk.PaymentTx,
					PaymentTxnFields: sdk.PaymentTxnFields{
						Receiver: a,
						Amount:   sdk.MicroAlgos(2),
					},
				},
			},
		},
	}

	ch := make(chan idb.TxnRow, 1)
	ch <- txnRow
	close(ch)
	var outCh <-chan idb.TxnRow = ch

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts", mock.Anything).Return(types.SpecialAddresses{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(8))

	account, err := AccountAtRound(context.Background(), account, 6, db)
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

	ch := make(chan idb.TxnRow)
	var outCh <-chan idb.TxnRow = ch
	close(ch)

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts", mock.Anything).Return(transactions.SpecialAddresses{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(7)).Once()

	account, err := AccountAtRound(context.Background(), account, 6, db)
	assert.True(t, errors.As(err, &ConsistencyError{}), "err: %v", err)
}
