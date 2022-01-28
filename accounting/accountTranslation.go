package accounting

import (
	"encoding/base64"
	"errors"
	"math"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/indexer/api/generated/v2"
)

// AccountToAccountData converts v2.generated.Account to basics.AccountData
func AccountToAccountData(a *generated.Account) (basics.AccountData, error) {
	var voteID crypto.OneTimeSignatureVerifier
	var selID crypto.VRFVerifier
	var voteFirstValid basics.Round
	var voteLastValid basics.Round
	var voteKeyDilution uint64
	if a.Participation != nil {
		copy(voteID[:], a.Participation.VoteParticipationKey)
		copy(selID[:], a.Participation.SelectionParticipationKey)
		voteFirstValid = basics.Round(a.Participation.VoteFirstValid)
		voteLastValid = basics.Round(a.Participation.VoteLastValid)
		voteKeyDilution = a.Participation.VoteKeyDilution
	}

	var rewardsBase uint64
	if a.RewardBase != nil {
		rewardsBase = *a.RewardBase
	}

	var assetParams map[basics.AssetIndex]basics.AssetParams
	if a.CreatedAssets != nil && len(*a.CreatedAssets) > 0 {
		assetParams = make(map[basics.AssetIndex]basics.AssetParams, len(*a.CreatedAssets))
		var err error
		for _, ca := range *a.CreatedAssets {
			var metadataHash [32]byte
			if ca.Params.MetadataHash != nil {
				copy(metadataHash[:], *ca.Params.MetadataHash)
			}
			var manager, reserve, freeze, clawback basics.Address
			if ca.Params.Manager != nil {
				if manager, err = basics.UnmarshalChecksumAddress(*ca.Params.Manager); err != nil {
					return basics.AccountData{}, err
				}
			}
			if ca.Params.Reserve != nil {
				if reserve, err = basics.UnmarshalChecksumAddress(*ca.Params.Reserve); err != nil {
					return basics.AccountData{}, err
				}
			}
			if ca.Params.Freeze != nil {
				if freeze, err = basics.UnmarshalChecksumAddress(*ca.Params.Freeze); err != nil {
					return basics.AccountData{}, err
				}
			}
			if ca.Params.Clawback != nil {
				if clawback, err = basics.UnmarshalChecksumAddress(*ca.Params.Clawback); err != nil {
					return basics.AccountData{}, err
				}
			}

			var defaultFrozen bool
			if ca.Params.DefaultFrozen != nil {
				defaultFrozen = *ca.Params.DefaultFrozen
			}
			var url string
			if ca.Params.Url != nil {
				url = *ca.Params.Url
			}
			var unitName string
			if ca.Params.UnitName != nil {
				unitName = *ca.Params.UnitName
			}
			var name string
			if ca.Params.Name != nil {
				name = *ca.Params.Name
			}

			assetParams[basics.AssetIndex(ca.Index)] = basics.AssetParams{
				Total:         ca.Params.Total,
				Decimals:      uint32(ca.Params.Decimals),
				DefaultFrozen: defaultFrozen,
				UnitName:      unitName,
				AssetName:     name,
				URL:           url,
				MetadataHash:  metadataHash,
				Manager:       manager,
				Reserve:       reserve,
				Freeze:        freeze,
				Clawback:      clawback,
			}
		}
	}
	var assets map[basics.AssetIndex]basics.AssetHolding
	if a.Assets != nil && len(*a.Assets) > 0 {
		assets = make(map[basics.AssetIndex]basics.AssetHolding, len(*a.Assets))
		for _, h := range *a.Assets {
			assets[basics.AssetIndex(h.AssetId)] = basics.AssetHolding{
				Amount: h.Amount,
				Frozen: h.IsFrozen,
			}
		}
	}

	var appLocalStates map[basics.AppIndex]basics.AppLocalState
	if a.AppsLocalState != nil && len(*a.AppsLocalState) > 0 {
		appLocalStates = make(map[basics.AppIndex]basics.AppLocalState, len(*a.AppsLocalState))
		for _, ls := range *a.AppsLocalState {
			kv, err := convertGeneratedTKV(ls.KeyValue)
			if err != nil {
				return basics.AccountData{}, err
			}
			appLocalStates[basics.AppIndex(ls.Id)] = basics.AppLocalState{
				Schema: basics.StateSchema{
					NumUint:      ls.Schema.NumUint,
					NumByteSlice: ls.Schema.NumByteSlice,
				},
				KeyValue: kv,
			}
		}
	}

	var appParams map[basics.AppIndex]basics.AppParams
	if a.CreatedApps != nil && len(*a.CreatedApps) > 0 {
		appParams = make(map[basics.AppIndex]basics.AppParams, len(*a.CreatedApps))
		for _, params := range *a.CreatedApps {
			ap, err := ApplicationParamsToAppParams(&params.Params)
			if err != nil {
				return basics.AccountData{}, err
			}
			appParams[basics.AppIndex(params.Id)] = ap
		}
	}

	totalSchema := basics.StateSchema{}
	if a.AppsTotalSchema != nil {
		totalSchema.NumUint = a.AppsTotalSchema.NumUint
		totalSchema.NumByteSlice = a.AppsTotalSchema.NumByteSlice
	}

	var totalExtraPages uint32
	if a.AppsTotalExtraPages != nil {
		if *a.AppsTotalExtraPages > math.MaxUint32 {
			return basics.AccountData{}, errors.New("AppsTotalExtraPages exceeds maximum decodable value")
		}
		totalExtraPages = uint32(*a.AppsTotalExtraPages)
	}

	// status, err := basics.UnmarshalStatus(a.Status)
	// if err != nil {
	// 	return basics.AccountData{}, err
	// }

	ad := basics.AccountData{
		// Status:             status,
		MicroAlgos:         basics.MicroAlgos{Raw: a.Amount},
		RewardsBase:        rewardsBase,
		RewardedMicroAlgos: basics.MicroAlgos{Raw: a.Rewards},
		VoteID:             voteID,
		SelectionID:        selID,
		VoteFirstValid:     voteFirstValid,
		VoteLastValid:      voteLastValid,
		VoteKeyDilution:    voteKeyDilution,
		Assets:             assets,
		AppLocalStates:     appLocalStates,
		AppParams:          appParams,
		TotalAppSchema:     totalSchema,
		TotalExtraAppPages: totalExtraPages,
	}

	if a.AuthAddr != nil {
		authAddr, err := basics.UnmarshalChecksumAddress(*a.AuthAddr)
		if err != nil {
			return basics.AccountData{}, err
		}
		ad.AuthAddr = authAddr
	}
	if len(assetParams) > 0 {
		ad.AssetParams = assetParams
	}
	if len(assets) > 0 {
		ad.Assets = assets
	}
	if len(appLocalStates) > 0 {
		ad.AppLocalStates = appLocalStates
	}
	if len(appParams) > 0 {
		ad.AppParams = appParams
	}

	return ad, nil
}

