package api

import (
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
)

//////////////////////////////////////////////////////////////////////
// String decoding helpers (with 'errorArr' helper to group errors) //
//////////////////////////////////////////////////////////////////////

// decodeDigest verifies that the digest is valid, then returns the dereferenced input string, or appends an error to errorArr
func decodeDigest(str *string, field string, errorArr []string) (string, []string) {
	if str != nil {
		_, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(*str)
		if err != nil {
			return "", append(errorArr, fmt.Sprintf("%s '%s': %v", errUnableToParseDigest, field, err))
		}
		return *str, errorArr
	}
	// Pass through
	return "", errorArr
}

// decodeAddress returns the byte representation of the input string, or appends an error to errorArr
func decodeAddress(str *string, field string, errorArr []string) ([]byte, []string) {
	if str != nil {
		addr, err := sdk_types.DecodeAddress(*str)
		if err != nil {
			return nil, append(errorArr, fmt.Sprintf("%s '%s': %v", errUnableToParseAddress, field, err))
		}
		return addr[:], errorArr
	}
	// Pass through
	return nil, errorArr
}

// decodeAddress converts the role information into a bitmask, or appends an error to errorArr
func decodeAddressRole(role *string, excludeCloseTo *bool, errorArr []string) (idb.AddressRole, []string) {
	// If the string is nil, return early.
	if role == nil {
		return 0, errorArr
	}

	lc := strings.ToLower(*role)

	if _, ok := addressRoleEnumMap[lc]; !ok {
		return 0, append(errorArr, fmt.Sprintf("%s: '%s'", errUnknownAddressRole, lc))
	}

	exclude := false
	if excludeCloseTo != nil {
		exclude = *excludeCloseTo
	}

	if lc == addrRoleSender {
		return idb.AddressRoleSender | idb.AddressRoleAssetSender, errorArr
	}

	// Receiver + closeTo flags if excludeCloseTo is missing/disabled
	if lc == addrRoleReceiver && !exclude {
		mask := idb.AddressRoleReceiver | idb.AddressRoleAssetReceiver | idb.AddressRoleCloseRemainderTo | idb.AddressRoleAssetCloseTo
		return mask, errorArr
	}

	// closeTo must have been true to get here
	if lc == addrRoleReceiver {
		return idb.AddressRoleReceiver | idb.AddressRoleAssetReceiver, errorArr
	}

	if lc == addrRoleFreeze {
		return idb.AddressRoleFreeze, errorArr
	}

	return 0, append(errorArr, fmt.Sprintf("%s: '%s'", errUnknownAddressRole, lc))
}

const (
	addrRoleSender   = "sender"
	addrRoleReceiver = "receiver"
	addrRoleFreeze   = "freeze-target"
)

var addressRoleEnumMap = map[string]bool{
	addrRoleSender:   true,
	addrRoleReceiver: true,
	addrRoleFreeze:   true,
}

func decodeBase64Byte(str *string, field string, errorArr []string) ([]byte, []string) {
	if str != nil {
		data, err := base64.StdEncoding.DecodeString(*str)
		if err != nil {
			return nil, append(errorArr, fmt.Sprintf("%s: '%s'", errUnableToParseBase64, field))
		}
		return data, errorArr
	}
	return nil, errorArr
}

// decodeSigType validates the input string and dereferences it if present, or appends an error to errorArr
func decodeSigType(str *string, errorArr []string) (idb.SigType, []string) {
	if str != nil {
		sigTypeLc := strings.ToLower(*str)
		sigtype := idb.SigType(*str)
		if idb.IsSigTypeValid(sigtype) {
			return sigtype, errorArr
		}
		return "", append(errorArr, fmt.Sprintf("%s: '%s'", errUnknownSigType, sigTypeLc))
	}
	// Pass through
	return "", errorArr
}

// decodeType validates the input string and dereferences it if present, or appends an error to errorArr
func decodeType(str *string, errorArr []string) (t int, err []string) {
	if str != nil {
		typeLc := strings.ToLower(*str)
		if val, ok := importer.TypeEnumMap[typeLc]; ok {
			return val, errorArr
		}
		return 0, append(errorArr, fmt.Sprintf("%s: '%s'", errUnknownTxType, typeLc))
	}
	// Pass through
	return 0, errorArr
}

