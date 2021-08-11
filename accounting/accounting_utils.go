package accounting

import (
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
)

// AccountCloseTxn returns whether the transaction closed the given account.
func AccountCloseTxn(addr basics.Address, stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.PaymentTx {
		return false
	}
	return !stxn.Txn.CloseRemainderTo.IsZero() && stxn.Txn.Sender == addr
}

// AssetCreateTxn returns whether the transaction is created an asset.
func AssetCreateTxn(stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.AssetConfigTx {
		return false
	}
	return stxn.Txn.ConfigAsset == 0
}

// AssetDestroyTxn returns whether the transaction is destroys an asset.
func AssetDestroyTxn(stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.AssetConfigTx {
		return false
	}
	return stxn.Txn.AssetParams == (basics.AssetParams{})
}

// AssetOptInTxn returns whether the transaction opted into an asset.
func AssetOptInTxn(stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.AssetTransferTx {
		return false
	}
	return stxn.Txn.AssetAmount == 0 && stxn.Txn.Sender == stxn.Txn.AssetReceiver && stxn.Txn.AssetCloseTo.IsZero()
}

// AssetOptOutTxn returns whether the transaction opted out of an asset.
func AssetOptOutTxn(stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.AssetTransferTx {
		return false
	}
	return !stxn.Txn.AssetCloseTo.IsZero()
}

// AppCreateTxn returns whether the transaction created an application.
func AppCreateTxn(stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.ApplicationCallTx {
		return false
	}

	return stxn.Txn.ApplicationID == 0
}

// AppDestroyTxn returns whether the transaction destroys an application.
func AppDestroyTxn(stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.ApplicationCallTx {
		return false
	}

	return stxn.Txn.OnCompletion == transactions.DeleteApplicationOC
}

// AppOptInTxn returns whether the transaction opts into an application.
func AppOptInTxn(stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.ApplicationCallTx {
		return false
	}

	return stxn.Txn.OnCompletion == transactions.OptInOC
}

// AppOptOutTxn returns whether the transaction opts out of an application.
func AppOptOutTxn(stxn transactions.SignedTxnWithAD) bool {
	if stxn.Txn.Type != protocol.ApplicationCallTx {
		return false
	}

	return stxn.Txn.OnCompletion == transactions.CloseOutOC || stxn.Txn.OnCompletion == transactions.ClearStateOC
}
