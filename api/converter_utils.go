package api

import (
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"

	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util"
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
		addr, err := basics.UnmarshalChecksumAddress(*str)
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
func decodeType(str *string, errorArr []string) (t idb.TxnTypeEnum, err []string) {
	if str != nil {
		typeLc := protocol.TxType(strings.ToLower(*str))
		if val, ok := idb.GetTypeEnum(typeLc); ok {
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

func sigToTransactionSig(sig crypto.Signature) *[]byte {
	if sig == (crypto.Signature{}) {
		return nil
	}

	tsig := sig[:]
	return &tsig
}

func msigToTransactionMsig(msig crypto.MultisigSig) *generated.TransactionSignatureMultisig {
	if msig.Blank() {
		return nil
	}

	subsigs := make([]generated.TransactionSignatureMultisigSubsignature, 0)
	for _, subsig := range msig.Subsigs {
		subsigs = append(subsigs, generated.TransactionSignatureMultisigSubsignature{
			PublicKey: byteSliceOmitZeroPtr(subsig.Key[:]),
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

func lsigToTransactionLsig(lsig transactions.LogicSig) *generated.TransactionSignatureLogicsig {
	if lsig.Blank() {
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

func onCompletionToTransactionOnCompletion(oc transactions.OnCompletion) generated.OnCompletion {
	switch oc {
	case transactions.NoOpOC:
		return "noop"
	case transactions.OptInOC:
		return "optin"
	case transactions.CloseOutOC:
		return "closeout"
	case transactions.ClearStateOC:
		return "clear"
	case transactions.UpdateApplicationOC:
		return "update"
	case transactions.DeleteApplicationOC:
		return "delete"
	}
	return "unknown"
}

// The state delta bits need to be sorted for testing. Maybe it would be
// for end users too, people always seem to notice results changing.
func stateDeltaToStateDelta(d basics.StateDelta) *generated.StateDelta {
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
				Bytes:  strPtr(base64.StdEncoding.EncodeToString([]byte(v.Bytes))),
				Uint:   uint64Ptr(v.Uint),
			},
		})
	}
	return &delta
}

// rowData is a subset of fields of idb.TxnRow
type rowData struct {
	Round            uint64
	RoundTime        int64
	Intra            uint
	AssetID          uint64
	AssetCloseAmount uint64
}

// txnRowToTransaction parses the idb.TxnRow and generates the appropriate generated.Transaction object.
func txnRowToTransaction(row idb.TxnRow) (generated.Transaction, error) {
	if row.Error != nil {
		return generated.Transaction{}, row.Error
	}

	var stxn *transactions.SignedTxnWithAD
	if row.Txn != nil {
		stxn = row.Txn
	} else if row.RootTxn != nil {
		stxn = row.RootTxn
	} else {
		return generated.Transaction{}, fmt.Errorf("%d:%d transaction bytes missing", row.Round, row.Intra)
	}

	extra := rowData{
		Round:            row.Round,
		RoundTime:        row.RoundTime.Unix(),
		Intra:            uint(row.Intra),
		AssetID:          row.AssetID,
		AssetCloseAmount: row.Extra.AssetCloseAmount,
	}

	if row.Extra.RootIntra.Present {
		extra.Intra = row.Extra.RootIntra.Value
	}

	txn, err := signedTxnWithAdToTransaction(stxn, extra)
	if err != nil {
		return generated.Transaction{}, fmt.Errorf("txnRowToTransaction(): failure converting signed transaction to response: %w", err)
	}

	sig := generated.TransactionSignature{
		Logicsig: lsigToTransactionLsig(stxn.Lsig),
		Multisig: msigToTransactionMsig(stxn.Msig),
		Sig:      sigToTransactionSig(stxn.Sig),
	}

	var txid string
	if row.Extra.RootIntra.Present {
		txid = row.Extra.RootTxid
	} else {
		txid = stxn.Txn.ID().String()
	}

	txn.Id = &txid
	txn.Signature = &sig

	return txn, nil
}

func signedTxnWithAdToTransaction(stxn *transactions.SignedTxnWithAD, extra rowData) (generated.Transaction, error) {
	var payment *generated.TransactionPayment
	var keyreg *generated.TransactionKeyreg
	var assetConfig *generated.TransactionAssetConfig
	var assetFreeze *generated.TransactionAssetFreeze
	var assetTransfer *generated.TransactionAssetTransfer
	var application *generated.TransactionApplication
	var stateProof *generated.TransactionStateProof

	switch stxn.Txn.Type {
	case protocol.PaymentTx:
		p := generated.TransactionPayment{
			CloseAmount:      uint64Ptr(stxn.ApplyData.ClosingAmount.Raw),
			CloseRemainderTo: addrPtr(stxn.Txn.CloseRemainderTo),
			Receiver:         stxn.Txn.Receiver.String(),
			Amount:           stxn.Txn.Amount.Raw,
		}
		payment = &p
	case protocol.KeyRegistrationTx:
		k := generated.TransactionKeyreg{
			NonParticipation:          boolPtr(stxn.Txn.Nonparticipation),
			SelectionParticipationKey: byteSliceOmitZeroPtr(stxn.Txn.SelectionPK[:]),
			VoteFirstValid:            uint64Ptr(uint64(stxn.Txn.VoteFirst)),
			VoteLastValid:             uint64Ptr(uint64(stxn.Txn.VoteLast)),
			VoteKeyDilution:           uint64Ptr(stxn.Txn.VoteKeyDilution),
			VoteParticipationKey:      byteSliceOmitZeroPtr(stxn.Txn.VotePK[:]),
			StateProofKey:             byteSliceOmitZeroPtr(stxn.Txn.StateProofPK[:]),
		}
		keyreg = &k
	case protocol.AssetConfigTx:
		assetParams := generated.AssetParams{
			Clawback:      addrPtr(stxn.Txn.AssetParams.Clawback),
			Creator:       stxn.Txn.Sender.String(),
			Decimals:      uint64(stxn.Txn.AssetParams.Decimals),
			DefaultFrozen: boolPtr(stxn.Txn.AssetParams.DefaultFrozen),
			Freeze:        addrPtr(stxn.Txn.AssetParams.Freeze),
			Manager:       addrPtr(stxn.Txn.AssetParams.Manager),
			MetadataHash:  byteSliceOmitZeroPtr(stxn.Txn.AssetParams.MetadataHash[:]),
			Name:          strPtr(util.PrintableUTF8OrEmpty(stxn.Txn.AssetParams.AssetName)),
			NameB64:       byteSlicePtr([]byte(stxn.Txn.AssetParams.AssetName)),
			Reserve:       addrPtr(stxn.Txn.AssetParams.Reserve),
			Total:         stxn.Txn.AssetParams.Total,
			UnitName:      strPtr(util.PrintableUTF8OrEmpty(stxn.Txn.AssetParams.UnitName)),
			UnitNameB64:   byteSlicePtr([]byte(stxn.Txn.AssetParams.UnitName)),
			Url:           strPtr(util.PrintableUTF8OrEmpty(stxn.Txn.AssetParams.URL)),
			UrlB64:        byteSlicePtr([]byte(stxn.Txn.AssetParams.URL)),
		}
		config := generated.TransactionAssetConfig{
			AssetId: uint64Ptr(uint64(stxn.Txn.ConfigAsset)),
			Params:  &assetParams,
		}
		assetConfig = &config
	case protocol.AssetTransferTx:
		t := generated.TransactionAssetTransfer{
			Amount:      stxn.Txn.AssetAmount,
			AssetId:     uint64(stxn.Txn.XferAsset),
			CloseTo:     addrPtr(stxn.Txn.AssetCloseTo),
			Receiver:    stxn.Txn.AssetReceiver.String(),
			Sender:      addrPtr(stxn.Txn.AssetSender),
			CloseAmount: uint64Ptr(extra.AssetCloseAmount),
		}
		assetTransfer = &t
	case protocol.AssetFreezeTx:
		f := generated.TransactionAssetFreeze{
			Address:         stxn.Txn.FreezeAccount.String(),
			AssetId:         uint64(stxn.Txn.FreezeAsset),
			NewFreezeStatus: stxn.Txn.AssetFrozen,
		}
		assetFreeze = &f
	case protocol.ApplicationCallTx:
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
			ApprovalProgram:   byteSliceOmitZeroPtr(stxn.Txn.ApprovalProgram),
			ClearStateProgram: byteSliceOmitZeroPtr(stxn.Txn.ClearStateProgram),
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
	case protocol.StateProofTx:
		partPath := make([][]byte, len(stxn.Txn.StateProof.PartProofs.Path))
		for idx, part := range stxn.Txn.StateProof.PartProofs.Path {
			digest := make([]byte, len(part))
			copy(digest, part)
			partPath[idx] = digest
		}

		sigProofPath := make([][]byte, len(stxn.Txn.StateProof.SigProofs.Path))
		for idx, sigPart := range stxn.Txn.StateProof.SigProofs.Path {
			digest := make([]byte, len(sigPart))
			copy(digest, sigPart)
			sigProofPath[idx] = digest
		}

		// We need to iterate through these in order, to make sure our responses are deterministic
		keys := make([]uint64, len(stxn.Txn.StateProof.Reveals))
		elems := 0
		for key := range stxn.Txn.StateProof.Reveals {
			keys[elems] = key
			elems++
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		reveals := make([]generated.StateProofReveal, len(stxn.Txn.StateProof.Reveals))
		for i, key := range keys {
			revToConv := stxn.Txn.StateProof.Reveals[key]
			commitment := revToConv.Part.PK.Commitment[:]
			falconSig := []byte(revToConv.SigSlot.Sig.Signature)
			verifyKey := revToConv.SigSlot.Sig.VerifyingKey.PublicKey[:]
			proofPath := make([][]byte, len(revToConv.SigSlot.Sig.Proof.Path))
			for idx, proofPart := range revToConv.SigSlot.Sig.Proof.Path {
				proofPath[idx] = proofPart
			}

			reveals[i] = generated.StateProofReveal{
				Participant: &generated.StateProofParticipant{
					Verifier: &generated.StateProofVerifier{
						Commitment:  &commitment,
						KeyLifetime: uint64Ptr(revToConv.Part.PK.KeyLifetime),
					},
					Weight: uint64Ptr(revToConv.Part.Weight),
				},
				Position: uint64Ptr(key),
				SigSlot: &generated.StateProofSigSlot{
					LowerSigWeight: uint64Ptr(revToConv.SigSlot.L),
					Signature: &generated.StateProofSignature{
						FalconSignature:  &falconSig,
						MerkleArrayIndex: uint64Ptr(revToConv.SigSlot.Sig.VectorCommitmentIndex),
						Proof: &generated.MerkleArrayProof{
							HashFactory: &generated.HashFactory{
								HashType: uint64Ptr(uint64(revToConv.SigSlot.Sig.Proof.HashFactory.HashType)),
							},
							Path:      &proofPath,
							TreeDepth: uint64Ptr(uint64(revToConv.SigSlot.Sig.Proof.TreeDepth)),
						},
						VerifyingKey: &verifyKey,
					},
				},
			}
		}
		proof := generated.StateProofFields{
			PartProofs: &generated.MerkleArrayProof{
				HashFactory: &generated.HashFactory{
					HashType: uint64Ptr(uint64(stxn.Txn.StateProof.PartProofs.HashFactory.HashType)),
				},
				Path:      &partPath,
				TreeDepth: uint64Ptr(uint64(stxn.Txn.StateProof.PartProofs.TreeDepth)),
			},
			Reveals:     &reveals,
			SaltVersion: uint64Ptr(uint64(stxn.Txn.StateProof.MerkleSignatureSaltVersion)),
			SigCommit:   byteSliceOmitZeroPtr(stxn.Txn.StateProof.SigCommit),
			SigProofs: &generated.MerkleArrayProof{
				HashFactory: &generated.HashFactory{
					HashType: uint64Ptr(uint64(stxn.Txn.StateProof.SigProofs.HashFactory.HashType)),
				},
				Path:      &sigProofPath,
				TreeDepth: uint64Ptr(uint64(stxn.Txn.StateProof.SigProofs.TreeDepth)),
			},
			SignedWeight:      uint64Ptr(stxn.Txn.StateProof.SignedWeight),
			PositionsToReveal: &stxn.Txn.StateProof.PositionsToReveal,
		}

		message := generated.IndexerStateProofMessage{
			BlockHeadersCommitment: &stxn.Txn.Message.BlockHeadersCommitment,
			FirstAttestedRound:     uint64Ptr(stxn.Txn.Message.FirstAttestedRound),
			LatestAttestedRound:    uint64Ptr(stxn.Txn.Message.LastAttestedRound),
			LnProvenWeight:         uint64Ptr(stxn.Txn.Message.LnProvenWeight),
			VotersCommitment:       &stxn.Txn.Message.VotersCommitment,
		}

		proofTxn := generated.TransactionStateProof{
			Message:        &message,
			StateProof:     &proof,
			StateProofType: uint64Ptr(uint64(stxn.Txn.StateProofType)),
		}
		stateProof = &proofTxn
	}

	var localStateDelta *[]generated.AccountStateDelta
	type tuple struct {
		key     uint64
		address basics.Address
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
				addr := basics.Address{}
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

	var logs *[][]byte
	if len(stxn.ApplyData.EvalDelta.Logs) > 0 {
		l := make([][]byte, 0, len(stxn.ApplyData.EvalDelta.Logs))
		for _, v := range stxn.ApplyData.EvalDelta.Logs {
			l = append(l, []byte(v))
		}
		logs = &l
	}

	var inners *[]generated.Transaction
	if len(stxn.ApplyData.EvalDelta.InnerTxns) > 0 {
		itxns := make([]generated.Transaction, 0, len(stxn.ApplyData.EvalDelta.InnerTxns))
		for _, t := range stxn.ApplyData.EvalDelta.InnerTxns {
			extra2 := extra
			if t.Txn.Type == protocol.ApplicationCallTx {
				extra2.AssetID = uint64(t.ApplyData.ApplicationID)
			} else if t.Txn.Type == protocol.AssetConfigTx {
				extra2.AssetID = uint64(t.ApplyData.ConfigAsset)
			} else {
				extra2.AssetID = 0
			}
			extra2.AssetCloseAmount = t.ApplyData.AssetClosingAmount

			itxn, err := signedTxnWithAdToTransaction(&t, extra2)
			if err != nil {
				return generated.Transaction{}, err
			}
			itxns = append(itxns, itxn)
		}

		inners = &itxns
	}

	txn := generated.Transaction{
		ApplicationTransaction:   application,
		AssetConfigTransaction:   assetConfig,
		AssetFreezeTransaction:   assetFreeze,
		AssetTransferTransaction: assetTransfer,
		PaymentTransaction:       payment,
		KeyregTransaction:        keyreg,
		StateProofTransaction:    stateProof,
		ClosingAmount:            uint64Ptr(stxn.ClosingAmount.Raw),
		ConfirmedRound:           uint64Ptr(extra.Round),
		IntraRoundOffset:         uint64Ptr(uint64(extra.Intra)),
		RoundTime:                uint64Ptr(uint64(extra.RoundTime)),
		Fee:                      stxn.Txn.Fee.Raw,
		FirstValid:               uint64(stxn.Txn.FirstValid),
		GenesisHash:              byteSliceOmitZeroPtr(stxn.SignedTxn.Txn.GenesisHash[:]),
		GenesisId:                strPtr(stxn.SignedTxn.Txn.GenesisID),
		Group:                    byteSliceOmitZeroPtr(stxn.Txn.Group[:]),
		LastValid:                uint64(stxn.Txn.LastValid),
		Lease:                    byteSliceOmitZeroPtr(stxn.Txn.Lease[:]),
		Note:                     byteSliceOmitZeroPtr(stxn.Txn.Note[:]),
		Sender:                   stxn.Txn.Sender.String(),
		ReceiverRewards:          uint64Ptr(stxn.ReceiverRewards.Raw),
		CloseRewards:             uint64Ptr(stxn.CloseRewards.Raw),
		SenderRewards:            uint64Ptr(stxn.SenderRewards.Raw),
		TxType:                   generated.TransactionTxType(stxn.Txn.Type),
		RekeyTo:                  addrPtr(stxn.Txn.RekeyTo),
		GlobalStateDelta:         stateDeltaToStateDelta(stxn.EvalDelta.GlobalDelta),
		LocalStateDelta:          localStateDelta,
		Logs:                     logs,
		InnerTxns:                inners,
	}

	if stxn.Txn.Type == protocol.AssetConfigTx {
		if txn.AssetConfigTransaction != nil && txn.AssetConfigTransaction.AssetId != nil && *txn.AssetConfigTransaction.AssetId == 0 {
			txn.CreatedAssetIndex = uint64Ptr(extra.AssetID)
		}
	}

	if stxn.Txn.Type == protocol.ApplicationCallTx {
		if txn.ApplicationTransaction != nil && txn.ApplicationTransaction.ApplicationId == 0 {
			txn.CreatedApplicationIndex = uint64Ptr(extra.AssetID)
		}
	}

	return txn, nil
}

func (si *ServerImplementation) assetParamsToAssetQuery(params generated.SearchForAssetsParams) (idb.AssetsQuery, error) {
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
		Limit:              min(uintOrDefaultValue(params.Limit, si.opts.DefaultAssetsLimit), si.opts.MaxAssetsLimit),
	}

	return query, nil
}

func (si *ServerImplementation) appParamsToApplicationQuery(params generated.SearchForApplicationsParams) (idb.ApplicationQuery, error) {
	addr, errorArr := decodeAddress(params.Creator, "creator", make([]string, 0))
	if len(errorArr) != 0 {
		return idb.ApplicationQuery{}, errors.New(errUnableToParseAddress)
	}

	var appGreaterThan uint64 = 0
	if params.Next != nil {
		agt, err := strconv.ParseUint(*params.Next, 10, 64)
		if err != nil {
			return idb.ApplicationQuery{}, fmt.Errorf("%s: %v", errUnableToParseNext, err)
		}
		appGreaterThan = agt
	}

	return idb.ApplicationQuery{
		ApplicationID:            uintOrDefault(params.ApplicationId),
		ApplicationIDGreaterThan: appGreaterThan,
		Address:                  addr,
		IncludeDeleted:           boolOrDefault(params.IncludeAll),
		Limit:                    min(uintOrDefaultValue(params.Limit, si.opts.DefaultApplicationsLimit), si.opts.MaxApplicationsLimit),
	}, nil
}

func (si *ServerImplementation) transactionParamsToTransactionFilter(params generated.SearchForTransactionsParams) (filter idb.TransactionFilter, err error) {
	var errorArr = make([]string, 0)

	// Integer
	filter.MaxRound = uintOrDefault(params.MaxRound)
	filter.MinRound = uintOrDefault(params.MinRound)
	filter.AssetID = uintOrDefault(params.AssetId)
	filter.ApplicationID = uintOrDefault(params.ApplicationId)
	filter.Limit = min(uintOrDefaultValue(params.Limit, si.opts.DefaultTransactionsLimit), si.opts.MaxTransactionsLimit)
	filter.Round = params.Round

	// String
	filter.AddressRole, errorArr = decodeAddressRole((*string)(params.AddressRole), params.ExcludeCloseTo, errorArr)
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
	filter.SigType, errorArr = decodeSigType((*string)(params.SigType), errorArr)
	filter.TypeEnum, errorArr = decodeType((*string)(params.TxType), errorArr)

	// Boolean
	filter.RekeyTo = params.RekeyTo

	// filter Algos or Asset but not both.
	if filter.AssetID != 0 || filter.TypeEnum == idb.TypeEnumAssetTransfer {
		filter.AssetAmountLT = params.CurrencyLessThan
		filter.AssetAmountGT = params.CurrencyGreaterThan
	} else {
		filter.AlgosLT = params.CurrencyLessThan
		filter.AlgosGT = params.CurrencyGreaterThan
	}

	// If there were any errorArr while setting up the TransactionFilter, return now.
	if len(errorArr) > 0 {
		err = errors.New("invalid input: " + strings.Join(errorArr, ", "))

		// clear out the intermediates.
		filter = idb.TransactionFilter{}
	}

	return
}

func (si *ServerImplementation) maxAccountsErrorToAccountsErrorResponse(maxErr idb.MaxAPIResourcesPerAccountError) generated.ErrorResponse {
	addr := maxErr.Address.String()
	max := uint64(si.opts.MaxAPIResourcesPerAccount)
	extraData := map[string]interface{}{
		"max-results":           max,
		"address":               addr,
		"total-assets-opted-in": maxErr.TotalAssets,
		"total-created-assets":  maxErr.TotalAssetParams,
		"total-apps-opted-in":   maxErr.TotalAppLocalStates,
		"total-created-apps":    maxErr.TotalAppParams,
	}
	return generated.ErrorResponse{
		Message: ErrResultLimitReached,
		Data:    &extraData,
	}
}
