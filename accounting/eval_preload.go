package accounting

import (
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/protocol"
)

// Add requests for asset and app creators to `assetsReq` and `appsReq` for the given
// transaction.
func addToCreatorsRequest(stxnad *transactions.SignedTxnWithAD, assetsReq map[basics.AssetIndex]struct{}, appsReq map[basics.AppIndex]struct{}) {
	txn := &stxnad.Txn

	switch txn.Type {
	case protocol.AssetConfigTx:
		fields := &txn.AssetConfigTxnFields
		if fields.ConfigAsset != 0 {
			assetsReq[fields.ConfigAsset] = struct{}{}
		}
	case protocol.AssetTransferTx:
		fields := &txn.AssetTransferTxnFields
		if fields.XferAsset != 0 {
			assetsReq[fields.XferAsset] = struct{}{}
		}
	case protocol.AssetFreezeTx:
		fields := &txn.AssetFreezeTxnFields
		if fields.FreezeAsset != 0 {
			assetsReq[fields.FreezeAsset] = struct{}{}
		}
	case protocol.ApplicationCallTx:
		fields := &txn.ApplicationCallTxnFields
		if fields.ApplicationID != 0 {
			appsReq[fields.ApplicationID] = struct{}{}
		}
		for _, index := range fields.ForeignApps {
			appsReq[index] = struct{}{}
		}
		for _, index := range fields.ForeignAssets {
			assetsReq[index] = struct{}{}
		}
	}

	for i := range stxnad.ApplyData.EvalDelta.InnerTxns {
		addToCreatorsRequest(&stxnad.ApplyData.EvalDelta.InnerTxns[i], assetsReq, appsReq)
	}
}

// MakePreloadCreatorsRequest makes a request for preloading creators in the batch mode.
func MakePreloadCreatorsRequest(payset transactions.Payset) (map[basics.AssetIndex]struct{}, map[basics.AppIndex]struct{}) {
	assetsReq := make(map[basics.AssetIndex]struct{}, len(payset))
	appsReq := make(map[basics.AppIndex]struct{}, len(payset))

	for i := range payset {
		addToCreatorsRequest(&payset[i].SignedTxnWithAD, assetsReq, appsReq)
	}

	return assetsReq, appsReq
}

