package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/algorand/indexer/api/generated/v2"
)

func MakeStructProcessor(config params) *StructProcessor {
	return &StructProcessor{
	}
}

type StructProcessor struct {
}

func getData(url, token string) ([]byte, error) {
	auth := fmt.Sprintf("Bearer %s", token)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			errorLog.Fatalf("failed to close body: %v", err)
		}
	}()

	return ioutil.ReadAll(resp.Body)
}

func getAccountIndexer(url, token string) (generated.Account, error) {
	data, err := getData(url, token)
	if err != nil {
		return generated.Account{}, err
	}

	var response generated.AccountResponse
	err = json.Unmarshal(data, &response)
	if err != nil {
		return generated.Account{}, err
	}
	return response.Account, nil
}

func getAccountAlgod(url, token string) (generated.Account, error) {
	data, err := getData(url, token)
	if err != nil {
		return generated.Account{}, err
	}

	var response generated.Account
	err = json.Unmarshal(data, &response)
	return response, err
}

func (gp *StructProcessor) ProcessAddress(addr string, config params) error {
	indexerUrl := fmt.Sprintf("%s:/v2/accounts/%s", config.indexerUrl, addr)
	indexerAcct, err := getAccountIndexer(indexerUrl, config.indexerToken)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("unable to lookup indexer acct %s", addr))
	}
	algodUrl := fmt.Sprintf("%s:/v2/accounts/%s", config.algodUrl, addr)
	algodAcct, err := getAccountAlgod(algodUrl, config.algodToken)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("unable to lookup algod acct %s", addr))
	}

	if ! equals(indexerAcct, algodAcct) {
		// report mismatch
		fmt.Println("MISMATCH!")
	}

	return nil
}
func equals(indexer, algod generated.Account) bool {
	// Ignore uninitialized accounts from algod
	if algod.Amount == 0 {
		return true
	}
	if indexer.Deleted != nil && *indexer.Deleted && algod.Amount == 0 {
		return true
	}

	// Ignored fields:
	//   RewardBase
	//   Round
	//   SigType

	if algod.Address != indexer.Address {
		return false
	}
	if algod.Amount != indexer.Amount {
		return false
	}
	if algod.AmountWithoutPendingRewards != indexer.AmountWithoutPendingRewards {
		return false
	}
	if !appSchemaComparePtrEqual(algod.AppsTotalSchema, indexer.AppsTotalSchema) {
		return false
	}
	if algod.AuthAddr != indexer.AuthAddr {
		return false
	}
	if algod.PendingRewards != indexer.PendingRewards {
		return false
	}
	// With and without the pending rewards fix
	if algod.Rewards != indexer.Rewards && algod.Rewards != indexer.Rewards + indexer.PendingRewards {
		return false
	}
	if algod.Status != strings.ReplaceAll(indexer.Status, " ", "") {
		return false
	}

	////////////
	// Assets //
	////////////
	indexerAssets := assetLookupMap(indexer.Assets)
	indexerAssetCount := len(indexerAssets)
	// There should be the same number of undeleted indexer assets as algod assets
	if algod.Assets == nil && indexerAssetCount > 0 || indexerAssetCount != len(*algod.Assets) {
		return false
	}
	if algod.Assets != nil {
		for _, algodAsset := range *algod.Assets {
			// Make sure the asset exists in both results
			indexerAsset, ok := indexerAssets[algodAsset.AssetId]
			if !ok {
				return false
			}

			// Ignored fields:
			//   AssetId (already checked)
			//   Creator
			//   Deleted
			//   OptedInAtRound
			//   OptedOutAtRound

			if algodAsset.Amount != indexerAsset.Amount {
				return false
			}
			if algodAsset.IsFrozen != indexerAsset.IsFrozen {
				return false
			}
		}
	}

	///////////////////
	// CreatedAssets //
	///////////////////
	indexerCreatedAssets := createdAssetLookupMap(indexer.CreatedAssets)
	indexerCreatedAssetCount := len(indexerAssets)
	// There should be the same number of undeleted indexer created assets as algod assets
	if algod.CreatedAssets == nil && indexerCreatedAssetCount > 0 || indexerCreatedAssetCount != len(*algod.CreatedAssets) {
		return false
	}
	if algod.CreatedAssets != nil {
		for _, algodCreatedAsset := range *algod.CreatedAssets {
			// Make sure the asset exists in both results
			indexerCreatedAsset, ok := indexerCreatedAssets[algodCreatedAsset.Index]
			if !ok {
				return false
			}

			// Ignored fields:
			//   Index (already checked)
			//   Deleted
			//   CreatedAtRound
			//   DestroyedAtRound

			if !stringPtrEqual(algodCreatedAsset.Params.Freeze, indexerCreatedAsset.Params.Freeze) {
				return false
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Clawback, indexerCreatedAsset.Params.Clawback) {
				return false
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Manager, indexerCreatedAsset.Params.Manager) {
				return false
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Reserve, indexerCreatedAsset.Params.Reserve) {
				return false
			}
			if !boolPtrEqual(algodCreatedAsset.Params.DefaultFrozen, indexerCreatedAsset.Params.DefaultFrozen) {
				return false
			}
			if algodCreatedAsset.Params.Creator != indexerCreatedAsset.Params.Creator {
				return false
			}
			if algodCreatedAsset.Params.Decimals != indexerCreatedAsset.Params.Decimals {
				return false
			}
			if algodCreatedAsset.Params.Total != indexerCreatedAsset.Params.Total {
				return false
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Name, indexerCreatedAsset.Params.Name) {
				return false
			}
			if !stringPtrEqual(algodCreatedAsset.Params.UnitName, indexerCreatedAsset.Params.UnitName) {
				return false
			}
			if !stringPtrEqual(algodCreatedAsset.Params.Url, indexerCreatedAsset.Params.Url) {
				return false
			}
			if !bytesPtrEqual(algodCreatedAsset.Params.MetadataHash, indexerCreatedAsset.Params.MetadataHash) {
				return false
			}
		}
	}

	////////////////////
	// AppsLocalState //
	////////////////////
	indexerAppLocalState := appLookupMap(indexer.AppsLocalState)
	indexerAppLocalStateCount := len(indexerAppLocalState)
	if algod.AppsLocalState == nil && indexerAppLocalStateCount > 0 || indexerAppLocalStateCount != len(*algod.AppsLocalState) {
		return false
	}
	if algod.AppsLocalState != nil {
		for _, algodAppLocalState := range *algod.AppsLocalState {
			// Make sure the app local state exists in both results
			indexerAppLocalState, ok := indexerAppLocalState[algodAppLocalState.Id]
			if !ok {
				return false
			}

			if algodAppLocalState.Schema != indexerAppLocalState.Schema {
				return false
			}

			if !tealKeyValueStoreEqual(algodAppLocalState.KeyValue, indexerAppLocalState.KeyValue) {
				return false
			}
		}
	}

	/////////////////
	// CreatedApps //
	/////////////////
	indexerCreatedApps := createdAppLookupMap(indexer.CreatedApps)
	indexerCreatedAppsCount := len(indexerCreatedApps)
	if algod.CreatedApps == nil && indexerCreatedAppsCount > 0 || indexerCreatedAppsCount != len(*algod.CreatedApps) {
		return false
	}
	if algod.CreatedApps != nil {
		for _, algodCreatedApp := range *algod.CreatedApps {
			// Make sure the created app exists in both results
			indexerCreatedApp, ok := indexerCreatedApps[algodCreatedApp.Id]
			if !ok {
				return false
			}
			if !stringPtrEqual(algodCreatedApp.Params.Creator, indexerCreatedApp.Params.Creator) {
				return false
			}
			if bytes.Compare(algodCreatedApp.Params.ApprovalProgram, indexerCreatedApp.Params.ApprovalProgram) != 0 {
				return false
			}
			if bytes.Compare(algodCreatedApp.Params.ClearStateProgram, indexerCreatedApp.Params.ClearStateProgram) != 0 {
				return false
			}
			if !tealKeyValueStoreEqual(algodCreatedApp.Params.GlobalState, indexerCreatedApp.Params.GlobalState) {
				return false
			}
			if !stateSchemePtrEqual(algodCreatedApp.Params.LocalStateSchema, indexerCreatedApp.Params.LocalStateSchema) {
				return false
			}
			if !stateSchemePtrEqual(algodCreatedApp.Params.GlobalStateSchema, indexerCreatedApp.Params.GlobalStateSchema) {
				return false
			}
		}
	}

	return true
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
