package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/v3/idb"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

func TestCallWithTimeoutTimesOut(t *testing.T) {
	done := make(chan struct{})
	defer func() {
		close(done)
	}()

	logger, hook := test.NewNullLogger()
	err := callWithTimeout(context.Background(), logger, 1*time.Nanosecond, func(ctx context.Context) error {
		<-done
		return errors.New("should not return")
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errTimeout)

	time.Sleep(2 * time.Second)
	require.Len(t, hook.Entries, 1)
	require.Equal(t, errMisbehavingHandler, hook.LastEntry().Message)
}

func TestCallWithTimeoutExitsWhenHandlerFinishes(t *testing.T) {
	done := make(chan struct{})
	defer func() {
		<-done
	}()

	callError := errors.New("this should be the result")
	err := callWithTimeout(context.Background(), nil, 1*time.Minute, func(ctx context.Context) error {
		defer close(done)
		return callError
	})

	require.Error(t, err)
	require.ErrorIs(t, err, callError)
}

func TestInvalidTxnRow(t *testing.T) {
	stxn := sdk.SignedTxnWithAD{}
	invalidRow := idb.TxnRow{Txn: &stxn, RootTxn: &stxn}
	_, err := txnRowToTransaction(invalidRow)
	require.Error(t, err)
	require.ErrorContains(t, err, "Txn and RootTxn should be mutually exclusive")
}

// TestTxnAccessConversion tests the conversion of txn.Access field combinations
// from SDK types to the generated API types, exercising all the logic in
// converter_utils.go lines 504-588
func TestTxnAccessConversion(t *testing.T) {
	// Helper to create a valid non-zero address
	makeAddress := func(seed byte) sdk.Address {
		var addrBytes [32]byte
		for i := 0; i < 32; i++ {
			addrBytes[i] = seed + byte(i)
		}
		return sdk.Address(addrBytes)
	}

	// Helper to create a basic signed transaction with application call
	createAppCallTxn := func(access []sdk.ResourceRef) *sdk.SignedTxnWithAD {
		sender := makeAddress(10)

		txn := sdk.Transaction{
			Type: sdk.ApplicationCallTx,
			Header: sdk.Header{
				Sender:     sender,
				Fee:        sdk.MicroAlgos(1000),
				FirstValid: 1000,
				LastValid:  2000,
			},
			ApplicationFields: sdk.ApplicationFields{
				ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
					ApplicationID: sdk.AppIndex(123),
					Access:        access,
				},
			},
		}

		return &sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				Txn: txn,
			},
		}
	}

	extra := rowData{
		Round:     1,
		RoundTime: 1234567890,
		Intra:     0,
	}

	t.Run("Empty Access Array", func(t *testing.T) {
		stxn := createAppCallTxn([]sdk.ResourceRef{})

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		assert.Equal(t, 0, len(*result.ApplicationTransaction.Access))
	})

	t.Run("Direct Address Access", func(t *testing.T) {
		testAddr := makeAddress(1)

		access := []sdk.ResourceRef{
			{Address: testAddr},
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		accessItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, accessItem.Address)
		assert.Equal(t, testAddr.String(), *accessItem.Address)
		assert.Nil(t, accessItem.ApplicationId)
		assert.Nil(t, accessItem.AssetId)
		assert.Nil(t, accessItem.Holding)
		assert.Nil(t, accessItem.Local)
		assert.Nil(t, accessItem.Box)
	})

	t.Run("Direct App Access", func(t *testing.T) {
		access := []sdk.ResourceRef{
			{App: sdk.AppIndex(456)},
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		accessItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, accessItem.ApplicationId)
		assert.Equal(t, uint64(456), *accessItem.ApplicationId)
		assert.Nil(t, accessItem.Address)
		assert.Nil(t, accessItem.AssetId)
		assert.Nil(t, accessItem.Holding)
		assert.Nil(t, accessItem.Local)
		assert.Nil(t, accessItem.Box)
	})

	t.Run("Direct Asset Access", func(t *testing.T) {
		access := []sdk.ResourceRef{
			{Asset: sdk.AssetIndex(789)},
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		accessItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, accessItem.AssetId)
		assert.Equal(t, uint64(789), *accessItem.AssetId)
		assert.Nil(t, accessItem.Address)
		assert.Nil(t, accessItem.ApplicationId)
		assert.Nil(t, accessItem.Holding)
		assert.Nil(t, accessItem.Local)
		assert.Nil(t, accessItem.Box)
	})

	t.Run("Asset Holding Access", func(t *testing.T) {
		testAddr := makeAddress(2)

		access := []sdk.ResourceRef{
			{Address: testAddr},                             // index 0 - address reference
			{Asset: sdk.AssetIndex(100)},                    // index 1 - asset reference
			{Holding: sdk.HoldingRef{Address: 1, Asset: 2}}, // holding referencing indices
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 3, len(*result.ApplicationTransaction.Access))

		// Check the holding access item (should be index 2)
		holdingItem := (*result.ApplicationTransaction.Access)[2]
		require.NotNil(t, holdingItem.Holding)
		assert.Equal(t, testAddr.String(), holdingItem.Holding.Address)
		assert.Equal(t, uint64(100), holdingItem.Holding.Asset)
		assert.Nil(t, holdingItem.Address)
		assert.Nil(t, holdingItem.ApplicationId)
		assert.Nil(t, holdingItem.AssetId)
		assert.Nil(t, holdingItem.Local)
		assert.Nil(t, holdingItem.Box)
	})

	t.Run("Asset Holding Access - Sender Reference", func(t *testing.T) {
		access := []sdk.ResourceRef{
			{Asset: sdk.AssetIndex(200)},                    // index 0
			{Holding: sdk.HoldingRef{Address: 0, Asset: 1}}, // Address: 0 = sender
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 2, len(*result.ApplicationTransaction.Access))

		holdingItem := (*result.ApplicationTransaction.Access)[1]
		require.NotNil(t, holdingItem.Holding)
		assert.Equal(t, makeAddress(10).String(), holdingItem.Holding.Address) // sender = makeAddress(10) from createAppCallTxn
		assert.Equal(t, uint64(200), holdingItem.Holding.Asset)
	})

	t.Run("Local State Access", func(t *testing.T) {
		testAddr := makeAddress(3)

		access := []sdk.ResourceRef{
			{Address: testAddr},                         // index 0 - address reference
			{App: sdk.AppIndex(300)},                    // index 1 - app reference
			{Locals: sdk.LocalsRef{Address: 1, App: 2}}, // local referencing indices
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 3, len(*result.ApplicationTransaction.Access))

		// Check the local access item (should be index 2)
		localItem := (*result.ApplicationTransaction.Access)[2]
		require.NotNil(t, localItem.Local)
		assert.Equal(t, testAddr.String(), localItem.Local.Address)
		assert.Equal(t, uint64(300), localItem.Local.App)
		assert.Nil(t, localItem.Address)
		assert.Nil(t, localItem.ApplicationId)
		assert.Nil(t, localItem.AssetId)
		assert.Nil(t, localItem.Holding)
		assert.Nil(t, localItem.Box)
	})

	t.Run("Local State Access - This App Reference", func(t *testing.T) {
		testAddr := makeAddress(4)

		access := []sdk.ResourceRef{
			{Address: testAddr},                         // index 0
			{Locals: sdk.LocalsRef{Address: 1, App: 0}}, // App: 0 = this app
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 2, len(*result.ApplicationTransaction.Access))

		localItem := (*result.ApplicationTransaction.Access)[1]
		require.NotNil(t, localItem.Local)
		assert.Equal(t, testAddr.String(), localItem.Local.Address)
		assert.Equal(t, uint64(123), localItem.Local.App) // this app = 123 (ApplicationID from createAppCallTxn)
	})

	t.Run("Local State Access - Sender Reference", func(t *testing.T) {
		access := []sdk.ResourceRef{
			{App: sdk.AppIndex(500)},                    // index 0
			{Locals: sdk.LocalsRef{Address: 0, App: 1}}, // Address: 0 = sender
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 2, len(*result.ApplicationTransaction.Access))

		localItem := (*result.ApplicationTransaction.Access)[1]
		require.NotNil(t, localItem.Local)
		assert.Equal(t, makeAddress(10).String(), localItem.Local.Address) // sender = makeAddress(10) from createAppCallTxn
		assert.Equal(t, uint64(500), localItem.Local.App)
	})

	t.Run("Box Reference Access", func(t *testing.T) {
		boxName := []byte("test-box-name")

		access := []sdk.ResourceRef{
			{App: sdk.AppIndex(400)},                                 // index 0 - app reference
			{Box: sdk.BoxReference{ForeignAppIdx: 1, Name: boxName}}, // box referencing app
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 2, len(*result.ApplicationTransaction.Access))

		// Check the box access item (should be index 1)
		boxItem := (*result.ApplicationTransaction.Access)[1]
		require.NotNil(t, boxItem.Box)
		assert.Equal(t, uint64(400), boxItem.Box.App)
		assert.Equal(t, boxName, boxItem.Box.Name)
		assert.Nil(t, boxItem.Address)
		assert.Nil(t, boxItem.ApplicationId)
		assert.Nil(t, boxItem.AssetId)
		assert.Nil(t, boxItem.Holding)
		assert.Nil(t, boxItem.Local)
	})

	t.Run("Box Reference Access - This App", func(t *testing.T) {
		boxName := []byte("this-app-box")

		access := []sdk.ResourceRef{
			{Box: sdk.BoxReference{ForeignAppIdx: 0, Name: boxName}}, // ForeignAppIdx: 0 = this app
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		boxItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, boxItem.Box)
		assert.Equal(t, uint64(123), boxItem.Box.App) // this app = 123 (ApplicationID from createAppCallTxn)
		assert.Equal(t, boxName, boxItem.Box.Name)
	})

	t.Run("Default to Box Reference", func(t *testing.T) {
		// Test the default case: when all fields are empty/zero, it should default to box reference
		boxName := []byte("default-box")

		access := []sdk.ResourceRef{
			{Box: sdk.BoxReference{ForeignAppIdx: 0, Name: boxName}}, // All other fields zero
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		// Should have created a box reference as the default
		boxItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, boxItem.Box)
		assert.Equal(t, uint64(123), boxItem.Box.App) // ForeignAppIdx 0 resolves to ApplicationID (123)
		assert.Equal(t, boxName, boxItem.Box.Name)
	})

	t.Run("Multiple Access Types", func(t *testing.T) {
		testAddr1 := makeAddress(5)
		boxName := []byte("multi-test-box")

		access := []sdk.ResourceRef{
			{Address: testAddr1},                                     // Direct address
			{App: sdk.AppIndex(500)},                                 // Direct app
			{Asset: sdk.AssetIndex(600)},                             // Direct asset
			{Holding: sdk.HoldingRef{Address: 1, Asset: 3}},          // Holding reference
			{Locals: sdk.LocalsRef{Address: 1, App: 2}},              // Local reference
			{Box: sdk.BoxReference{ForeignAppIdx: 2, Name: boxName}}, // Box reference
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		assert.Equal(t, 6, len(*result.ApplicationTransaction.Access))

		// Verify each access type
		accessArray := *result.ApplicationTransaction.Access

		// Index 0: Direct address
		assert.NotNil(t, accessArray[0].Address)
		assert.Equal(t, testAddr1.String(), *accessArray[0].Address)

		// Index 1: Direct app
		assert.NotNil(t, accessArray[1].ApplicationId)
		assert.Equal(t, uint64(500), *accessArray[1].ApplicationId)

		// Index 2: Direct asset
		assert.NotNil(t, accessArray[2].AssetId)
		assert.Equal(t, uint64(600), *accessArray[2].AssetId)

		// Index 3: Holding reference
		require.NotNil(t, accessArray[3].Holding)
		assert.Equal(t, testAddr1.String(), accessArray[3].Holding.Address) // references index 1
		assert.Equal(t, uint64(600), accessArray[3].Holding.Asset)          // references index 3

		// Index 4: Local reference
		require.NotNil(t, accessArray[4].Local)
		assert.Equal(t, testAddr1.String(), accessArray[4].Local.Address) // references index 1
		assert.Equal(t, uint64(500), accessArray[4].Local.App)            // references index 2

		// Index 5: Box reference
		require.NotNil(t, accessArray[5].Box)
		assert.Equal(t, uint64(500), accessArray[5].Box.App) // references index 2
		assert.Equal(t, boxName, accessArray[5].Box.Name)
	})

	t.Run("Invalid Reference Indices - Holding", func(t *testing.T) {
		// Test that invalid holding reference indices result in zero values
		access := []sdk.ResourceRef{
			{Holding: sdk.HoldingRef{Address: 10, Asset: 20}}, // Invalid indices
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		holdingItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, holdingItem.Holding)
		// Invalid indices should result in zero address and zero asset
		assert.Equal(t, sdk.Address{}.String(), holdingItem.Holding.Address)
		assert.Equal(t, uint64(0), holdingItem.Holding.Asset)
	})

	t.Run("Invalid Reference Indices - Locals", func(t *testing.T) {
		// Test that invalid locals reference indices result in zero values
		access := []sdk.ResourceRef{
			{Locals: sdk.LocalsRef{Address: 5, App: 10}}, // Invalid indices
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		localItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, localItem.Local)
		// Invalid indices should result in zero address and zero app
		assert.Equal(t, sdk.Address{}.String(), localItem.Local.Address)
		assert.Equal(t, uint64(0), localItem.Local.App)
	})

	t.Run("Invalid Reference Indices - Box", func(t *testing.T) {
		// Test that invalid box reference indices result in zero appID
		boxName := []byte("invalid-box")
		access := []sdk.ResourceRef{
			{Box: sdk.BoxReference{ForeignAppIdx: 10, Name: boxName}}, // Invalid index
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		// Should have 1 item with zero appID due to invalid reference
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		boxItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, boxItem.Box)
		assert.Equal(t, uint64(0), boxItem.Box.App) // Invalid index should result in 0
		assert.Equal(t, boxName, boxItem.Box.Name)
	})

	t.Run("Partially Invalid Reference Indices", func(t *testing.T) {
		// Test mixed valid and invalid reference indices
		testAddr := makeAddress(6)
		access := []sdk.ResourceRef{
			{Address: testAddr},                             // Valid - index 0
			{Holding: sdk.HoldingRef{Address: 1, Asset: 5}}, // Valid address ref, invalid asset ref
			{Locals: sdk.LocalsRef{Address: 8, App: 1}},     // Invalid address ref, but App should resolve to zero
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 3, len(*result.ApplicationTransaction.Access))

		// First item should be valid direct address
		assert.NotNil(t, (*result.ApplicationTransaction.Access)[0].Address)
		assert.Equal(t, testAddr.String(), *(*result.ApplicationTransaction.Access)[0].Address)

		// Second item: holding with valid address but invalid asset reference
		holdingItem := (*result.ApplicationTransaction.Access)[1]
		require.NotNil(t, holdingItem.Holding)
		assert.Equal(t, testAddr.String(), holdingItem.Holding.Address) // Should resolve to testAddr
		assert.Equal(t, uint64(0), holdingItem.Holding.Asset)           // Invalid asset ref should be 0

		// Third item: locals with invalid address but app should be resolved
		localItem := (*result.ApplicationTransaction.Access)[2]
		require.NotNil(t, localItem.Local)
		assert.Equal(t, sdk.Address{}.String(), localItem.Local.Address) // Invalid address ref should be zero
		assert.Equal(t, uint64(0), localItem.Local.App)                  // Should try to resolve but get 0
	})

	t.Run("Edge Case - App Creation Local State Access", func(t *testing.T) {
		// Test local state access during app creation (ApplicationID = 0)
		sender := makeAddress(10)
		txn := sdk.Transaction{
			Type: sdk.ApplicationCallTx,
			Header: sdk.Header{
				Sender:     sender,
				Fee:        sdk.MicroAlgos(1000),
				FirstValid: 1000,
				LastValid:  2000,
			},
			ApplicationFields: sdk.ApplicationFields{
				ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
					ApplicationID: 0, // App creation
					Access: []sdk.ResourceRef{
						{Locals: sdk.LocalsRef{Address: 0, App: 0}}, // sender's locals for this app
					},
				},
			},
		}

		stxn := &sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				Txn: txn,
			},
			ApplyData: sdk.ApplyData{
				ApplicationID: 456, // Created app ID from ApplyData
			},
		}

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		accessItem := (*result.ApplicationTransaction.Access)[0]
		// Since both Address and App are 0, this will fall through to the default case (box reference)
		require.NotNil(t, accessItem.Box)
		assert.Equal(t, uint64(456), accessItem.Box.App) // ForeignAppIdx 0 resolves to ApplyData.ApplicationID (456)
		assert.Equal(t, []byte(nil), accessItem.Box.Name)
		assert.Nil(t, accessItem.Local)
	})

	t.Run("Edge Case - Mixed Zero and Non-Zero in ResourceRef", func(t *testing.T) {
		// Test a ResourceRef where only some fields are set
		boxName := []byte("mixed-box")
		access := []sdk.ResourceRef{
			{
				// This tests the "else" fallback case where direct fields are zero but Box is set
				Box: sdk.BoxReference{ForeignAppIdx: 0, Name: boxName},
			},
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		boxItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, boxItem.Box)
		assert.Equal(t, uint64(123), boxItem.Box.App) // ForeignAppIdx 0 should resolve to ApplicationID (123)
		assert.Equal(t, boxName, boxItem.Box.Name)
		// Ensure other fields are nil
		assert.Nil(t, boxItem.Address)
		assert.Nil(t, boxItem.ApplicationId)
		assert.Nil(t, boxItem.AssetId)
		assert.Nil(t, boxItem.Holding)
		assert.Nil(t, boxItem.Local)
	})

	t.Run("Edge Case - Box Creation During App Creation", func(t *testing.T) {
		// Test box reference during app creation (ApplicationID = 0, ForeignAppIdx = 0)
		sender := makeAddress(11)
		boxName := []byte("creation-box")
		txn := sdk.Transaction{
			Type: sdk.ApplicationCallTx,
			Header: sdk.Header{
				Sender:     sender,
				Fee:        sdk.MicroAlgos(1000),
				FirstValid: 1000,
				LastValid:  2000,
			},
			ApplicationFields: sdk.ApplicationFields{
				ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
					ApplicationID: 0, // App creation
					Access: []sdk.ResourceRef{
						{Box: sdk.BoxReference{ForeignAppIdx: 0, Name: boxName}}, // this app during creation
					},
				},
			},
		}

		stxn := &sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				Txn: txn,
			},
			ApplyData: sdk.ApplyData{
				ApplicationID: 789, // Created app ID from ApplyData
			},
		}

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		boxItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, boxItem.Box)
		assert.Equal(t, uint64(789), boxItem.Box.App) // Should use ApplyData.ApplicationID
		assert.Equal(t, boxName, boxItem.Box.Name)
	})

	t.Run("Edge Case - Zero Asset in Holding", func(t *testing.T) {
		// Test holding reference where asset is zero - this will NOT create a holding because Asset == 0 fails the condition check
		// Instead it will fall through to the default case (box reference)
		boxName := []byte("") // Empty box name
		access := []sdk.ResourceRef{
			{Holding: sdk.HoldingRef{Address: 1, Asset: 0}, Box: sdk.BoxReference{ForeignAppIdx: 0, Name: boxName}}, // Asset = 0, falls to default
		}
		stxn := createAppCallTxn(access)

		result, err := signedTxnWithAdToTransaction(stxn, extra)
		require.NoError(t, err)

		require.NotNil(t, result.ApplicationTransaction)
		require.NotNil(t, result.ApplicationTransaction.Access)
		require.Equal(t, 1, len(*result.ApplicationTransaction.Access))

		// Should be a box reference due to fallthrough from Asset == 0
		boxItem := (*result.ApplicationTransaction.Access)[0]
		require.NotNil(t, boxItem.Box)
		assert.Equal(t, uint64(123), boxItem.Box.App) // ForeignAppIdx 0 resolves to ApplicationID (123)
		assert.Equal(t, boxName, boxItem.Box.Name)
		// Ensure holding is nil
		assert.Nil(t, boxItem.Holding)
	})
}
