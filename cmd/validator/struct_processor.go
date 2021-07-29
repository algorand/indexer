package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/algorand/indexer/api/generated/v2"
)

// StructProcessor implements the process function by serializing API responses into structs and comparing the typed
// objects to each other directly.
type StructProcessor struct {
}

// ProcessAddress is the entrypoint for the StructProcessor
func (gp StructProcessor) ProcessAddress(algodData, indexerData []byte) (Result, error) {
	var indexerResponse generated.AccountResponse
	err := json.Unmarshal(indexerData, &indexerResponse)
	if err != nil {
		return Result{}, fmt.Errorf("unable to parse indexer data: %v", err)
	}

	indexerAcct := indexerResponse.Account
	var algodAcct generated.Account
	err = json.Unmarshal(algodData, &algodAcct)
	if err != nil {
		return Result{}, fmt.Errorf("unable to parse algod data: %v", err)
	}

	differences := equals(indexerAcct, algodAcct)
	if len(differences) > 0 {
		return Result{
			Equal:     false,
			SameRound: indexerAcct.Round == algodAcct.Round,
			Retries:   0,
			Details: &ErrorDetails{
				algod:   mustEncode(algodAcct),
				indexer: mustEncode(indexerAcct),
				diff:    differences,
			},
		}, nil
	}
	return Result{Equal: true}, nil
}