// ApplicationParamsToAppParams converts generated.ApplicationParams to basics.AppParams
func ApplicationParamsToAppParams(gap *generated.ApplicationParams) (basics.AppParams, error) {
	ap := basics.AppParams{
		ApprovalProgram:   gap.ApprovalProgram,
		ClearStateProgram: gap.ClearStateProgram,
	}
	if gap.ExtraProgramPages != nil {
		if *gap.ExtraProgramPages > math.MaxUint32 {
			return basics.AppParams{}, errors.New("ExtraProgramPages exceeds maximum decodable value")
		}
		ap.ExtraProgramPages = uint32(*gap.ExtraProgramPages)
	}
	if gap.LocalStateSchema != nil {
		ap.LocalStateSchema = basics.StateSchema{
			NumUint:      gap.LocalStateSchema.NumUint,
			NumByteSlice: gap.LocalStateSchema.NumByteSlice,
		}
	}
	if gap.GlobalStateSchema != nil {
		ap.GlobalStateSchema = basics.StateSchema{
			NumUint:      gap.GlobalStateSchema.NumUint,
			NumByteSlice: gap.GlobalStateSchema.NumByteSlice,
		}
	}
	kv, err := convertGeneratedTKV(gap.GlobalState)
	if err != nil {
		return basics.AppParams{}, err
	}
	ap.GlobalState = kv

	return ap, nil
}

func convertGeneratedTKV(akvs *generated.TealKeyValueStore) (basics.TealKeyValue, error) {
	if akvs == nil || len(*akvs) == 0 {
		return nil, nil
	}

	tkv := make(basics.TealKeyValue)
	for _, kv := range *akvs {
		// Decode base-64 encoded map key
		decodedKey, err := base64.StdEncoding.DecodeString(kv.Key)
		if err != nil {
			return nil, err
		}

		// Decode base-64 encoded map value (OK even if empty string)
		decodedBytes, err := base64.StdEncoding.DecodeString(kv.Value.Bytes)
		if err != nil {
			return nil, err
		}

		tkv[string(decodedKey)] = basics.TealValue{
			Type:  basics.TealType(kv.Value.Type),
			Uint:  kv.Value.Uint,
			Bytes: string(decodedBytes),
		}
	}
	return tkv, nil
}
