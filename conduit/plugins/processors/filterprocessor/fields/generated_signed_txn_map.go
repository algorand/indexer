// Code generated via go generate. DO NOT EDIT.

package fields

import (
	"encoding/base64"
	"fmt"

	"github.com/algorand/go-algorand/data/transactions"
)

// LookupFieldByTag takes a tag and associated SignedTxnInBlock and returns the value
// referenced by the tag.  An error is returned if the tag does not exist
func LookupFieldByTag(tag string, input *transactions.SignedTxnInBlock) (interface{}, error) {
	switch tag {
	case "aca":
		value := input.SignedTxnWithAD.ApplyData.AssetClosingAmount
		return &value, nil
	case "apid":
		value := uint64(input.SignedTxnWithAD.ApplyData.ApplicationID)
		return &value, nil
	case "ca":
		value := uint64(input.SignedTxnWithAD.ApplyData.ClosingAmount.Raw)
		return &value, nil
	case "caid":
		value := uint64(input.SignedTxnWithAD.ApplyData.ConfigAsset)
		return &value, nil
	case "hgh":
		value := fmt.Sprintf("%t", input.HasGenesisHash)
		return &value, nil
	case "hgi":
		value := fmt.Sprintf("%t", input.HasGenesisID)
		return &value, nil
	case "lsig.msig.thr":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Lsig.Msig.Threshold)
		return &value, nil
	case "lsig.msig.v":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Lsig.Msig.Version)
		return &value, nil
	case "msig.thr":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Msig.Threshold)
		return &value, nil
	case "msig.v":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Msig.Version)
		return &value, nil
	case "rc":
		value := uint64(input.SignedTxnWithAD.ApplyData.CloseRewards.Raw)
		return &value, nil
	case "rr":
		value := uint64(input.SignedTxnWithAD.ApplyData.ReceiverRewards.Raw)
		return &value, nil
	case "rs":
		value := uint64(input.SignedTxnWithAD.ApplyData.SenderRewards.Raw)
		return &value, nil
	case "sgnr":
		value := input.SignedTxnWithAD.SignedTxn.AuthAddr.String()
		return &value, nil
	case "txn.aamt":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetAmount
		return &value, nil
	case "txn.aclose":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetCloseTo.String()
		return &value, nil
	case "txn.afrz":
		value := fmt.Sprintf("%t", input.SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.AssetFrozen)
		return &value, nil
	case "txn.amt":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount.Raw)
		return &value, nil
	case "txn.apan":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.OnCompletion)
		return &value, nil
	case "txn.apar.am":
		value := base64.StdEncoding.EncodeToString(input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.MetadataHash[:])
		return &value, nil
	case "txn.apar.an":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.AssetName
		return &value, nil
	case "txn.apar.au":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.URL
		return &value, nil
	case "txn.apar.c":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Clawback.String()
		return &value, nil
	case "txn.apar.dc":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Decimals)
		return &value, nil
	case "txn.apar.df":
		value := fmt.Sprintf("%t", input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.DefaultFrozen)
		return &value, nil
	case "txn.apar.f":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Freeze.String()
		return &value, nil
	case "txn.apar.m":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Manager.String()
		return &value, nil
	case "txn.apar.r":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Reserve.String()
		return &value, nil
	case "txn.apar.t":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Total
		return &value, nil
	case "txn.apar.un":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.UnitName
		return &value, nil
	case "txn.apep":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ExtraProgramPages)
		return &value, nil
	case "txn.apgs.nbs":
		value := input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.GlobalStateSchema.NumByteSlice
		return &value, nil
	case "txn.apgs.nui":
		value := input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.GlobalStateSchema.NumUint
		return &value, nil
	case "txn.apid":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ApplicationID)
		return &value, nil
	case "txn.apls.nbs":
		value := input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.LocalStateSchema.NumByteSlice
		return &value, nil
	case "txn.apls.nui":
		value := input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.LocalStateSchema.NumUint
		return &value, nil
	case "txn.arcv":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetReceiver.String()
		return &value, nil
	case "txn.asnd":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetSender.String()
		return &value, nil
	case "txn.caid":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.ConfigAsset)
		return &value, nil
	case "txn.close":
		value := input.SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.CloseRemainderTo.String()
		return &value, nil
	case "txn.fadd":
		value := input.SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.FreezeAccount.String()
		return &value, nil
	case "txn.faid":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.FreezeAsset)
		return &value, nil
	case "txn.fee":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.Header.Fee.Raw)
		return &value, nil
	case "txn.fv":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.Header.FirstValid)
		return &value, nil
	case "txn.gen":
		value := input.SignedTxnWithAD.SignedTxn.Txn.Header.GenesisID
		return &value, nil
	case "txn.grp":
		value := base64.StdEncoding.EncodeToString(input.SignedTxnWithAD.SignedTxn.Txn.Header.Group[:])
		return &value, nil
	case "txn.lv":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.Header.LastValid)
		return &value, nil
	case "txn.nonpart":
		value := fmt.Sprintf("%t", input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.Nonparticipation)
		return &value, nil
	case "txn.note":
		value := base64.StdEncoding.EncodeToString(input.SignedTxnWithAD.SignedTxn.Txn.Header.Note[:])
		return &value, nil
	case "txn.rcv":
		value := input.SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver.String()
		return &value, nil
	case "txn.rekey":
		value := input.SignedTxnWithAD.SignedTxn.Txn.Header.RekeyTo.String()
		return &value, nil
	case "txn.snd":
		value := input.SignedTxnWithAD.SignedTxn.Txn.Header.Sender.String()
		return &value, nil
	case "txn.sp.P.td":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.TreeDepth)
		return &value, nil
	case "txn.sp.S.td":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.TreeDepth)
		return &value, nil
	case "txn.sp.v":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.MerkleSignatureSaltVersion)
		return &value, nil
	case "txn.sp.w":
		value := input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SignedWeight
		return &value, nil
	case "txn.spmsg.P":
		value := input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.LnProvenWeight
		return &value, nil
	case "txn.spmsg.f":
		value := input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.FirstAttestedRound
		return &value, nil
	case "txn.spmsg.l":
		value := input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.LastAttestedRound
		return &value, nil
	case "txn.sptype":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProofType)
		return &value, nil
	case "txn.type":
		value := string(input.SignedTxnWithAD.SignedTxn.Txn.Type)
		return &value, nil
	case "txn.votefst":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteFirst)
		return &value, nil
	case "txn.votekd":
		value := input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteKeyDilution
		return &value, nil
	case "txn.votelst":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteLast)
		return &value, nil
	case "txn.xaid":
		value := uint64(input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.XferAsset)
		return &value, nil
	default:
		return nil, fmt.Errorf("unknown tag: %s", tag)
	}
}
