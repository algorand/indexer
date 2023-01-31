package idb

import (
	"fmt"
	"strings"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
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

// SignatureType returns the signature type of the given signed transaction.
func SignatureType(stxn *sdk.SignedTxn) (SigType, error) {
	blankSignature := sdk.Signature{}
	if stxn.Sig != blankSignature {
		return Sig, nil
	}
	if !stxn.Msig.Blank() {
		return Msig, nil
	}
	if !stxn.Lsig.Blank() {
		if stxn.Lsig.Sig != blankSignature {
			return Sig, nil
		}
		if !stxn.Lsig.Msig.Blank() {
			return Msig, nil
		}
		return Lsig, nil
	}

	return "", fmt.Errorf("unable to determine the signature type for stxn=%+v", stxn)
}
