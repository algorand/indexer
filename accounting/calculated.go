package accounting

import (
	"fmt"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/api/generated/v2"
)

// EnrichMinBalance addes the MinBalance information to a generated.Account by:
// 1. projecting its essential maintenance cost information to an initially empty basics.AccountData bAccount
// 2. calculating bAccount.MinBalance(protocol) using the protocol gotten from a recent blockheader
// 3. adding the result to the generated.Account
// TODO: this could present compatability challenges with rewind() as we need to also rewind the
// MinBalance calculation-logic and protocol along with the account.
func EnrichMinBalance(account *generated.Account, blockheader *bookkeeping.BlockHeader) error {
	proto, ok := config.Consensus[blockheader.CurrentProtocol]
	if !ok {
		return fmt.Errorf("cannot EnrichMinBalance as blockheader's CurrentProtocol is not known to Consensus. CurrentProtocol: %s", blockheader.CurrentProtocol)
	}
	raw := minBalanceProjection(account).MinBalance(&proto).Raw
	account.MinBalance = &raw
	return nil
}

// copy pasta the next few from api::pointer_utils.go and then renamed
// TODO: unify common utils

func boolZerovalIfNil(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}

func uint32ZerovalIfNil(x *uint64) uint32 {
	if x != nil {
		return uint32(*x)
	}
	return uint32(0)
}

func convertAssetHoldings(gAssets *[]generated.AssetHolding) map[basics.AssetIndex]basics.AssetHolding {
	// assumes WYSIWYG ignoring round information except for Deleted which is skipped
	// TODO: handle OptedInAtRound and OptedOutAtRound
	// Make sure to add an integration test case that opts in and out and also back in
	if gAssets == nil {
		return nil
	}
	bAssets := make(map[basics.AssetIndex]basics.AssetHolding)
	for _, gAsset := range *gAssets {
		if boolZerovalIfNil(gAsset.Deleted) {
			continue
		}

		bAssetId := basics.AssetIndex(gAsset.AssetId)
		if _, alreadyExists := bAssets[bAssetId]; alreadyExists {
			// skip dupes
			continue
		}

		// for the purposes if MinBalance calculation, there is no need for Amount field, etc.
		bAssets[bAssetId] = basics.AssetHolding{}
	}
	if len(bAssets) > 0 {
		return bAssets
	}
	return nil
}

func convertAppsCreated(gApps *[]generated.Application) map[basics.AppIndex]basics.AppParams {
	// assumes WYSIWYG ignoring most round information except for Deleted or DeletedAtRound which are skipped
	if gApps == nil {
		return nil
	}
	bAapps := make(map[basics.AppIndex]basics.AppParams)
	for _, gApp := range *gApps {
		if boolZerovalIfNil(gApp.Deleted) || gApp.DeletedAtRound != nil {
			continue
		}
		bAppId := basics.AppIndex(gApp.Id)
		if _, alreadyExists := bAapps[bAppId]; alreadyExists {
			// skip dupes
			continue
		}

		// MinBalance doesn't dig into AppParams, so provide a zeroval struct
		bAapps[bAppId] = basics.AppParams{}
	}
	if len(bAapps) > 0 {
		return bAapps
	}
	return nil
}

func convertAppsOptedIn(gApps *[]generated.ApplicationLocalState) map[basics.AppIndex]basics.AppLocalState {
	// assumes WYSIWYG ignoring most round information except for Deleted or ClosedOutAtRound which are skipped
	if gApps == nil {
		return nil
	}
	bAapps := make(map[basics.AppIndex]basics.AppLocalState)
	for _, gApp := range *gApps {
		if boolZerovalIfNil(gApp.Deleted) || gApp.ClosedOutAtRound != nil {
			continue
		}
		bAppId := basics.AppIndex(gApp.Id)
		if _, alreadyExists := bAapps[bAppId]; alreadyExists {
			// skip dupes
			continue
		}

		// MinBalance doesn't dig into AppLocalStates, so provide a zeroval struct
		bAapps[bAppId] = basics.AppLocalState{}
	}
	if len(bAapps) > 0 {
		return bAapps
	}
	return nil
}

// minBalanceProjection projects a part of a generated.Account to a basics.AccountData struct.
// It does so for the purpose of calculating an account's minumum balance using the official
// calculator go-algorand/data/basics/userBalance.go::MinBalance()
// Some data which is relevant for MinBalance but duplicated is ignored. In particular:
// * TotalAppSchema - gotten from account.AppsTotalSchema and not re-calc'ed from CreatedApps
// * TotalExtraPages - gotten from account.AppsTotalExtraPages and not re-calc'ed from CreatedApps
func minBalanceProjection(account *generated.Account) basics.AccountData {
	if boolZerovalIfNil(account.Deleted) {
		return basics.AccountData{}
	}

	totalAppSchema := basics.StateSchema{}
	if account.AppsTotalSchema != nil {
		totalAppSchema.NumUint = account.AppsTotalSchema.NumUint
		totalAppSchema.NumByteSlice = account.AppsTotalSchema.NumByteSlice
	}
	totalExtraAppPages := uint32(0)
	if account.AppsTotalExtraPages != nil {
		totalExtraAppPages = uint32ZerovalIfNil(account.AppsTotalExtraPages)
	}

	return basics.AccountData{
		Assets:             convertAssetHoldings(account.Assets),
		AppParams:          convertAppsCreated(account.CreatedApps),
		AppLocalStates:     convertAppsOptedIn(account.AppsLocalState),
		TotalAppSchema:     totalAppSchema,
		TotalExtraAppPages: totalExtraAppPages,
	}
}