////////////////////////////////////////////////////
// Helpers to convert to and from generated types //
////////////////////////////////////////////////////

func sigToTransactionSig(sig sdk_types.Signature) *[]byte {
	if sig == (sdk_types.Signature{}) {
		return nil
	}

	tsig := sig[:]
	return &tsig
}

func msigToTransactionMsig(msig sdk_types.MultisigSig) *generated.TransactionSignatureMultisig {
	if msig.Blank() {
		return nil
	}

	subsigs := make([]generated.TransactionSignatureMultisigSubsignature, 0)
	for _, subsig := range msig.Subsigs {
		subsigs = append(subsigs, generated.TransactionSignatureMultisigSubsignature{
			PublicKey: bytePtr(subsig.Key[:]),
			Signature: sigToTransactionSig(subsig.Sig),
		})
	}

	ret := generated.TransactionSignatureMultisig{
		Subsignature: &subsigs,
		Threshold:    uint64Ptr(uint64(msig.Threshold)),
		Version:      uint64Ptr(uint64(msig.Version)),
	}
	return &ret
}

// TODO: Replace with lsig.Blank() when that gets merged into go-algorand-sdk
func isBlank(lsig sdk_types.LogicSig) bool {
	if lsig.Args != nil {
		return false
	}
	if len(lsig.Logic) != 0 {
		return false
	}
	if !lsig.Msig.Blank() {
		return false
	}
	if lsig.Sig != (sdk_types.Signature{}) {
		return false
	}
	return true
}

func lsigToTransactionLsig(lsig sdk_types.LogicSig) *generated.TransactionSignatureLogicsig {
	if isBlank(lsig) {
		return nil
	}

	args := make([]string, 0)
	for _, arg := range lsig.Args {
		args = append(args, base64.StdEncoding.EncodeToString(arg))
	}

	ret := generated.TransactionSignatureLogicsig{
		Args:              &args,
		Logic:             lsig.Logic,
		MultisigSignature: msigToTransactionMsig(lsig.Msig),
		Signature:         sigToTransactionSig(lsig.Sig),
	}

	return &ret
}

func onCompletionToTransactionOnCompletion(oc sdk_types.OnCompletion) generated.OnCompletion {
	switch oc {
	case sdk_types.NoOpOC:
		return "noop"
	case sdk_types.OptInOC:
		return "optin"
	case sdk_types.CloseOutOC:
		return "closeout"
	case sdk_types.ClearStateOC:
		return "clear"
	case sdk_types.UpdateApplicationOC:
		return "update"
	case sdk_types.DeleteApplicationOC:
		return "delete"
	}
	return "unknown"
}

// The state delta bits need to be sorted for testing. Maybe it would be
// for end users too, people always seem to notice results changing.
func stateDeltaToStateDelta(d types.StateDelta) *generated.StateDelta {
	if len(d) == 0 {
		return nil
	}
	var delta generated.StateDelta
	keys := make([]string, 0)
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := d[k]
		delta = append(delta, generated.EvalDeltaKeyValue{
			Key: base64.StdEncoding.EncodeToString([]byte(k)),
			Value: generated.EvalDelta{
				Action: uint64(v.Action),
				Bytes:  strPtr(base64.StdEncoding.EncodeToString(v.Bytes)),
				Uint:   uint64Ptr(v.Uint),
			},
		})
	}
	return &delta
}

