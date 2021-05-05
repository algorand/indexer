package idb

import (
	"github.com/algorand/go-algorand/data/transactions"
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

func SigTypeString() string {
	return string(Sig) + ", " + string(Msig) + ", " + string(Lsig)
}

func SignatureType(stxn transactions.SignedTxn) SigType {
	if !stxn.Sig.Blank() {
		return Sig
	}
	if !stxn.Msig.Blank() {
		return Msig
	}
	if !stxn.Lsig.Sig.Blank() {
		return Sig
	}
	if !stxn.Lsig.Msig.Blank() {
		return Msig
	}
	return Lsig
}
