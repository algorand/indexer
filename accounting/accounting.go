package accounting

import (
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
)

// GetTransactionParticipants calls function `add` for every address referenced in the
// given transaction, possibly with repetition.
func GetTransactionParticipants(stxnad *transactions.SignedTxnWithAD, includeInner bool, add func(address basics.Address)) {
	txn := &stxnad.Txn

	add(txn.Sender)
	add(txn.Receiver)
	add(txn.CloseRemainderTo)
	add(txn.AssetSender)
	add(txn.AssetReceiver)
	add(txn.AssetCloseTo)
	add(txn.FreezeAccount)

	for _, address := range txn.ApplicationCallTxnFields.Accounts {
		add(address)
	}

	if includeInner {
		for _, inner := range stxnad.ApplyData.EvalDelta.InnerTxns {
			GetTransactionParticipants(&inner, includeInner, add)
		}
	}
}