// equals compares the indexer and algod accounts, and returns a slice of differences
func equals(indexer, algod generated.Account) (differences []string) {
	//differences := make([]string, 0)

	// Ignore uninitialized accounts from algod
	if algod.Amount == 0 {
		return
	}
	if indexer.Deleted != nil && *indexer.Deleted && algod.Amount == 0 {
		return
	}

	// Ignored fields:
	//   RewardBase
	//   Round
	//   SigType

	if algod.Address != indexer.Address {
		differences = append(differences, "account address")
	}
	if algod.Amount != indexer.Amount {
		differences = append(differences, fmt.Sprintf("microalgo amount: %d != %d", algod.Amount, indexer.Amount))
	}
	if algod.AmountWithoutPendingRewards != indexer.AmountWithoutPendingRewards {
		differences = append(differences, "amount-without-pending-rewards")
	}
	if !appSchemaComparePtrEqual(algod.AppsTotalSchema, indexer.AppsTotalSchema) {
		differences = append(differences, "apps-total-schema")
	}
	if !uint64PtrEqual(algod.AppsTotalExtraPages, indexer.AppsTotalExtraPages) {
		differences = append(differences, "apps-total-extra-pages")
	}
	if !stringPtrEqual(algod.AuthAddr, indexer.AuthAddr) {
		// Indexer doesn't remove the auth addr when it is removed.
		if indexer.AuthAddr == nil || *indexer.AuthAddr != indexer.Address {
			differences = append(differences, "auth-addr")
		}
	}
	if algod.PendingRewards != indexer.PendingRewards {
		differences = append(differences, "pending-rewards")
	}
	// With and without the pending rewards fix
	if algod.Rewards != indexer.Rewards && algod.Rewards != indexer.Rewards+indexer.PendingRewards {
		differences = append(differences, "rewards (including adjusted)")
	}
	if strings.ReplaceAll(algod.Status, " ", "") != indexer.Status {
		differences = append(differences, "status")
	}

	////////////
	// Assets //
	////////////
	indexerAssets := assetLookupMap(indexer.Assets)
	indexerAssetCount := len(indexerAssets)
	// There should be the same number of undeleted indexer assets as algod assets
	if algod.Assets == nil && indexerAssetCount > 0 || indexerAssetCount != len(*algod.Assets) {
		differences = append(differences, "assets")
	}
	if algod.Assets != nil {
		for _, algodAsset := range *algod.Assets {
			// Make sure the asset exists in both results
			indexerAsset, ok := indexerAssets[algodAsset.AssetId]
			if !ok {
				differences = append(differences, fmt.Sprintf("missing asset %d", algodAsset.AssetId))
			}

			// Ignored fields:
			//   AssetId (already checked)
			//   Creator
			//   Deleted
			//   OptedInAtRound
			//   OptedOutAtRound

			if algodAsset.Amount != indexerAsset.Amount {
				differences = append(differences, fmt.Sprintf("asset amount %d: %d != %d", algodAsset.AssetId, algodAsset.Amount, indexerAsset.Amount))
			}
			if algodAsset.IsFrozen != indexerAsset.IsFrozen {
				differences = append(differences, fmt.Sprintf("asset is-frozen %d", algodAsset.AssetId))
			}
		}
	}

	///////////////////
	// CreatedAssets //
	///////////////////
	indexerCreatedAssets := createdAssetLookupMap(indexer.CreatedAssets)
	indexerCreatedAssetCount := len(indexerCreatedAssets)
	// There should be the same number of undeleted indexer created assets as algod assets
	if algod.CreatedAssets == nil && indexerCreatedAssetCount > 0 || indexerCreatedAssetCount != len(*algod.CreatedAssets) {
		differences = append(differences, "created-assets")
	}
	if algod.CreatedAssets != nil {
		for _, algodCreatedAsset := range *algod.CreatedAssets {
			// Make sure the asset exists in both results
			indexerCreatedAsset, ok := indexerCreatedAssets[algodCreatedAsset.Index]
			if !ok {
				differences = append(differences, fmt.Sprintf("missing created-asset %d", algodCreatedAsset.Index))
			}

			// Ignored fields:
			//   Index (already checked)
			//   Deleted
			//   CreatedAtRound
			//   DestroyedAtRound

			if !stringPtrEqual(algodCreatedAsset.Params.Freeze, indexerCreatedAsset.Params.Freeze) {
				differences = append(differences, fmt.Sprintf("created-asset freeze-addr %d", algodCreatedAsset.Index))
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Clawback, indexerCreatedAsset.Params.Clawback) {
				differences = append(differences, fmt.Sprintf("created-asset clawback-addr %d", algodCreatedAsset.Index))
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Manager, indexerCreatedAsset.Params.Manager) {
				differences = append(differences, fmt.Sprintf("created-asset manager-addr %d", algodCreatedAsset.Index))
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Reserve, indexerCreatedAsset.Params.Reserve) {
				differences = append(differences, fmt.Sprintf("created-asset reserve-addr %d", algodCreatedAsset.Index))
			}
			if !boolPtrEqual(algodCreatedAsset.Params.DefaultFrozen, indexerCreatedAsset.Params.DefaultFrozen) {
				differences = append(differences, fmt.Sprintf("created-asset default-frozen %d", algodCreatedAsset.Index))
			}
			if algodCreatedAsset.Params.Creator != indexerCreatedAsset.Params.Creator {
				differences = append(differences, fmt.Sprintf("created-asset creator-addr %d", algodCreatedAsset.Index))
			}
			if algodCreatedAsset.Params.Decimals != indexerCreatedAsset.Params.Decimals {
				differences = append(differences, fmt.Sprintf("created-asset decimals %d", algodCreatedAsset.Index))
			}
			if algodCreatedAsset.Params.Total != indexerCreatedAsset.Params.Total {
				differences = append(differences, fmt.Sprintf("created-asset total %d", algodCreatedAsset.Index))
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Name, indexerCreatedAsset.Params.Name) {
				differences = append(differences, fmt.Sprintf("created-asset name %d", algodCreatedAsset.Index))
			}
			if !stringPtrEqual(algodCreatedAsset.Params.UnitName, indexerCreatedAsset.Params.UnitName) {
				differences = append(differences, fmt.Sprintf("created-asset unit-name %d", algodCreatedAsset.Index))
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Url, indexerCreatedAsset.Params.Url) {
				differences = append(differences, fmt.Sprintf("created-asset url %d", algodCreatedAsset.Index))
			}
			if !bytesPtrEqual(algodCreatedAsset.Params.NameB64, indexerCreatedAsset.Params.NameB64) {
				differences = append(differences, fmt.Sprintf("created-asset name-b64 %d", algodCreatedAsset.Index))
			}
			if !bytesPtrEqual(algodCreatedAsset.Params.UnitNameB64, indexerCreatedAsset.Params.UnitNameB64) {
				differences = append(differences, fmt.Sprintf("created-asset unit-name-b64 %d", algodCreatedAsset.Index))
			}
			if !bytesPtrEqual(algodCreatedAsset.Params.UrlB64, indexerCreatedAsset.Params.UrlB64) {
				differences = append(differences, fmt.Sprintf("created-asset url-b64 %d", algodCreatedAsset.Index))
			}
			if !bytesPtrEqual(algodCreatedAsset.Params.MetadataHash, indexerCreatedAsset.Params.MetadataHash) {
				differences = append(differences, fmt.Sprintf("created-asset metadata-hash %d", algodCreatedAsset.Index))
			}
		}
	}

	////////////////////
	// AppsLocalState //
	////////////////////
	indexerAppLocalState := appLookupMap(indexer.AppsLocalState)
	indexerAppLocalStateCount := len(indexerAppLocalState)
	if algod.AppsLocalState == nil && indexerAppLocalStateCount > 0 || indexerAppLocalStateCount != len(*algod.AppsLocalState) {
		differences = append(differences, "apps-local-state")
	}
	if algod.AppsLocalState != nil {
		for _, algodAppLocalState := range *algod.AppsLocalState {
			// Make sure the app local state exists in both results
			indexerAppLocalState, ok := indexerAppLocalState[algodAppLocalState.Id]
			if !ok {
				differences = append(differences, fmt.Sprintf("missing app-local-state %d", algodAppLocalState.Id))
			}

			if algodAppLocalState.Schema != indexerAppLocalState.Schema {
				differences = append(differences, fmt.Sprintf("app-local-state schema %d", algodAppLocalState.Id))
			}

			if !tealKeyValueStoreEqual(algodAppLocalState.KeyValue, indexerAppLocalState.KeyValue) {
				differences = append(differences, fmt.Sprintf("app-local-state key-values %d", algodAppLocalState.Id))
			}
		}
	}

	/////////////////
	// CreatedApps //
	/////////////////
	indexerCreatedApps := createdAppLookupMap(indexer.CreatedApps)
	indexerCreatedAppsCount := len(indexerCreatedApps)
	if algod.CreatedApps == nil && indexerCreatedAppsCount > 0 || indexerCreatedAppsCount != len(*algod.CreatedApps) {
		differences = append(differences, "created-apps")
	}
	if algod.CreatedApps != nil {
		for _, algodCreatedApp := range *algod.CreatedApps {
			// Make sure the created app exists in both results
			indexerCreatedApp, ok := indexerCreatedApps[algodCreatedApp.Id]
			if !ok {
				differences = append(differences, fmt.Sprintf("missing created-app %d", algodCreatedApp.Id))
			}
			if !stringPtrEqual(algodCreatedApp.Params.Creator, indexerCreatedApp.Params.Creator) {
				differences = append(differences, fmt.Sprintf("created-app creator %d", algodCreatedApp.Id))
			}
			if bytes.Compare(algodCreatedApp.Params.ApprovalProgram, indexerCreatedApp.Params.ApprovalProgram) != 0 {
				differences = append(differences, fmt.Sprintf("created-app approval-program %d", algodCreatedApp.Id))
			}
			if bytes.Compare(algodCreatedApp.Params.ClearStateProgram, indexerCreatedApp.Params.ClearStateProgram) != 0 {
				differences = append(differences, fmt.Sprintf("created-app clear-state-program %d", algodCreatedApp.Id))
			}
			if !tealKeyValueStoreEqual(algodCreatedApp.Params.GlobalState, indexerCreatedApp.Params.GlobalState) {
				differences = append(differences, fmt.Sprintf("created-app global-state %d", algodCreatedApp.Id))
			}
			if !stateSchemePtrEqual(algodCreatedApp.Params.LocalStateSchema, indexerCreatedApp.Params.LocalStateSchema) {
				differences = append(differences, fmt.Sprintf("created-app local-state-schema %d", algodCreatedApp.Id))
			}
			if !stateSchemePtrEqual(algodCreatedApp.Params.GlobalStateSchema, indexerCreatedApp.Params.GlobalStateSchema) {
				differences = append(differences, fmt.Sprintf("created-app global-state-schema %d", algodCreatedApp.Id))
			}
			if !uint64PtrEqual(algodCreatedApp.Params.ExtraProgramPages, indexerCreatedApp.Params.ExtraProgramPages) {
				differences = append(differences, fmt.Sprintf("created-app extra-pages %d", algodCreatedApp.Id))
			}
		}
	}

	return
}