func txnRowToTransaction(row idb.TxnRow) (generated.Transaction, error) {
	if row.Error != nil {
		return generated.Transaction{}, row.Error
	}

	var stxn types.SignedTxnWithAD
	err := msgpack.Decode(row.TxnBytes, &stxn)
	if err != nil {
		return generated.Transaction{}, fmt.Errorf("%s: %s", errUnableToDecodeTransaction, err.Error())
	}

	var payment *generated.TransactionPayment
	var keyreg *generated.TransactionKeyreg
	var assetConfig *generated.TransactionAssetConfig
	var assetFreeze *generated.TransactionAssetFreeze
	var assetTransfer *generated.TransactionAssetTransfer
	var application *generated.TransactionApplication

	switch stxn.Txn.Type {
	case sdk_types.PaymentTx:
		p := generated.TransactionPayment{
			CloseAmount:      uint64Ptr(uint64(stxn.ApplyData.ClosingAmount)),
			CloseRemainderTo: addrPtr(stxn.Txn.CloseRemainderTo),
			Receiver:         stxn.Txn.Receiver.String(),
			Amount:           uint64(stxn.Txn.Amount),
		}
		payment = &p
	case sdk_types.KeyRegistrationTx:
		k := generated.TransactionKeyreg{
			NonParticipation:          boolPtr(stxn.Txn.Nonparticipation),
			SelectionParticipationKey: bytePtr(stxn.Txn.SelectionPK[:]),
			VoteFirstValid:            uint64Ptr(uint64(stxn.Txn.VoteFirst)),
			VoteLastValid:             uint64Ptr(uint64(stxn.Txn.VoteLast)),
			VoteKeyDilution:           uint64Ptr(stxn.Txn.VoteKeyDilution),
			VoteParticipationKey:      bytePtr(stxn.Txn.VotePK[:]),
		}
		keyreg = &k
	case sdk_types.AssetConfigTx:
		assetParams := generated.AssetParams{
			Clawback:      addrPtr(stxn.Txn.AssetParams.Clawback),
			Creator:       stxn.Txn.Sender.String(),
			Decimals:      uint64(stxn.Txn.AssetParams.Decimals),
			DefaultFrozen: boolPtr(stxn.Txn.AssetParams.DefaultFrozen),
			Freeze:        addrPtr(stxn.Txn.AssetParams.Freeze),
			Manager:       addrPtr(stxn.Txn.AssetParams.Manager),
			MetadataHash:  bytePtr(stxn.Txn.AssetParams.MetadataHash[:]),
			Name:          strPtr(stxn.Txn.AssetParams.AssetName),
			Reserve:       addrPtr(stxn.Txn.AssetParams.Reserve),
			Total:         stxn.Txn.AssetParams.Total,
			UnitName:      strPtr(stxn.Txn.AssetParams.UnitName),
			Url:           strPtr(stxn.Txn.AssetParams.URL),
		}
		config := generated.TransactionAssetConfig{
			AssetId: uint64Ptr(uint64(stxn.Txn.ConfigAsset)),
			Params:  &assetParams,
		}
		assetConfig = &config
	case sdk_types.AssetTransferTx:
		t := generated.TransactionAssetTransfer{
			Amount:      stxn.Txn.AssetAmount,
			AssetId:     uint64(stxn.Txn.XferAsset),
			CloseTo:     addrPtr(stxn.Txn.AssetCloseTo),
			Receiver:    stxn.Txn.AssetReceiver.String(),
			Sender:      addrPtr(stxn.Txn.AssetSender),
			CloseAmount: uint64Ptr(row.Extra.AssetCloseAmount),
		}
		assetTransfer = &t
	case sdk_types.AssetFreezeTx:
		f := generated.TransactionAssetFreeze{
			Address:         stxn.Txn.FreezeAccount.String(),
			AssetId:         uint64(stxn.Txn.FreezeAsset),
			NewFreezeStatus: stxn.Txn.AssetFrozen,
		}
		assetFreeze = &f
	case sdk_types.ApplicationCallTx:
		args := make([]string, 0)
		for _, v := range stxn.Txn.ApplicationArgs {
			args = append(args, base64.StdEncoding.EncodeToString(v))
		}

		accts := make([]string, 0)
		for _, v := range stxn.Txn.Accounts {
			accts = append(accts, v.String())
		}

		apps := make([]uint64, 0)
		for _, v := range stxn.Txn.ForeignApps {
			apps = append(apps, uint64(v))
		}

		assets := make([]uint64, 0)
		for _, v := range stxn.Txn.ForeignAssets {
			assets = append(assets, uint64(v))
		}

		a := generated.TransactionApplication{
			Accounts:          &accts,
			ApplicationArgs:   &args,
			ApplicationId:     uint64(stxn.Txn.ApplicationID),
			ApprovalProgram:   bytePtr(stxn.Txn.ApprovalProgram),
			ClearStateProgram: bytePtr(stxn.Txn.ClearStateProgram),
			ForeignApps:       &apps,
			ForeignAssets:     &assets,
			GlobalStateSchema: &generated.StateSchema{
				NumByteSlice: stxn.Txn.GlobalStateSchema.NumByteSlice,
				NumUint:      stxn.Txn.GlobalStateSchema.NumUint,
			},
			LocalStateSchema: &generated.StateSchema{
				NumByteSlice: stxn.Txn.LocalStateSchema.NumByteSlice,
				NumUint:      stxn.Txn.LocalStateSchema.NumUint,
			},
			OnCompletion:      onCompletionToTransactionOnCompletion(stxn.Txn.OnCompletion),
			ExtraProgramPages: uint64PtrOrNil(uint64(stxn.Txn.ExtraProgramPages)),
		}

		application = &a
	}

	sig := generated.TransactionSignature{
		Logicsig: lsigToTransactionLsig(stxn.Lsig),
		Multisig: msigToTransactionMsig(stxn.Msig),
		Sig:      sigToTransactionSig(stxn.Sig),
	}

	var localStateDelta *[]generated.AccountStateDelta
	type tuple struct {
		key     uint64
		address types.Address
	}
	if len(stxn.ApplyData.EvalDelta.LocalDeltas) > 0 {
		keys := make([]tuple, 0)
		for k := range stxn.ApplyData.EvalDelta.LocalDeltas {
			if k == 0 {
				keys = append(keys, tuple{
					key:     0,
					address: stxn.Txn.Sender,
				})
			} else {
				addr := types.Address{}
				copy(addr[:], stxn.Txn.Accounts[k-1][:])
				keys = append(keys, tuple{
					key:     k,
					address: addr,
				})
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].key < keys[j].key })
		d := make([]generated.AccountStateDelta, 0)
		for _, k := range keys {
			v := stxn.ApplyData.EvalDelta.LocalDeltas[k.key]
			delta := stateDeltaToStateDelta(v)
			if delta != nil {
				d = append(d, generated.AccountStateDelta{
					Address: k.address.String(),
					Delta:   *delta,
				})
			}
		}
		localStateDelta = &d
	}

	txn := generated.Transaction{
		ApplicationTransaction:   application,
		AssetConfigTransaction:   assetConfig,
		AssetFreezeTransaction:   assetFreeze,
		AssetTransferTransaction: assetTransfer,
		PaymentTransaction:       payment,
		KeyregTransaction:        keyreg,
		ClosingAmount:            uint64Ptr(uint64(stxn.ClosingAmount)),
		ConfirmedRound:           uint64Ptr(row.Round),
		IntraRoundOffset:         uint64Ptr(uint64(row.Intra)),
		RoundTime:                uint64Ptr(uint64(row.RoundTime.Unix())),
		Fee:                      uint64(stxn.Txn.Fee),
		FirstValid:               uint64(stxn.Txn.FirstValid),
		GenesisHash:              bytePtr(stxn.SignedTxn.Txn.GenesisHash[:]),
		GenesisId:                strPtr(stxn.SignedTxn.Txn.GenesisID),
		Group:                    bytePtr(stxn.Txn.Group[:]),
		LastValid:                uint64(stxn.Txn.LastValid),
		Lease:                    bytePtr(stxn.Txn.Lease[:]),
		Note:                     bytePtr(stxn.Txn.Note[:]),
		Sender:                   stxn.Txn.Sender.String(),
		ReceiverRewards:          uint64Ptr(uint64(stxn.ReceiverRewards)),
		CloseRewards:             uint64Ptr(uint64(stxn.CloseRewards)),
		SenderRewards:            uint64Ptr(uint64(stxn.SenderRewards)),
		TxType:                   string(stxn.Txn.Type),
		Signature:                sig,
		Id:                       crypto.TransactionIDString(stxn.Txn),
		RekeyTo:                  addrPtr(stxn.Txn.RekeyTo),
		GlobalStateDelta:         stateDeltaToStateDelta(stxn.EvalDelta.GlobalDelta),
		LocalStateDelta:          localStateDelta,
	}

	if stxn.Txn.Type == sdk_types.AssetConfigTx {
		if txn.AssetConfigTransaction != nil && txn.AssetConfigTransaction.AssetId != nil && *txn.AssetConfigTransaction.AssetId == 0 {
			txn.CreatedAssetIndex = uint64Ptr(row.AssetID)
		}
	}

	if stxn.Txn.Type == sdk_types.ApplicationCallTx {
		if txn.ApplicationTransaction != nil && txn.ApplicationTransaction.ApplicationId == 0 {
			txn.CreatedApplicationIndex = uint64Ptr(row.AssetID)
		}
	}

	return txn, nil
}

