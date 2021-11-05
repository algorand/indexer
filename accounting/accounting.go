package accounting

import (
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
)

// GetTransactionParticipants calls function `add` for every address referenced in the
// given transaction, possibly with repetition.
func GetTransactionParticipants(stxnad *transactions.SignedTxnWithAD, includeInner bool, add func(address basics.Address)) {
	txn := &stxnad.Txn

	add(txn.Sender)

	switch txn.Type {
	case protocol.PaymentTx:
		add(txn.Receiver)
		// Close address is optional.
		if !txn.CloseRemainderTo.IsZero() {
			add(txn.CloseRemainderTo)
		}
	case protocol.AssetTransferTx:
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
	case protocol.AssetFreezeTx:
		add(txn.FreezeAccount)
	case protocol.ApplicationCallTx:
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