func appSchemaComparePtrEqual(val1, val2 *generated.ApplicationStateSchema) bool {
	// both nil
	if val1 == val2 {
		return true
	}
	if val1 == nil {
		val1 = &generated.ApplicationStateSchema{}
	}
	if val2 == nil {
		val2 = &generated.ApplicationStateSchema{}
	}
	return *val1 == *val2
}

func tealKeyValueStoreEqual(val1, val2 *generated.TealKeyValueStore) bool {
	v1Map := keyValueLookupMap(val1)
	v2Map := keyValueLookupMap(val2)
	if len(v1Map) != len(v2Map) {
		return false
	}
	for k, v1Val := range v1Map {
		v2Val, ok := v2Map[k]
		if !ok {
			return false
		}
		if v1Val != v2Val {
			return false
		}
	}
	return true
}

func stateSchemePtrEqual(val1, val2 *generated.ApplicationStateSchema) bool {
	// both nil
	if val1 == val2 {
		return true
	}
	// one not nil
	if val1 == nil || val2 == nil {
		return false
	}

	return *val1 == *val2
}

func bytesPtrEqual(val1, val2 *[]uint8) bool {
	// both nil
	if val1 == val2 {
		return true
	}
	// one not nil
	if val1 == nil || val2 == nil {
		return false
	}

	return bytes.Compare(*val1, *val2) == 0
}