func assetParamsToAssetQuery(params generated.SearchForAssetsParams) (idb.AssetsQuery, error) {
	creator, errorArr := decodeAddress(params.Creator, "creator", make([]string, 0))
	if len(errorArr) != 0 {
		return idb.AssetsQuery{}, errors.New(errUnableToParseAddress)
	}

	var assetGreaterThan uint64 = 0
	if params.Next != nil {
		agt, err := strconv.ParseUint(*params.Next, 10, 64)
		if err != nil {
			return idb.AssetsQuery{}, fmt.Errorf("%s: %v", errUnableToParseNext, err)
		}
		assetGreaterThan = agt
	}

	query := idb.AssetsQuery{
		AssetID:            uintOrDefault(params.AssetId),
		AssetIDGreaterThan: assetGreaterThan,
		Creator:            creator,
		Name:               strOrDefault(params.Name),
		Unit:               strOrDefault(params.Unit),
		Query:              "",
		IncludeDeleted:     boolOrDefault(params.IncludeAll),
		Limit:              min(uintOrDefaultValue(params.Limit, defaultAssetsLimit), maxAssetsLimit),
	}

	return query, nil
}

func transactionParamsToTransactionFilter(params generated.SearchForTransactionsParams) (filter idb.TransactionFilter, err error) {
	var errorArr = make([]string, 0)

	// Round + min/max round
	if params.Round != nil && (params.MaxRound != nil || params.MinRound != nil) {
		errorArr = append(errorArr, errInvalidRoundAndMinMax)
	}

	// If min/max are mixed up
	if params.Round == nil && params.MinRound != nil && params.MaxRound != nil && *params.MinRound > *params.MaxRound {
		errorArr = append(errorArr, errInvalidRoundMinMax)
	}

	// Integer
	filter.MaxRound = uintOrDefault(params.MaxRound)
	filter.MinRound = uintOrDefault(params.MinRound)
	filter.AssetID = uintOrDefault(params.AssetId)
	filter.ApplicationID = uintOrDefault(params.ApplicationId)
	filter.Limit = min(uintOrDefaultValue(params.Limit, defaultTransactionsLimit), maxTransactionsLimit)

	// filter Algos or Asset but not both.
	if filter.AssetID != 0 {
		filter.AssetAmountLT = params.CurrencyLessThan
		filter.AssetAmountGT = params.CurrencyGreaterThan
	} else {
		filter.AlgosLT = params.CurrencyLessThan
		filter.AlgosGT = params.CurrencyGreaterThan
	}
	filter.Round = params.Round

	// String
	filter.AddressRole, errorArr = decodeAddressRole(params.AddressRole, params.ExcludeCloseTo, errorArr)
	filter.NextToken = strOrDefault(params.Next)

	// Address
	filter.Address, errorArr = decodeAddress(params.Address, "address", errorArr)
	filter.Txid, errorArr = decodeDigest(params.Txid, "txid", errorArr)

	// Byte array
	filter.NotePrefix, errorArr = decodeBase64Byte(params.NotePrefix, "note-prefix", errorArr)

	// Time
	if params.AfterTime != nil {
		filter.AfterTime = *params.AfterTime
	}
	if params.BeforeTime != nil {
		filter.BeforeTime = *params.BeforeTime
	}

	// Enum
	filter.SigType, errorArr = decodeSigType(params.SigType, errorArr)
	filter.TypeEnum, errorArr = decodeType(params.TxType, errorArr)

	// Boolean
	filter.RekeyTo = params.RekeyTo

	// If there were any errorArr while setting up the TransactionFilter, return now.
	if len(errorArr) > 0 {
		err = errors.New("invalid input: " + strings.Join(errorArr, ", "))

		// clear out the intermediates.
		filter = idb.TransactionFilter{}
	}

	return
}
