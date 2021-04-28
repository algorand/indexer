package accounting

import (
	"testing"

	sdk_types "github.com/algorand/go-algorand-sdk/types"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util/test"
)

func GetAccounting() *State {
	accountingState := New(make(map[uint64]bool))
	accountingState.InitRoundParts(test.Round, test.FeeAddr, test.RewardAddr, 0)
	return accountingState
}

func assertUpdates(t *testing.T, update *idb.AlgoUpdate, closed bool, balance, rewards int64) {
	assert.Equal(t, closed, update.Closed)
	assert.Equal(t, balance, update.Balance)
	assert.Equal(t, rewards, update.Rewards)
}

func getSenderAmounts(txn *sdk_types.SignedTxnWithAD) (balance, rewards int64) {
	balance = -int64(txn.Txn.Amount) + -int64(txn.Txn.Fee) + -int64(txn.ClosingAmount) + int64(txn.SenderRewards)
	rewards = int64(txn.SenderRewards)
	return
}

func getReceiverAmounts(txn *sdk_types.SignedTxnWithAD) (balance, rewards int64) {
	balance = int64(txn.Txn.Amount) + int64(txn.ReceiverRewards)
	rewards = int64(txn.ReceiverRewards)
	return
}

func getCloseAmounts(txn *sdk_types.SignedTxnWithAD) (balance, rewards int64) {
	balance = int64(txn.ClosingAmount) + int64(txn.CloseRewards)
	rewards = int64(txn.CloseRewards)
	return
}

// TestSimplePayment checks that the accounting deltas are correct for a simple single payment.
func TestSimplePayment(t *testing.T) {
	var update *idb.AlgoUpdate
	state := GetAccounting()
	state.AddTransaction(test.OpenMain)

	senderBalance, senderRewards := getSenderAmounts(test.OpenMainStxn)
	update = state.AlgoUpdates[test.OpenMainStxn.Txn.Sender]
	assertUpdates(t, update, false, senderBalance, senderRewards)

	receiverBalance, receiverRewards := getReceiverAmounts(test.OpenMainStxn)
	update = state.AlgoUpdates[test.OpenMainStxn.Txn.Receiver]
	assertUpdates(t, update, false, receiverBalance, receiverRewards)
}

// TestSimpleAccountClose verifies that the accounting state is set correctly for an account being closed.
func TestSimpleAccountClose(t *testing.T) {
	state := GetAccounting()
	state.AddTransaction(test.CloseMainToBC)

	senderBalance, _ := getSenderAmounts(test.CloseMainToBCStxn)
	assertUpdates(t, state.AlgoUpdates[test.AccountA], true, senderBalance, 0)

	closeBalance, closeRewards := getCloseAmounts(test.CloseMainToBCStxn)
	assertUpdates(t, state.AlgoUpdates[test.CloseMainToBCStxn.Txn.CloseRemainderTo], false, closeBalance, closeRewards)

	receiverBalance, receiverRewards := getReceiverAmounts(test.CloseMainToBCStxn)
	assertUpdates(t, state.AlgoUpdates[test.CloseMainToBCStxn.Txn.Receiver], false, receiverBalance, receiverRewards)
}

// TestOpenClose verifies that the accounting state is set correctly when an account is opened and closed in the same round.
func TestOpenClose(t *testing.T) {
	var update *idb.AlgoUpdate
	state := GetAccounting()
	state.AddTransaction(test.OpenMain)
	// In order to make sure rewards were actually reset, they need to be non-zero at this point
	assert.True(t, state.AlgoUpdates[test.AccountA].Rewards > 0)
	state.AddTransaction(test.CloseMainToBC)

	// main account balance should add up to 0, and rewards should be reset to 0
	update = state.AlgoUpdates[test.AccountA]
	assertUpdates(t, update, true, 0, 0)

	senderBalance, senderRewards := getSenderAmounts(test.OpenMainStxn)
	receiverBalance, receiverRewards := getReceiverAmounts(test.CloseMainToBCStxn)
	update = state.AlgoUpdates[test.OpenMainStxn.Txn.Sender]
	assertUpdates(t, update, false, senderBalance+receiverBalance, senderRewards+receiverRewards)
}

