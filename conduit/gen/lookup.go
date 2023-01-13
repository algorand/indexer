// Code generated via go generate. DO NOT EDIT.
package fields

import "github.com/algorand/go-algorand/data/transactions"

// SignedTxnMap generates a map with the key as the codec tag and the value as a function
// that returns the associated variable
var SignedTxnMap = map[string]func(*transactions.SignedTxnInBlock) interface{}{
	"aca": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.AssetClosingAmount)
	},
	"apid": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.ApplicationID)
	},
	"ca": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.ClosingAmount)
	},
	"caid": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.ConfigAsset)
	},
	"dt": func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.ApplyData.EvalDelta) },
	"dt.gd": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.EvalDelta.GlobalDelta)
	},
	"dt.itx": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.EvalDelta.InnerTxns)
	},
	"dt.ld": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.EvalDelta.LocalDeltas)
	},
	"dt.lg": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.EvalDelta.Logs)
	},
	"hgh":      func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).HasGenesisHash) },
	"hgi":      func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).HasGenesisID) },
	"lsig":     func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.Lsig) },
	"lsig.arg": func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.Lsig.Args) },
	"lsig.l": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Lsig.Logic)
	},
	"lsig.msig": func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.Lsig.Msig) },
	"lsig.msig.subsig": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Lsig.Msig.Subsigs)
	},
	"lsig.msig.thr": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Lsig.Msig.Threshold)
	},
	"lsig.msig.v": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Lsig.Msig.Version)
	},
	"lsig.sig": func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.Lsig.Sig) },
	"msig":     func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.Msig) },
	"msig.subsig": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Msig.Subsigs)
	},
	"msig.thr": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Msig.Threshold)
	},
	"msig.v": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Msig.Version)
	},
	"rc": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.CloseRewards)
	},
	"rr": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.ReceiverRewards)
	},
	"rs": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.ApplyData.SenderRewards)
	},
	"sgnr": func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.AuthAddr) },
	"sig":  func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.Sig) },
	"txn":  func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.Txn) },
	"txn.aamt": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetAmount)
	},
	"txn.aclose": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetCloseTo)
	},
	"txn.afrz": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.AssetFrozen)
	},
	"txn.amt": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Amount)
	},
	"txn.apaa": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ApplicationArgs)
	},
	"txn.apan": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.OnCompletion)
	},
	"txn.apap": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ApprovalProgram)
	},
	"txn.apar": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams)
	},
	"txn.apar.am": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.MetadataHash)
	},
	"txn.apar.an": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.AssetName)
	},
	"txn.apar.au": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.URL)
	},
	"txn.apar.c": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Clawback)
	},
	"txn.apar.dc": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Decimals)
	},
	"txn.apar.df": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.DefaultFrozen)
	},
	"txn.apar.f": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Freeze)
	},
	"txn.apar.m": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Manager)
	},
	"txn.apar.r": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Reserve)
	},
	"txn.apar.t": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.Total)
	},
	"txn.apar.un": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.AssetParams.UnitName)
	},
	"txn.apas": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ForeignAssets)
	},
	"txn.apat": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.Accounts)
	},
	"txn.apep": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ExtraProgramPages)
	},
	"txn.apfa": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ForeignApps)
	},
	"txn.apgs": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.GlobalStateSchema)
	},
	"txn.apgs.nbs": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.GlobalStateSchema.NumByteSlice)
	},
	"txn.apgs.nui": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.GlobalStateSchema.NumUint)
	},
	"txn.apid": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ApplicationID)
	},
	"txn.apls": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.LocalStateSchema)
	},
	"txn.apls.nbs": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.LocalStateSchema.NumByteSlice)
	},
	"txn.apls.nui": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.LocalStateSchema.NumUint)
	},
	"txn.apsu": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.ApplicationCallTxnFields.ClearStateProgram)
	},
	"txn.arcv": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetReceiver)
	},
	"txn.asnd": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.AssetSender)
	},
	"txn.caid": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetConfigTxnFields.ConfigAsset)
	},
	"txn.close": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.CloseRemainderTo)
	},
	"txn.fadd": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.FreezeAccount)
	},
	"txn.faid": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetFreezeTxnFields.FreezeAsset)
	},
	"txn.fee": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.Fee)
	},
	"txn.fv": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.FirstValid)
	},
	"txn.gen": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.GenesisID)
	},
	"txn.gh": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.GenesisHash)
	},
	"txn.grp": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.Group)
	},
	"txn.lv": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.LastValid)
	},
	"txn.lx": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.Lease)
	},
	"txn.nonpart": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.Nonparticipation)
	},
	"txn.note": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.Note)
	},
	"txn.rcv": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.Receiver)
	},
	"txn.rekey": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.RekeyTo)
	},
	"txn.selkey": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.SelectionPK)
	},
	"txn.snd": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.Header.Sender)
	},
	"txn.sp": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof)
	},
	"txn.sp.P": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs)
	},
	"txn.sp.P.hsh": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.HashFactory)
	},
	"txn.sp.P.hsh.t": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.HashFactory.HashType)
	},
	"txn.sp.P.pth": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.Path)
	},
	"txn.sp.P.td": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PartProofs.TreeDepth)
	},
	"txn.sp.S": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs)
	},
	"txn.sp.S.hsh": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.HashFactory)
	},
	"txn.sp.S.hsh.t": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.HashFactory.HashType)
	},
	"txn.sp.S.pth": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.Path)
	},
	"txn.sp.S.td": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigProofs.TreeDepth)
	},
	"txn.sp.c": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SigCommit)
	},
	"txn.sp.pr": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.PositionsToReveal)
	},
	"txn.sp.r": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.Reveals)
	},
	"txn.sp.v": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.MerkleSignatureSaltVersion)
	},
	"txn.sp.w": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProof.SignedWeight)
	},
	"txn.spmsg": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message)
	},
	"txn.spmsg.P": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.LnProvenWeight)
	},
	"txn.spmsg.b": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.BlockHeadersCommitment)
	},
	"txn.spmsg.f": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.FirstAttestedRound)
	},
	"txn.spmsg.l": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.LastAttestedRound)
	},
	"txn.spmsg.v": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.Message.VotersCommitment)
	},
	"txn.sprfkey": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.StateProofPK)
	},
	"txn.sptype": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.StateProofTxnFields.StateProofType)
	},
	"txn.type": func(i *transactions.SignedTxnInBlock) interface{} { return &((*i).SignedTxnWithAD.SignedTxn.Txn.Type) },
	"txn.votefst": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteFirst)
	},
	"txn.votekd": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteKeyDilution)
	},
	"txn.votekey": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VotePK)
	},
	"txn.votelst": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.KeyregTxnFields.VoteLast)
	},
	"txn.xaid": func(i *transactions.SignedTxnInBlock) interface{} {
		return &((*i).SignedTxnWithAD.SignedTxn.Txn.AssetTransferTxnFields.XferAsset)
	},
}
