package idb

import (
	sdk_types "github.com/algorand/go-algorand-sdk/types"
)

type SigType string

const (
	Sig  SigType = "sig"
	Msig SigType = "msig"
	Lsig SigType = "lsig"
)

func IsSigTypeValid(sigtype SigType) bool {
	return (sigtype == Sig) || (sigtype == Msig) || (sigtype == Lsig)
}

func isZeroSig(sig sdk_types.Signature) bool {
	for _, b := range sig {
		if b != 0 {
			return false
		}
	}
	return true
}

func SignatureType(stxn *sdk_types.SignedTxn) SigType {
	if !isZeroSig(stxn.Sig) {
		return Sig
	}
	if !stxn.Msig.Blank() {
		return Msig
	}
	if !isZeroSig(stxn.Lsig.Sig) {
		return Sig
	}
	if !stxn.Lsig.Msig.Blank() {
		return Msig
	}
	return Lsig
}
