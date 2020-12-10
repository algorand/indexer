package accounting

import (
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/types"
)

// TODO: Move these to the SDK as helpers on the transaction type

// AccountCloseTxn returns whether the transaction closed the given account.
func AccountCloseTxn(addr sdk_types.Address, stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.PaymentTx {
		return false
	}
	return !stxn.Txn.CloseRemainderTo.IsZero() && stxn.Txn.Sender == addr
}

// AssetCreateTxn returns whether the transaction is created an asset.
func AssetCreateTxn(stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.AssetConfigTx {
		return false
	}
	return stxn.Txn.ConfigAsset == 0
}

// AssetDestroyTxn returns whether the transaction is destroys an asset.
func AssetDestroyTxn(stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.AssetConfigTx {
		return false
	}
	return stxn.Txn.AssetParams.IsZero()
}

// AssetOptInTxn returns whether the transaction opted into an asset.
func AssetOptInTxn(stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.AssetTransferTx {
		return false
	}
	return stxn.Txn.AssetAmount == 0 && stxn.Txn.Sender == stxn.Txn.AssetReceiver && stxn.Txn.AssetCloseTo.IsZero()
}

// AssetOptOutTxn returns whether the transaction opted out of an asset.
func AssetOptOutTxn(stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.AssetTransferTx {
		return false
	}
	return !stxn.Txn.AssetCloseTo.IsZero()
}

// AppCreateTxn returns whether the transaction created an application.
func AppCreateTxn(stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.ApplicationCallTx {
		return false
	}

	return stxn.Txn.ApplicationID == 0
}

// AppDestroyTxn returns whether the transaction destroys an application.
func AppDestroyTxn(stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.ApplicationCallTx {
		return false
	}

	return stxn.Txn.OnCompletion == sdk_types.DeleteApplicationOC
}

// AppOptInTxn returns whether the transaction opts into an application.
func AppOptInTxn(stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.ApplicationCallTx {
		return false
	}

	return stxn.Txn.OnCompletion == sdk_types.OptInOC
}

// AppOptOutTxn returns whether the transaction opts out of an application.
func AppOptOutTxn(stxn types.SignedTxnWithAD) bool {
	if stxn.Txn.Type != sdk_types.ApplicationCallTx {
		return false
	}

	return stxn.Txn.OnCompletion == sdk_types.CloseOutOC || stxn.Txn.OnCompletion == sdk_types.ClearStateOC
}
