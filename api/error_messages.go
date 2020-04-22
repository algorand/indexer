package api

import(
	"fmt"

	"github.com/algorand/indexer/importer"
)

const (
	errInvalidRoundAndMinMax = "cannot specify round and min-round/max-round"
	errInvalidRoundMinMax    = "min-round must be less than max-round"
	errUnableToParseAddress  = "unable to parse address"
	errUnableToParseBase64   = "unable to parse base64 data"
)

var errUnknownAddressRole string
var errUnknownTxType string
var errUnknownSigType string

func init() {
	errUnknownAddressRole = fmt.Sprintf("unknown address role [valid roles: %s]", AddressRoleEnumString)
	errUnknownTxType      = fmt.Sprintf("unknown tx-type [valid types: %s]", importer.TypeEnumString)
	errUnknownSigType     = fmt.Sprintf("unknown sig-type [valid types: %s]", SigTypeEnumString)
}