func boolPtrEqual(val1, val2 *bool) bool {
	// both nil
	if val1 == val2 {
		return true
	}
	// one not nil
	if val1 == nil || val2 == nil {
		return false
	}

	return *val1 == *val2
}

func stringPtrEqual(val1, val2 *string) bool {
	// both nil
	if val1 == val2 {
		return true
	}
	// one not nil
	if val1 == nil || val2 == nil {
		return false
	}

	return *val1 == *val2
}

func uint64PtrEqual(val1, val2 *uint64) bool {
	// both nil
	if val1 == val2 {
		return true
	}
	// one not nil
	if val1 == nil || val2 == nil {
		return false
	}

	return *val1 == *val2
}

func keyValueLookupMap(kvstore *generated.TealKeyValueStore) map[string]generated.TealValue {
	result := make(map[string]generated.TealValue)
	if kvstore == nil {
		return result
	}
	for _, kv := range *kvstore {
		result[kv.Key] = kv.Value
	}

	return result
}

func assetLookupMap(assets *[]generated.AssetHolding) map[uint64]generated.AssetHolding {
	result := make(map[uint64]generated.AssetHolding)
	if assets == nil {
		return result
	}
	for _, asset := range *assets {
		// Don't add deleted assets
		if asset.Deleted == nil || !*asset.Deleted {
			result[asset.AssetId] = asset
		}
	}
	return result
}

func createdAssetLookupMap(assets *[]generated.Asset) map[uint64]generated.Asset {
	result := make(map[uint64]generated.Asset)
	if assets == nil {
		return result
	}
	for _, asset := range *assets {
		// Don't add deleted assets
		if asset.Deleted == nil || !*asset.Deleted {
			result[asset.Index] = asset
		}
	}
	return result
}

func appLookupMap(apps *[]generated.ApplicationLocalState) map[uint64]generated.ApplicationLocalState {
	result := make(map[uint64]generated.ApplicationLocalState)
	if apps == nil {
		return result
	}
	for _, app := range *apps {
		// Don't add deleted apps
		if app.Deleted == nil || !*app.Deleted {
			result[app.Id] = app
		}
	}
	return result
}

func createdAppLookupMap(apps *[]generated.Application) map[uint64]generated.Application {
	result := make(map[uint64]generated.Application)
	if apps == nil {
		return result
	}
	for _, app := range *apps {
		// Don't add deleted apps
		if app.Deleted == nil || !*app.Deleted {
			result[app.Id] = app
		}
	}
	return result
}
