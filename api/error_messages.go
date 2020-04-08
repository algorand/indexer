package api

import(
	"fmt"

	"github.com/algorand/indexer/importer"
)

const (
	errInvalidRoundMinMax   = "cannot specify round and min-round/max-round"
	errUnableToParseAddress = "unable to parse address"
)

var errUnknownAddressRole string
var errUnknownTxType string
var errUnknownSigType string

func init() {
	errUnknownAddressRole = fmt.Sprintf("unknown address role [valid roles: %s]", AddressRoleEnumString)
	errUnknownTxType      = fmt.Sprintf("unknown tx-type [valid types: %s]", importer.TypeEnumString)
	errUnknownSigType     = fmt.Sprintf("unknown sig-type [valid types: %s]", SigTypeEnumString)
}

