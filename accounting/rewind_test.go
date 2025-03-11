package accounting

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	models "github.com/algorand/indexer/v3/api/generated/v2"
	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/idb/mocks"
	"github.com/algorand/indexer/v3/types"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
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
	var a sdk.Address
	a[0] = 'a'

	account := models.Account{
		Address: a.String(),
		Round:   8,
	}

	ch := make(chan idb.TxnRow)
	var outCh <-chan idb.TxnRow = ch
	close(ch)

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts", mock.Anything).Return(types.SpecialAddresses{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(7)).Once()

	account, err := AccountAtRound(context.Background(), account, 6, db)
	assert.True(t, errors.As(err, &ConsistencyError{}), "err: %v", err)
}

func TestKeyRegistrationApplicationTxn(t *testing.T) {
	var a sdk.Address
	a[0] = 'a'

	// Set up account with participation and app state
	account := models.Account{
		Address:                     a.String(),
		Amount:                      100,
		AmountWithoutPendingRewards: 100,
		Round:                       8,
		Participation: &models.AccountParticipation{
			VoteFirstValid: 100, VoteLastValid: 200, VoteKeyDilution: 10000,
		},
		AppsLocalState: &[]models.ApplicationLocalState{{Id: 123}},
		CreatedApps:    &[]models.Application{{Id: 456}},
	}

	// Create test transactions - one KeyReg and one AppCall
	keyregTxn := idb.TxnRow{
		Round: 7,
		Txn: &sdk.SignedTxnWithAD{SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{Type: sdk.KeyRegistrationTx, Header: sdk.Header{Sender: a}},
		}}}

	appCallTxn := idb.TxnRow{
		Round: 8,
		Txn: &sdk.SignedTxnWithAD{SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{Type: sdk.ApplicationCallTx, Header: sdk.Header{Sender: a}},
		}}}

	// Send both transactions to the mock DB
	ch := make(chan idb.TxnRow, 2)
	ch <- appCallTxn
	ch <- keyregTxn
	close(ch)
	var outCh <-chan idb.TxnRow = ch

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts", mock.Anything).Return(types.SpecialAddresses{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(8))

	// Run the rewind
	result, err := AccountAtRound(context.Background(), account, 6, db)
	assert.NoError(t, err)

	// Verify that both participation and app state fields are nil after rewind
	assert.Nil(t, result.Participation, "Participation should be nil after rewinding KeyRegistration transaction")
	assert.Nil(t, result.AppsLocalState, "AppsLocalState should be nil after rewinding ApplicationCall transaction")
	assert.Nil(t, result.CreatedApps, "CreatedApps should be nil after rewinding ApplicationCall transaction")
}
