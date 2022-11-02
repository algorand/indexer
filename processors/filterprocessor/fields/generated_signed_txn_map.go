// Code generated via go generate. DO NOT EDIT.

package fields

import (
	"fmt"

	"github.com/algorand/go-algorand/data/transactions"
)

// SignedTxnFunc takes a tag and associated SignedTxnInBlock and returns the value
// referenced by the tag.  An error is returned if the tag does not exist
func SignedTxnFunc(tag string, input *transactions.SignedTxnInBlock) (interface{}, error) {

	switch tag {
	case "aca":
		return &input.SignedTxnWithAD.ApplyData.AssetClosingAmount, nil
	case "apid":
		return &input.SignedTxnWithAD.ApplyData.ApplicationID, nil
	case "ca":
		return &input.SignedTxnWithAD.ApplyData.ClosingAmount, nil
	case "caid":
		return &input.SignedTxnWithAD.ApplyData.ConfigAsset, nil
	case "dt":
		return &input.SignedTxnWithAD.ApplyData.EvalDelta, nil
	case "dt.gd":
		return &input.SignedTxnWithAD.ApplyData.EvalDelta.GlobalDelta, nil
	case "dt.itx":
		return &input.SignedTxnWithAD.ApplyData.EvalDelta.InnerTxns, nil
	case "dt.ld":
		return &input.SignedTxnWithAD.ApplyData.EvalDelta.LocalDeltas, nil
	case "dt.lg":
		return &input.SignedTxnWithAD.ApplyData.EvalDelta.Logs, nil
	case "hgh":
		return &input.HasGenesisHash, nil
	case "hgi":
		return &input.HasGenesisID, nil
	case "lsig":
		return &input.SignedTxnWithAD.SignedTxn.Lsig, nil
	case "lsig.arg":
		return &input.SignedTxnWithAD.SignedTxn.Lsig.Args, nil
	case "lsig.l":
		return &input.SignedTxnWithAD.SignedTxn.Lsig.Logic, nil
	case "lsig.msig":
		return &input.SignedTxnWithAD.SignedTxn.Lsig.Msig, nil
	case "lsig.msig.subsig":
		return &input.SignedTxnWithAD.SignedTxn.Lsig.Msig.Subsigs, nil
	case "lsig.msig.thr":
		return &input.SignedTxnWithAD.SignedTxn.Lsig.Msig.Threshold, nil
	case "lsig.msig.v":
		return &input.SignedTxnWithAD.SignedTxn.Lsig.Msig.Version, nil
	case "lsig.sig":
		return &input.SignedTxnWithAD.SignedTxn.Lsig.Sig, nil
	case "msig":
		return &input.SignedTxnWithAD.SignedTxn.Msig, nil
	case "msig.subsig":
		return &input.SignedTxnWithAD.SignedTxn.Msig.Subsigs, nil
	case "msig.thr":
		return &input.SignedTxnWithAD.SignedTxn.Msig.Threshold, nil
	case "msig.v":
		return &input.SignedTxnWithAD.SignedTxn.Msig.Version, nil
	case "rc":
		return &input.SignedTxnWithAD.ApplyData.CloseRewards, nil
	case "rr":
		return &input.SignedTxnWithAD.ApplyData.ReceiverRewards, nil
	case "rs":
		return &input.SignedTxnWithAD.ApplyData.SenderRewards, nil
	case "sgnr":
		return &input.SignedTxnWithAD.SignedTxn.AuthAddr, nil
	case "sig":
		return &input.SignedTxnWithAD.SignedTxn.Sig, nil
	case "txn":
		return &input.SignedTxnWithAD.SignedTxn.Txn, nil
	case "txn.aamt":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetAmount, nil
	case "txn.aclose":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetCloseTo, nil
	case "txn.afrz":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.AssetFrozen, nil
	case "txn.amt":
		return &input.SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount, nil
	case "txn.apaa":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ApplicationArgs, nil
	case "txn.apan":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.OnCompletion, nil
	case "txn.apap":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ApprovalProgram, nil
	case "txn.apar":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams, nil
	case "txn.apar.am":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.MetadataHash, nil
	case "txn.apar.an":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.AssetName, nil
	case "txn.apar.au":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.URL, nil
	case "txn.apar.c":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Clawback, nil
	case "txn.apar.dc":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Decimals, nil
	case "txn.apar.df":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.DefaultFrozen, nil
	case "txn.apar.f":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Freeze, nil
	case "txn.apar.m":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Manager, nil
	case "txn.apar.r":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Reserve, nil
	case "txn.apar.t":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Total, nil
	case "txn.apar.un":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.UnitName, nil
	case "txn.apas":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ForeignAssets, nil
	case "txn.apat":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.Accounts, nil
	case "txn.apbx":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.Boxes, nil
	case "txn.apep":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ExtraProgramPages, nil
	case "txn.apfa":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ForeignApps, nil
	case "txn.apgs":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.GlobalStateSchema, nil
	case "txn.apgs.nbs":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.GlobalStateSchema.NumByteSlice, nil
	case "txn.apgs.nui":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.GlobalStateSchema.NumUint, nil
	case "txn.apid":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ApplicationID, nil
	case "txn.apls":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.LocalStateSchema, nil
	case "txn.apls.nbs":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.LocalStateSchema.NumByteSlice, nil
	case "txn.apls.nui":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.LocalStateSchema.NumUint, nil
	case "txn.apsu":
		return &input.SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ClearStateProgram, nil
	case "txn.arcv":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetReceiver, nil
	case "txn.asnd":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetSender, nil
	case "txn.caid":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.ConfigAsset, nil
	case "txn.close":
		return &input.SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.CloseRemainderTo, nil
	case "txn.fadd":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.FreezeAccount, nil
	case "txn.faid":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.FreezeAsset, nil
	case "txn.fee":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.Fee, nil
	case "txn.fv":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.FirstValid, nil
	case "txn.gen":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.GenesisID, nil
	case "txn.gh":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.GenesisHash, nil
	case "txn.grp":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.Group, nil
	case "txn.lv":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.LastValid, nil
	case "txn.lx":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.Lease, nil
	case "txn.nonpart":
		return &input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.Nonparticipation, nil
	case "txn.note":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.Note, nil
	case "txn.rcv":
		return &input.SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver, nil
	case "txn.rekey":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.RekeyTo, nil
	case "txn.selkey":
		return &input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.SelectionPK, nil
	case "txn.snd":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Header.Sender, nil
	case "txn.sp":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof, nil
	case "txn.sp.P":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs, nil
	case "txn.sp.P.hsh":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.HashFactory, nil
	case "txn.sp.P.hsh.t":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.HashFactory.HashType, nil
	case "txn.sp.P.pth":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.Path, nil
	case "txn.sp.P.td":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.TreeDepth, nil
	case "txn.sp.S":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs, nil
	case "txn.sp.S.hsh":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.HashFactory, nil
	case "txn.sp.S.hsh.t":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.HashFactory.HashType, nil
	case "txn.sp.S.pth":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.Path, nil
	case "txn.sp.S.td":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.TreeDepth, nil
	case "txn.sp.c":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigCommit, nil
	case "txn.sp.pr":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PositionsToReveal, nil
	case "txn.sp.r":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.Reveals, nil
	case "txn.sp.v":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.MerkleSignatureSaltVersion, nil
	case "txn.sp.w":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SignedWeight, nil
	case "txn.spmsg":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message, nil
	case "txn.spmsg.P":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.LnProvenWeight, nil
	case "txn.spmsg.b":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.BlockHeadersCommitment, nil
	case "txn.spmsg.f":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.FirstAttestedRound, nil
	case "txn.spmsg.l":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.LastAttestedRound, nil
	case "txn.spmsg.v":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.VotersCommitment, nil
	case "txn.sprfkey":
		return &input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.StateProofPK, nil
	case "txn.sptype":
		return &input.SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProofType, nil
	case "txn.type":
		return &input.SignedTxnWithAD.SignedTxn.Txn.Type, nil
	case "txn.votefst":
		return &input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteFirst, nil
	case "txn.votekd":
		return &input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteKeyDilution, nil
	case "txn.votekey":
		return &input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VotePK, nil
	case "txn.votelst":
		return &input.SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteLast, nil
	case "txn.xaid":
		return &input.SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.XferAsset, nil
	default:
		return nil, fmt.Errorf("unknown tag: %s", tag)
	}
}