// Add requests for account data and account resources to `addressesReq` and
// `resourcesReq` respectively for the given transaction.
func addToAccountsResourcesRequest(stxnad *transactions.SignedTxnWithAD, assetCreators map[basics.AssetIndex]ledger.FoundAddress, appCreators map[basics.AppIndex]ledger.FoundAddress, addressesReq map[basics.Address]struct{}, resourcesReq map[basics.Address]map[ledger.Creatable]struct{}) {
	lookupResourcesReq :=
		func(addr basics.Address) map[ledger.Creatable]struct{} {
			c, ok := resourcesReq[addr]
			if ok {
				return c
			}
			c = make(map[ledger.Creatable]struct{})
			resourcesReq[addr] = c
			return c
		}

	txn := &stxnad.Txn

	addressesReq[txn.Sender] = struct{}{}

	switch txn.Type {
	case protocol.PaymentTx:
		fields := &txn.PaymentTxnFields
		addressesReq[fields.Receiver] = struct{}{}
		// Close address is optional.
		if !fields.CloseRemainderTo.IsZero() {
			addressesReq[fields.CloseRemainderTo] = struct{}{}
		}
	case protocol.AssetConfigTx:
		fields := &txn.AssetConfigTxnFields
		if fields.ConfigAsset == 0 {
			if stxnad.ApplyData.ConfigAsset != 0 {
				creatable := ledger.Creatable{
					Index: basics.CreatableIndex(stxnad.ApplyData.ConfigAsset),
					Type:  basics.AssetCreatable,
				}
				lookupResourcesReq(txn.Sender)[creatable] = struct{}{}
			}
		} else {
			if creator := assetCreators[fields.ConfigAsset]; creator.Exists {
				creatable := ledger.Creatable{
					Index: basics.CreatableIndex(fields.ConfigAsset),
					Type:  basics.AssetCreatable,
				}
				addressesReq[creator.Address] = struct{}{}
				lookupResourcesReq(creator.Address)[creatable] = struct{}{}
			}
		}
	case protocol.AssetTransferTx:
		fields := &txn.AssetTransferTxnFields
		creatable := ledger.Creatable{
			Index: basics.CreatableIndex(fields.XferAsset),
			Type:  basics.AssetCreatable,
		}
		if creator := assetCreators[fields.XferAsset]; creator.Exists {
			lookupResourcesReq(creator.Address)[creatable] = struct{}{}
		}
		source := txn.Sender
		// If asset sender is non-zero, it is a clawback transaction. Otherwise,
		// the transaction sender address is used.
		if !fields.AssetSender.IsZero() {
			source = fields.AssetSender
		}
		addressesReq[source] = struct{}{}
		lookupResourcesReq(source)[creatable] = struct{}{}
		addressesReq[fields.AssetReceiver] = struct{}{}
		lookupResourcesReq(fields.AssetReceiver)[creatable] = struct{}{}
		// Asset close address is optional.
		if !fields.AssetCloseTo.IsZero() {
			addressesReq[fields.AssetCloseTo] = struct{}{}
			lookupResourcesReq(fields.AssetCloseTo)[creatable] = struct{}{}
		}
	case protocol.AssetFreezeTx:
		fields := &txn.AssetFreezeTxnFields
		creatable := ledger.Creatable{
			Index: basics.CreatableIndex(fields.FreezeAsset),
			Type:  basics.AssetCreatable,
		}
		if creator := assetCreators[fields.FreezeAsset]; creator.Exists {
			lookupResourcesReq(creator.Address)[creatable] = struct{}{}
		}
		lookupResourcesReq(fields.FreezeAccount)[creatable] = struct{}{}
	case protocol.ApplicationCallTx:
		fields := &txn.ApplicationCallTxnFields
		if fields.ApplicationID == 0 {
			if stxnad.ApplyData.ApplicationID != 0 {
				creatable := ledger.Creatable{
					Index: basics.CreatableIndex(stxnad.ApplyData.ApplicationID),
					Type:  basics.AppCreatable,
				}
				lookupResourcesReq(txn.Sender)[creatable] = struct{}{}
			}
		} else {
			creatable := ledger.Creatable{
				Index: basics.CreatableIndex(fields.ApplicationID),
				Type:  basics.AppCreatable,
			}
			if creator := appCreators[fields.ApplicationID]; creator.Exists {
				addressesReq[creator.Address] = struct{}{}
				lookupResourcesReq(creator.Address)[creatable] = struct{}{}
			}
			lookupResourcesReq(txn.Sender)[creatable] = struct{}{}
		}
		for _, address := range fields.Accounts {
			addressesReq[address] = struct{}{}
		}
		for _, index := range fields.ForeignApps {
			if creator := appCreators[index]; creator.Exists {
				creatable := ledger.Creatable{
					Index: basics.CreatableIndex(index),
					Type:  basics.AppCreatable,
				}
				lookupResourcesReq(creator.Address)[creatable] = struct{}{}
			}
		}
		for _, index := range fields.ForeignAssets {
			if creator := assetCreators[index]; creator.Exists {
				creatable := ledger.Creatable{
					Index: basics.CreatableIndex(index),
					Type:  basics.AssetCreatable,
				}
				lookupResourcesReq(creator.Address)[creatable] = struct{}{}
			}
		}
	}

	for i := range stxnad.ApplyData.EvalDelta.InnerTxns {
		addToAccountsResourcesRequest(
			&stxnad.ApplyData.EvalDelta.InnerTxns[i], assetCreators, appCreators,
			addressesReq, resourcesReq)
	}
}

// MakePreloadAccountsResourcesRequest makes a request for preloading account data and
// account resources in the batch mode.
func MakePreloadAccountsResourcesRequest(payset transactions.Payset, assetCreators map[basics.AssetIndex]ledger.FoundAddress, appCreators map[basics.AppIndex]ledger.FoundAddress) (map[basics.Address]struct{}, map[basics.Address]map[ledger.Creatable]struct{}) {
	addressesReq := make(map[basics.Address]struct{}, len(payset))
	resourcesReq := make(map[basics.Address]map[ledger.Creatable]struct{}, len(payset))

	for i := range payset {
		addToAccountsResourcesRequest(
			&payset[i].SignedTxnWithAD, assetCreators, appCreators, addressesReq, resourcesReq)
	}

	return addressesReq, resourcesReq
}
