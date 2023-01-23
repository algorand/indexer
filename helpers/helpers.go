package helpers

import (
	"encoding/base64"
	"fmt"
	"github.com/pkg/errors"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/algorand/indexer/protocol"
	"github.com/algorand/indexer/types"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	protocol2 "github.com/algorand/go-algorand/protocol"
)

// TODO: remove this file once all types have been converted to sdk types.

// ConvertParams converts basics.AssetParams to sdk.AssetParams
func ConvertParams(params basics.AssetParams) sdk.AssetParams {
	return sdk.AssetParams{
		Total:         params.Total,
		Decimals:      params.Decimals,
		DefaultFrozen: params.DefaultFrozen,
		UnitName:      params.UnitName,
		AssetName:     params.AssetName,
		URL:           params.URL,
		MetadataHash:  params.MetadataHash,
		Manager:       sdk.Address(params.Manager),
		Reserve:       sdk.Address(params.Reserve),
		Freeze:        sdk.Address(params.Freeze),
		Clawback:      sdk.Address(params.Clawback),
	}
}

// ConvertStateDelta is copied from the go-algorand function to build the StateDelta API response.
// Algod function: stateDeltaToLedgerStateDelta.
// Minor modifications were made:
// * dependant functions copied inside the function (presumably we will soon remove this code).
// * 3 arguments to stateDeltaToLEdgerStateDelta are resolved from ledgercore.StateDelta
// https://github.com/algorand/go-algorand/blob/7e37e006370d82644d911acbc803f4e3570b3835/daemon/algod/api/server/v2/delta.go#L74
func ConvertStateDelta(sDelta ledgercore.StateDelta) (models.LedgerStateDelta, error) {
	// In the algod function these are arguments.
	// Hopefully this is close enough for the test cases.
	rewardsLevel := sDelta.Totals.RewardsLevel
	round := sDelta.Hdr.Round
	consensus := config.Consensus[sDelta.Hdr.CurrentProtocol]

	// printableUTF8OrEmpty checks to see if the entire string is a UTF8 printable string.
	// If this is the case, the string is returned as is. Otherwise, the empty string is returned.
	printableUTF8OrEmpty := func(in string) string {
		// iterate throughout all the characters in the string to see if they are all printable.
		// when range iterating on go strings, go decode each element as a utf8 rune.
		for _, c := range in {
			// is this a printable character, or invalid rune ?
			if c == utf8.RuneError || !unicode.IsPrint(c) {
				return ""
			}
		}
		return in
	}

	// More complex helper functions

	convertTKVToGenerated := func(tkv *basics.TealKeyValue) []models.TealKeyValue {
		if tkv == nil || len(*tkv) == 0 {
			return nil
		}

		converted := make([]models.TealKeyValue, 0, len(*tkv))
		rawKeyBytes := make([]string, 0, len(*tkv))
		for k, v := range *tkv {
			converted = append(converted, models.TealKeyValue{
				Key: base64.StdEncoding.EncodeToString([]byte(k)),
				Value: models.TealValue{
					Type:  uint64(v.Type),
					Bytes: base64.StdEncoding.EncodeToString([]byte(v.Bytes)),
					Uint:  v.Uint,
				},
			})
			rawKeyBytes = append(rawKeyBytes, k)
		}
		sort.Slice(converted, func(i, j int) bool {
			return rawKeyBytes[i] < rawKeyBytes[j]
		})
		return converted
	}

	// AppLocalState converts between basics.AppLocalState and model.ApplicationLocalState
	AppLocalState := func(state basics.AppLocalState, appIdx basics.AppIndex) models.ApplicationLocalState {
		localState := convertTKVToGenerated(&state.KeyValue)
		return models.ApplicationLocalState{
			Id:       uint64(appIdx),
			KeyValue: localState,
			Schema: models.ApplicationStateSchema{
				NumByteSlice: state.Schema.NumByteSlice,
				NumUint:      state.Schema.NumUint,
			},
		}
	}

	// AppParamsToApplication converts basics.AppParams to model.Application
	AppParamsToApplication := func(creator string, appIdx basics.AppIndex, appParams *basics.AppParams) models.Application {
		globalState := convertTKVToGenerated(&appParams.GlobalState)
		extraProgramPages := uint64(appParams.ExtraProgramPages)
		app := models.Application{
			Id: uint64(appIdx),
			Params: models.ApplicationParams{
				Creator:           creator,
				ApprovalProgram:   appParams.ApprovalProgram,
				ClearStateProgram: appParams.ClearStateProgram,
				ExtraProgramPages: extraProgramPages,
				GlobalState:       globalState,
				LocalStateSchema: models.ApplicationStateSchema{
					NumByteSlice: appParams.LocalStateSchema.NumByteSlice,
					NumUint:      appParams.LocalStateSchema.NumUint,
				},
				GlobalStateSchema: models.ApplicationStateSchema{
					NumByteSlice: appParams.GlobalStateSchema.NumByteSlice,
					NumUint:      appParams.GlobalStateSchema.NumUint,
				},
			},
		}
		return app
	}

	convertAppResourceRecordToGenerated := func(app ledgercore.AppResourceRecord) models.AppResourceRecord {
		var appLocalState models.ApplicationLocalState
		if app.State.LocalState != nil {
			appLocalState = AppLocalState(*app.State.LocalState, app.Aidx)
		}
		var appParams models.ApplicationParams
		if app.Params.Params != nil {
			appParams = AppParamsToApplication(app.Addr.String(), app.Aidx, app.Params.Params).Params
		}
		return models.AppResourceRecord{
			Address:              app.Addr.String(),
			AppIndex:             uint64(app.Aidx),
			AppDeleted:           app.Params.Deleted,
			AppParams:            appParams,
			AppLocalStateDeleted: app.State.Deleted,
			AppLocalState:        appLocalState,
		}
	}

	AssetParamsToAsset := func(creator string, idx basics.AssetIndex, params *basics.AssetParams) models.Asset {
		frozen := params.DefaultFrozen
		assetParams := models.AssetParams{
			Creator:       creator,
			Total:         params.Total,
			Decimals:      uint64(params.Decimals),
			DefaultFrozen: frozen,
			Name:          printableUTF8OrEmpty(params.AssetName),
			NameB64:       []byte(params.AssetName),
			UnitName:      printableUTF8OrEmpty(params.UnitName),
			UnitNameB64:   []byte(params.UnitName),
			Url:           printableUTF8OrEmpty(params.URL),
			UrlB64:        []byte(params.URL),
			Clawback:      params.Clawback.String(),
			Freeze:        params.Freeze.String(),
			Manager:       params.Manager.String(),
			Reserve:       params.Reserve.String(),
		}
		if params.MetadataHash != ([32]byte{}) {
			metadataHash := make([]byte, len(params.MetadataHash))
			copy(metadataHash, params.MetadataHash[:])
			assetParams.MetadataHash = metadataHash
		}

		return models.Asset{
			Index:  uint64(idx),
			Params: assetParams,
		}
	}

	// AssetHolding converts between basics.AssetHolding and model.AssetHolding
	AssetHolding := func(ah basics.AssetHolding, ai basics.AssetIndex) models.AssetHolding {
		return models.AssetHolding{
			Amount:   ah.Amount,
			AssetId:  uint64(ai),
			IsFrozen: ah.Frozen,
		}
	}

	convertAssetResourceRecordToGenerated := func(asset ledgercore.AssetResourceRecord) models.AssetResourceRecord {
		var assetHolding models.AssetHolding
		if asset.Holding.Holding != nil {
			assetHolding = AssetHolding(*asset.Holding.Holding, asset.Aidx)
		}
		var assetParams models.AssetParams
		if asset.Params.Params != nil {
			a := AssetParamsToAsset(asset.Addr.String(), asset.Aidx, asset.Params.Params)
			assetParams = a.Params
		}
		return models.AssetResourceRecord{
			Address:             asset.Addr.String(),
			AssetIndex:          uint64(asset.Aidx),
			AssetHoldingDeleted: asset.Holding.Deleted,
			AssetHolding:        assetHolding,
			AssetParams:         assetParams,
			AssetDeleted:        asset.Params.Deleted,
		}
	}
	// AccountDataToAccount converts basics.AccountData to v2.model.Account
	AccountDataToAccount := func(
		address string, record *basics.AccountData,
		lastRound basics.Round, consensus *config.ConsensusParams,
		amountWithoutPendingRewards basics.MicroAlgos,
	) (models.Account, error) {

		assets := make([]models.AssetHolding, 0, len(record.Assets))
		for curid, holding := range record.Assets {
			// Empty is ok, asset may have been deleted, so we can no
			// longer fetch the creator
			holding := AssetHolding(holding, curid)

			assets = append(assets, holding)
		}
		sort.Slice(assets, func(i, j int) bool {
			return assets[i].AssetId < assets[j].AssetId
		})

		createdAssets := make([]models.Asset, 0, len(record.AssetParams))
		for idx, params := range record.AssetParams {
			asset := AssetParamsToAsset(address, idx, &params)
			createdAssets = append(createdAssets, asset)
		}
		sort.Slice(createdAssets, func(i, j int) bool {
			return createdAssets[i].Index < createdAssets[j].Index
		})

		var apiParticipation models.AccountParticipation
		if record.VoteID != (crypto.OneTimeSignatureVerifier{}) {
			apiParticipation = models.AccountParticipation{
				VoteParticipationKey:      record.VoteID[:],
				SelectionParticipationKey: record.SelectionID[:],
				VoteFirstValid:            uint64(record.VoteFirstValid),
				VoteLastValid:             uint64(record.VoteLastValid),
				VoteKeyDilution:           uint64(record.VoteKeyDilution),
			}
			if !record.StateProofID.IsEmpty() {
				tmp := record.StateProofID[:]
				apiParticipation.StateProofKey = tmp
			}
		}

		createdApps := make([]models.Application, 0, len(record.AppParams))
		for appIdx, appParams := range record.AppParams {
			app := AppParamsToApplication(address, appIdx, &appParams)
			createdApps = append(createdApps, app)
		}
		sort.Slice(createdApps, func(i, j int) bool {
			return createdApps[i].Id < createdApps[j].Id
		})

		appsLocalState := make([]models.ApplicationLocalState, 0, len(record.AppLocalStates))
		for appIdx, state := range record.AppLocalStates {
			appsLocalState = append(appsLocalState, AppLocalState(state, appIdx))
		}
		sort.Slice(appsLocalState, func(i, j int) bool {
			return appsLocalState[i].Id < appsLocalState[j].Id
		})

		totalAppSchema := models.ApplicationStateSchema{
			NumByteSlice: record.TotalAppSchema.NumByteSlice,
			NumUint:      record.TotalAppSchema.NumUint,
		}
		totalExtraPages := uint64(record.TotalExtraAppPages)

		amount := record.MicroAlgos
		pendingRewards, overflowed := basics.OSubA(amount, amountWithoutPendingRewards)
		if overflowed {
			return models.Account{}, errors.New("overflow on pending reward calculation")
		}

		//minBalance := record.MinBalance(consensus)

		return models.Account{
			SigType:                     "",
			Round:                       uint64(lastRound),
			Address:                     address,
			Amount:                      amount.Raw,
			PendingRewards:              pendingRewards.Raw,
			AmountWithoutPendingRewards: amountWithoutPendingRewards.Raw,
			Rewards:                     record.RewardedMicroAlgos.Raw,
			Status:                      record.Status.String(),
			RewardBase:                  record.RewardsBase,
			Participation:               apiParticipation,
			CreatedAssets:               createdAssets,
			TotalCreatedAssets:          uint64(len(createdAssets)),
			CreatedApps:                 createdApps,
			TotalCreatedApps:            uint64(len(createdApps)),
			Assets:                      assets,
			TotalAssetsOptedIn:          uint64(len(assets)),
			AuthAddr:                    record.AuthAddr.String(),
			AppsLocalState:              appsLocalState,
			TotalAppsOptedIn:            uint64(len(appsLocalState)),
			AppsTotalSchema:             totalAppSchema,
			AppsTotalExtraPages:         totalExtraPages,
			TotalBoxes:                  record.TotalBoxes,
			TotalBoxBytes:               record.TotalBoxBytes,
			//MinBalance:                  minBalance.Raw,
		}, nil
	}

	//////////////////////////////////////////////////////
	// Code below is original with minor modifications. //
	//////////////////////////////////////////////////////
	var response models.LedgerStateDelta

	var accts []models.AccountBalanceRecord
	var apps []models.AppResourceRecord
	var assets []models.AssetResourceRecord
	var keyValues []models.KvDelta
	var modifiedApps []models.ModifiedApp
	var modifiedAssets []models.ModifiedAsset
	var txLeases []models.TxLease

	for key, kvDelta := range sDelta.KvMods {
		var keyBytes = []byte(key)
		keyValues = append(keyValues, models.KvDelta{
			Key:   keyBytes,
			Value: kvDelta.Data,
		})
	}

	for _, record := range sDelta.Accts.Accts {
		var ot basics.OverflowTracker
		pendingRewards := basics.PendingRewards(&ot, consensus, record.MicroAlgos, record.RewardsBase, rewardsLevel)

		amountWithoutPendingRewards, overflowed := basics.OSubA(record.MicroAlgos, pendingRewards)
		if overflowed {
			return response, errors.New("overflow on pending reward calculation")
		}

		ad := basics.AccountData{}
		ledgercore.AssignAccountData(&ad, record.AccountData)
		a, err := AccountDataToAccount(record.Addr.String(), &ad, basics.Round(round), &consensus, amountWithoutPendingRewards)
		if err != nil {
			return response, err
		}

		accts = append(accts, models.AccountBalanceRecord{
			AccountData: a,
			Address:     record.Addr.String(),
		})
	}

	for _, app := range sDelta.Accts.GetAllAppResources() {
		apps = append(apps, convertAppResourceRecordToGenerated(app))
	}

	for _, asset := range sDelta.Accts.GetAllAssetResources() {
		assets = append(assets, convertAssetResourceRecordToGenerated(asset))
	}

	for createIdx, mod := range sDelta.Creatables {
		switch mod.Ctype {
		case basics.AppCreatable:
			modifiedApps = append(modifiedApps, models.ModifiedApp{
				Created: mod.Created,
				Creator: mod.Creator.String(),
				Id:      uint64(createIdx),
			})
		case basics.AssetCreatable:
			modifiedAssets = append(modifiedAssets, models.ModifiedAsset{
				Created: mod.Created,
				Creator: mod.Creator.String(),
				Id:      uint64(createIdx),
			})
		default:
			return response, fmt.Errorf("unable to determine type of creatable for modified creatable with index %d", createIdx)
		}
	}

	for lease, expRnd := range sDelta.Txleases {
		txLeases = append(txLeases, models.TxLease{
			Expiration: uint64(expRnd),
			Lease:      lease.Lease[:],
			Sender:     lease.Sender.String(),
		})
	}

	response = models.LedgerStateDelta{
		Accts: models.AccountDeltas{
			Accounts: accts,
			Apps:     apps,
			Assets:   assets,
		},
		ModifiedApps:   modifiedApps,
		ModifiedAssets: modifiedAssets,
		KvMods:         keyValues,
		PrevTimestamp:  uint64(sDelta.PrevTimestamp),
		StateProofNext: uint64(sDelta.StateProofNext),
		Totals: models.AccountTotals{
			NotParticipating: sDelta.Totals.NotParticipating.Money.Raw,
			Offline:          sDelta.Totals.Offline.Money.Raw,
			Online:           sDelta.Totals.Online.Money.Raw,
			RewardsLevel:     sDelta.Totals.RewardsLevel,
		},
		TxLeases: txLeases,
	}

	return response, nil
}

