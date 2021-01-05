package accounting

import (
	"fmt"
	"testing"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/types"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/mocks"
)

var mainAcct = decodeAddressOrPanic("GJR76Q6OXNZ2CYIVCFCDTJRBAAR6TYEJJENEII3G2U3JH546SPBQA62IFY")
var accountB = decodeAddressOrPanic("N5T74SANUWLHI6ZWYFQBEB6J2VXBTYUYZNWQB2V26DCF4ARKC7GDUW3IRU")
var accountC = decodeAddressOrPanic("OKUWMFFEKF4B4D7FRQYBVV3C2SNS54ZO4WZ2MJ3576UYKFDHM5P3AFMRWE")
var feeAddr = decodeAddressOrPanic("ZROKLZW4GVOK5WQIF2GUR6LHFVEZBMV56BIQEQD4OTIZL2BPSYYUKFBSHM")
var rewardAddr = decodeAddressOrPanic("4C3S3A5II6AYMEADSW7EVL7JAKVU2ASJMMJAGVUROIJHYMS6B24NCXVEWM")

// open / close are configured
var openMainStxn, openMain = makePayTxnRowOrPanic(10, 1000, 10234, 0, 111, 1111, 0, accountC, mainAcct, types.ZeroAddress)
var closeMainToBCStxn, closeMainToBC = makePayTxnRowOrPanic(10, 1000, 1234, 9111, 0, 111, 111, mainAcct, accountC, accountB)

func decodeAddressOrPanic(addr string) types.Address {
	if addr == "" {
		return types.ZeroAddress
	}

	result, err := types.DecodeAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decode address: '%s'", addr))
	}
	return result
}

func makePayTxnRowOrPanic(round, fee, amt, closeAmt, sendRewards, receiveRewards, closeRewards uint64, sender, receiver, close types.Address) (*types.SignedTxnWithAD, *idb.TxnRow) {
	txn := types.SignedTxnWithAD{
		SignedTxn: types.SignedTxn{
			Txn: types.Transaction{
				Type: "pay",
				Header: types.Header{
					Sender:     sender,
					Fee:        types.MicroAlgos(fee),
					FirstValid: types.Round(round),
					LastValid:  types.Round(round),
				},
				PaymentTxnFields: types.PaymentTxnFields{
					Receiver:         receiver,
					Amount:           types.MicroAlgos(amt),
					CloseRemainderTo: close,
				},
			},
		},
		ApplyData: types.ApplyData{
			ClosingAmount:   types.MicroAlgos(closeAmt),
			SenderRewards:   types.MicroAlgos(sendRewards),
			ReceiverRewards: types.MicroAlgos(receiveRewards),
			CloseRewards:    types.MicroAlgos(closeRewards),
		},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: msgpack.Encode(txn),
	}

	return &txn, &txnRow
}

func assertUpdates(t *testing.T, update *idb.AlgoUpdate, closed bool, balance, rewards int64) {
	assert.Equal(t, closed, update.Closed)
	assert.Equal(t, balance, update.Balance)
	assert.Equal(t, rewards, update.Rewards)
}

func getAccounting() *State {
	mockIndexer := &mocks.IndexerDb{}
	accountingState := New(mockIndexer)
	accountingState.feeAddr = feeAddr
	accountingState.rewardAddr = rewardAddr
	accountingState.currentRound = closeMainToBC.Round
	return accountingState
}

func getSenderAmounts(txn *types.SignedTxnWithAD) (balance, rewards int64) {
	balance = -int64(txn.Txn.Amount) + -int64(txn.Txn.Fee) + -int64(txn.ClosingAmount) + int64(txn.SenderRewards)
	rewards = int64(txn.SenderRewards)
	return
}

func getReceiverAmounts(txn *types.SignedTxnWithAD) (balance, rewards int64) {
	balance = int64(txn.Txn.Amount) + int64(txn.ReceiverRewards)
	rewards = int64(txn.ReceiverRewards)
	return
}

func getCloseAmounts(txn *types.SignedTxnWithAD) (balance, rewards int64) {
	balance = int64(txn.ClosingAmount) + int64(txn.CloseRewards)
	rewards = int64(txn.CloseRewards)
	return
}

// TestSimplePayment checks that the accounting deltas are correct for a simple single payment.
func TestSimplePayment(t *testing.T) {
	var update *idb.AlgoUpdate
	state := getAccounting()
	state.AddTransaction(openMain)

	senderBalance, senderRewards := getSenderAmounts(openMainStxn)
	update = state.AlgoUpdates[openMainStxn.Txn.Sender]
	assertUpdates(t, update, false, senderBalance, senderRewards)

	receiverBalance, receiverRewards := getReceiverAmounts(openMainStxn)
	update = state.AlgoUpdates[openMainStxn.Txn.Receiver]
	assertUpdates(t, update, false, receiverBalance, receiverRewards)
}

// TestSimpleAccountClose verifies that the accounting state is set correctly for an account being closed.
func TestSimpleAccountClose(t *testing.T) {
	state := getAccounting()
	state.AddTransaction(closeMainToBC)

	senderBalance, _ := getSenderAmounts(closeMainToBCStxn)
	assertUpdates(t, state.AlgoUpdates[mainAcct], true, senderBalance, 0)

	closeBalance, closeRewards := getCloseAmounts(closeMainToBCStxn)
	assertUpdates(t, state.AlgoUpdates[closeMainToBCStxn.Txn.CloseRemainderTo], false, closeBalance, closeRewards)

	receiverBalance, receiverRewards := getReceiverAmounts(closeMainToBCStxn)
	assertUpdates(t, state.AlgoUpdates[closeMainToBCStxn.Txn.Receiver], false, receiverBalance, receiverRewards)
}

// TestOpenClose verifies that the accounting state is set correctly when an account is opened and closed in the same round.
func TestOpenClose(t *testing.T) {
	var update *idb.AlgoUpdate
	state := getAccounting()
	state.AddTransaction(openMain)
	// In order to make sure rewards were actually reset, they need to be non-zero at this point
	assert.True(t, state.AlgoUpdates[mainAcct].Rewards > 0)
	state.AddTransaction(closeMainToBC)

	// main account balance should add up to 0, and rewards should be reset to 0
	update = state.AlgoUpdates[mainAcct]
	assertUpdates(t, update, true, 0, 0)

	senderBalance, senderRewards := getSenderAmounts(openMainStxn)
	receiverBalance, receiverRewards := getReceiverAmounts(closeMainToBCStxn)
	update = state.AlgoUpdates[openMainStxn.Txn.Sender]
	assertUpdates(t, update, false, senderBalance+receiverBalance, senderRewards+receiverRewards)
}

// TestCloseOpen verifies that the accounting state is set correctly when an account is closed then reopened in the same round.
func TestCloseOpen(t *testing.T) {
	var update *idb.AlgoUpdate
	state := getAccounting()
	state.AddTransaction(openMain)
	state.AddTransaction(closeMainToBC)
	// In the real world this would be a different transaction, but this is a test.
	state.AddTransaction(openMain)

	// The account is closed, but account was closed, but then re-opened and somehow accumulated rewards.
	// This could happen in a real network if the main account was re-opened by another close-to transaction which
	// sent closeRewards to the main-account. Yikes.
	receiverBalance, receiverRewards := getReceiverAmounts(openMainStxn)
	update = state.AlgoUpdates[mainAcct]
	assertUpdates(t, update, true, receiverBalance, receiverRewards)
}
