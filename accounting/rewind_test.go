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

// TestAllClearedFields verifies that all fields that should be reset during a rewind are properly cleared
func TestAllClearedFields(t *testing.T) {
	var a sdk.Address
	a[0] = 'a'

	// Helper functions for pointer values
	uint64Ptr := func(v uint64) *uint64 { return &v }
	boolPtr := func(v bool) *bool { return &v }
	bytesPtr := func(v []byte) *[]byte { return &v }
	stringPtr := func(v string) *string { return &v }

	// Create an account with ALL fields populated
	account := models.Account{
		Address:                     a.String(),
		Amount:                      1000,
		AmountWithoutPendingRewards: 980,
		PendingRewards:              20,
		Rewards:                     100,
		Round:                       10,
		Status:                      "Online",
		MinBalance:                  200,

		// Keyreg-related fields
		Participation: &models.AccountParticipation{
			VoteFirstValid:            100,
			VoteLastValid:             200,
			VoteKeyDilution:           10000,
			VoteParticipationKey:      []byte("votepk"),
			SelectionParticipationKey: []byte("selpk"),
			StateProofKey:             bytesPtr([]byte("stpk")),
		},

		// App-related fields
		AppsLocalState:      &[]models.ApplicationLocalState{{Id: 123}},
		AppsTotalExtraPages: uint64Ptr(2),
		AppsTotalSchema:     &models.ApplicationStateSchema{NumByteSlice: 10, NumUint: 10},
		CreatedApps:         &[]models.Application{{Id: 456}},
		TotalAppsOptedIn:    5,
		TotalBoxBytes:       1000,
		TotalBoxes:          10,
		TotalCreatedApps:    3,

		// Asset-related fields
		Assets:             &[]models.AssetHolding{{AssetId: 789, Amount: 50}},
		CreatedAssets:      &[]models.Asset{{Index: 999}},
		TotalAssetsOptedIn: 2,
		TotalCreatedAssets: 1,

		// Fields set at account creation/deletion
		ClosedAtRound:  uint64Ptr(500),
		CreatedAtRound: uint64Ptr(1),
		Deleted:        boolPtr(false),

		// Incentive fields
		IncentiveEligible: boolPtr(true),
		LastHeartbeat:     uint64Ptr(7),
		LastProposed:      uint64Ptr(6),

		// Auth fields
		AuthAddr: stringPtr("authaddr"),
		SigType:  (*models.AccountSigType)(stringPtr(string(models.AccountSigTypeSig))),
	}

	// Create various transaction types for testing
	txns := []idb.TxnRow{
		{ // Application call
			Round: 10,
			Txn: &sdk.SignedTxnWithAD{SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{Type: sdk.ApplicationCallTx, Header: sdk.Header{Sender: a}},
			}},
		},
		{ // Key registration
			Round: 9,
			Txn: &sdk.SignedTxnWithAD{SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{Type: sdk.KeyRegistrationTx, Header: sdk.Header{Sender: a}},
			}},
		},
		{ // Payment
			Round: 8,
			Txn: &sdk.SignedTxnWithAD{SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{
					Type:             sdk.PaymentTx,
					Header:           sdk.Header{Sender: a},
					PaymentTxnFields: sdk.PaymentTxnFields{Amount: 10},
				},
			}},
		},
	}

	// Set up mock DB
	ch := make(chan idb.TxnRow, len(txns))
	for _, txn := range txns {
		ch <- txn
	}
	close(ch)
	var outCh <-chan idb.TxnRow = ch

	db := &mocks.IndexerDb{}
	db.On("GetSpecialAccounts", mock.Anything).Return(types.SpecialAddresses{}, nil)
	db.On("Transactions", mock.Anything, mock.Anything).Return(outCh, uint64(10))

	// Run the rewind
	result, err := AccountAtRound(context.Background(), account, 5, db)
	assert.NoError(t, err)

	// Verify all fields that should be reset or zeroed out

	// Fields that should be preserved/changed correctly
	assert.Equal(t, a.String(), result.Address, "Address should be preserved")

	// Fields that are explicitly zeroed out
	assert.Equal(t, uint64(0), result.Rewards, "Rewards should be 0")
	assert.Equal(t, uint64(0), result.PendingRewards, "PendingRewards should be 0")
	assert.Equal(t, uint64(0), result.MinBalance, "MinBalance should be 0")

	// Fields that are explicitly set to nil
	assert.Nil(t, result.ClosedAtRound, "ClosedAtRound should be nil")

	// Fields nulled out by KeyRegistrationTx
	assert.Nil(t, result.Participation, "Participation should be nil")

	// Fields nulled out by ApplicationCallTx
	assert.Nil(t, result.AppsLocalState, "AppsLocalState should be nil")
	assert.Nil(t, result.AppsTotalExtraPages, "AppsTotalExtraPages should be nil")
	assert.Nil(t, result.AppsTotalSchema, "AppsTotalSchema should be nil")
	assert.Nil(t, result.CreatedApps, "CreatedApps should be nil")
	assert.Equal(t, uint64(0), result.TotalAppsOptedIn, "TotalAppsOptedIn should be 0")
	assert.Equal(t, uint64(0), result.TotalBoxBytes, "TotalBoxBytes should be 0")
	assert.Equal(t, uint64(0), result.TotalBoxes, "TotalBoxes should be 0")
	assert.Equal(t, uint64(0), result.TotalCreatedApps, "TotalCreatedApps should be 0")

	// Incentive fields explicitly set to nil
	assert.Nil(t, result.IncentiveEligible, "IncentiveEligible should be nil")
	assert.Nil(t, result.LastHeartbeat, "LastHeartbeat should be nil")
	assert.Nil(t, result.LastProposed, "LastProposed should be nil")

	// Assets should be preserved (although updated with AssetConfigTx/AssetTransferTx)
	assert.NotNil(t, result.Assets, "Assets should not be nil")

	// Round should be set to the target round
	assert.Equal(t, uint64(5), result.Round, "Round should be set to target round")
}
