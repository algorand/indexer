package accounting

import (
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// GetTransactionParticipants calls function `add` for every address referenced in the
// given transaction, possibly with repetition.
func GetTransactionParticipants(stxnad *sdk.SignedTxnWithAD, includeInner bool, add func(address sdk.Address)) {
	txn := &stxnad.Txn

	add(txn.Sender)

	switch txn.Type {
	case sdk.PaymentTx:
		add(txn.Receiver)
		// Close address is optional.
		if !txn.CloseRemainderTo.IsZero() {
			add(txn.CloseRemainderTo)
		}
	case sdk.AssetTransferTx:
		// If asset sender is non-zero, it is a clawback transaction. Otherwise,
		// the transaction sender address is used.
		if !txn.AssetSender.IsZero() {
			add(txn.AssetSender)
		}
		add(txn.AssetReceiver)
		// Asset close address is optional.
		if !txn.AssetCloseTo.IsZero() {
			add(txn.AssetCloseTo)
		}
	case sdk.AssetFreezeTx:
		add(txn.FreezeAccount)
	case sdk.ApplicationCallTx:
		for _, address := range txn.ApplicationCallTxnFields.Accounts {
			add(address)
		}
	}

	if includeInner {
		for _, inner := range stxnad.ApplyData.EvalDelta.InnerTxns {
			GetTransactionParticipants(&inner, includeInner, add)
		}
	}
}
