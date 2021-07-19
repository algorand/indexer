package idb

import (
	"fmt"
	"strings"

	sdk_types "github.com/algorand/go-algorand-sdk/types"
)

// SigType is signature type.
type SigType string

// Possible signature types.
const (
	Sig  SigType = "sig"
	Msig SigType = "msig"
	Lsig SigType = "lsig"
)

var sigTypeEnumMap = map[SigType]struct{}{
	Sig:  {},
	Msig: {},
	Lsig: {},
}

func makeSigTypeEnumString() string {
	keys := make([]string, 0, len(sigTypeEnumMap))
	for k := range sigTypeEnumMap {
		keys = append(keys, string(k))
	}
	return strings.Join(keys, ", ")
}

// SigTypeEnumString is a comma-separated list of possible signature types.
var SigTypeEnumString = makeSigTypeEnumString()

// IsSigTypeValid returns true if and only if `sigtype` is one of the possible
// signature types.
func IsSigTypeValid(sigtype SigType) bool {
	_, ok := sigTypeEnumMap[sigtype]
	return ok
}

func isZeroSig(sig sdk_types.Signature) bool {
	for _, b := range sig {
		if b != 0 {
			return false
		}
	}
	return true
}

func blankLsig(lsig sdk_types.LogicSig) bool {
	return len(lsig.Logic) == 0
}

// SignatureType returns the signature type of the given signed transaction.
func SignatureType(stxn *sdk_types.SignedTxn) (SigType, error) {
	if !isZeroSig(stxn.Sig) {
		return Sig, nil
	}
	if !stxn.Msig.Blank() {
		return Msig, nil
	}
	if !blankLsig(stxn.Lsig) {
		if !isZeroSig(stxn.Lsig.Sig) {
			return Sig, nil
		}
		if !stxn.Lsig.Msig.Blank() {
			return Msig, nil
		}
		return Lsig, nil
	}

	return "", fmt.Errorf("unable to determine the signature type")
}