// ConvertValidatedBlock converts ledgercore.ValidatedBlock to types.ValidatedBlock
func ConvertValidatedBlock(vb ledgercore.ValidatedBlock) (types.ValidatedBlock, error) {
	var ret types.ValidatedBlock
	b64data := base64.StdEncoding.EncodeToString(msgpack.Encode(vb.Block()))
	err := ret.Block.FromBase64String(b64data)
	if err != nil {
		return ret, fmt.Errorf("ConvertValidatedBlock err: %v", err)
	}
	ret.Delta, err = ConvertStateDelta(vb.Delta())
	if err != nil {
		return ret, fmt.Errorf("ConvertStateDelta err: %v", err)
	}
	return ret, nil
}

// ConvertConsensusType returns a go-algorand/protocol.ConsensusVersion type
func ConvertConsensusType(v protocol.ConsensusVersion) protocol2.ConsensusVersion {
	return protocol2.ConsensusVersion(v)
}

// ConvertAccountType returns a basics.AccountData type
func ConvertAccountType(account sdk.Account) basics.AccountData {
	return basics.AccountData{
		Status:          basics.Status(account.Status),
		MicroAlgos:      basics.MicroAlgos{Raw: account.MicroAlgos},
		VoteID:          account.VoteID,
		SelectionID:     account.SelectionID,
		VoteLastValid:   basics.Round(account.VoteLastValid),
		VoteKeyDilution: account.VoteKeyDilution,
	}
}

// ConvertGenesis returns bookkeeping.Genesis
func ConvertGenesis(genesis *sdk.Genesis) (*bookkeeping.Genesis, error) {
	var ret bookkeeping.Genesis
	b := json.Encode(genesis)
	err := json.Decode(b, &ret)
	if err != nil {
		return &ret, fmt.Errorf("ConvertGenesis err: %v", err)
	}
	return &ret, nil
}