// TestCloseOpen verifies that the accounting state is set correctly when an account is closed then reopened in the same round.
func TestCloseOpen(t *testing.T) {
	var update *idb.AlgoUpdate
	state := GetAccounting()
	state.AddTransaction(test.OpenMain)
	state.AddTransaction(test.CloseMainToBC)
	// In the real world this would be a different transaction, but this is a test.
	state.AddTransaction(test.OpenMain)

	// The account is closed, but account was closed, but then re-opened and somehow accumulated rewards.
	// This could happen in a real network if the main account was re-opened by another close-to transaction which
	// sent closeRewards to the main-account. Yikes.
	receiverBalance, receiverRewards := getReceiverAmounts(test.OpenMainStxn)
	update = state.AlgoUpdates[test.AccountA]
	assertUpdates(t, update, true, receiverBalance, receiverRewards)
}

// TestAssetCloseReopenPay checks that a subround is used when an asset close occurs.
func TestAssetCloseReopenPay(t *testing.T) {
	assetid := uint64(22222)
	amt := uint64(10000)
	_, closeMain := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountA, test.AccountB, test.AccountB)
	_, optinMain := test.MakeAssetTxnOrPanic(test.Round, assetid, 0, test.AccountA, test.AccountA, sdk_types.ZeroAddress)
	_, payMain := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountB, test.AccountA, sdk_types.ZeroAddress)

	state := GetAccounting()
	state.AddTransaction(closeMain)
	state.AddTransaction(optinMain)
	state.AddTransaction(payMain)

	// There should be two subrounds because the first transaction was a close.
	assert.Len(t, state.RoundUpdates.AssetUpdates, 2)

	// Subround 1 has 1 update (the close)
	assert.Len(t, state.RoundUpdates.AssetUpdates[0], 1)
	assert.Nil(t, state.RoundUpdates.AssetUpdates[0][test.AccountA][0].Transfer)
	assert.NotNil(t, state.RoundUpdates.AssetUpdates[0][test.AccountA][0].Close)

	// Subround 2 has 2 updates (debit one account, credit the other)
	assert.Len(t, state.RoundUpdates.AssetUpdates[1], 2)
	assert.Equal(t, state.RoundUpdates.AssetUpdates[1][test.AccountA][0].Transfer.Delta.Int64(), int64(amt))
	assert.Equal(t, state.RoundUpdates.AssetUpdates[1][test.AccountB][0].Transfer.Delta.Int64(), int64(amt)*-1)
}

// TestAssetCloseWithAmount checks that close with an amount creates a delta
func TestAssetCloseWithAmountReopenPay(t *testing.T) {
	assetid := uint64(22222)
	amt := uint64(10000)
	_, closeMain := test.MakeAssetTxnOrPanic(test.Round, assetid, amt, test.AccountA, test.AccountB, test.AccountB)

	state := GetAccounting()
	state.AddTransaction(closeMain)

	// There should be two subrounds because the first transaction was a close.
	assert.Len(t, state.RoundUpdates.AssetUpdates, 2)

	// Subround 1 has 2 updates (one debit, one close)
	assert.Len(t, state.RoundUpdates.AssetUpdates[0], 2)
	assert.Equal(t, int64(amt)*-1, state.RoundUpdates.AssetUpdates[0][test.AccountA][0].Transfer.Delta.Int64())
	assert.NotNil(t, state.RoundUpdates.AssetUpdates[0][test.AccountA][1].Close)

	// Subround 2 is empty
	assert.Len(t, state.RoundUpdates.AssetUpdates[1], 0)
}

// TestCreateAssetWithTotalZero checks that we can create an asset with total = 0
func TestCreateAssetWithTotalZero(t *testing.T) {
	assetid := uint64(2222)
	total := uint64(0)

	///////////
	// Given // Empty state.
	///////////
	state := GetAccounting()

	//////////
	// When // We add a create asset transaction with total = 0.
	//////////
	_, createAsset := test.MakeAssetConfigOrPanic(test.Round, 0, assetid, total, uint64(6), false, "empty-asset", "empty", "http://empty.com", test.AccountA)
	state.AddTransaction(createAsset)

	//////////
	// Then // There should be deltas.
	//////////
	// Config + Transfer
	assert.Len(t, state.RoundUpdates.AssetUpdates, 2)
	assert.True(t, state.RoundUpdates.AssetUpdates[0][test.AccountA][0].Config.IsNew)
	assert.Equal(t, state.RoundUpdates.AssetUpdates[1][test.AccountA][0].Transfer.Delta.Int64(), int64(0))
}
