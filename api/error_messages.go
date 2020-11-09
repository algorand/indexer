package api

import (
	"fmt"

	"github.com/algorand/indexer/importer"
)

const (
	errInvalidRoundAndMinMax     = "cannot specify round and min-round/max-round"
	errInvalidRoundMinMax        = "min-round must be less than max-round"
	errUnableToParseAddress      = "unable to parse address"
	errInvalidCreatorAddress     = "found an invalid creator address"
	errUnableToParseBase64       = "unable to parse base64 data"
	errUnableToParseDigest       = "unable to parse base32 digest data"
	errUnableToParseNext         = "unable to parse next token"
	errUnableToDecodeTransaction = "unable to decode transaction bytes"
	errFailedSearchingAccount    = "failed while searching for account"
	errNoAccountsFound           = "no accounts found for address"
	errNoAssetsFound             = "no assets found for asset-id"
	errNoTransactionFound        = "no transaction found for transaction id"
	errMultipleTransactions      = "multiple transactions found for this txid, please contact us this shouldn't happen"
	errMultipleAccounts          = "multiple accounts found for this address, please contact us this shouldn't happen"
	errMultipleAssets            = "multiple assets found for this id, please contact us this shouldn't happen"
	errMultiAcctRewind           = "multiple accounts rewind is not supported by this server"
	errRewindingAccount          = "error while rewinding account"
	errLookingUpBlock            = "error while looking up block for round"
	errTransactionSearch         = "error while searching for transaction"
)

var errUnknownAddressRole string
var errUnknownTxType string
var errUnknownSigType string

func init() {
	errUnknownAddressRole = fmt.Sprintf("unknown address role [valid roles: %s]", AddressRoleEnumString)
	errUnknownTxType = fmt.Sprintf("unknown tx-type [valid types: %s]", importer.TypeEnumString)
	errUnknownSigType = fmt.Sprintf("unknown sig-type [valid types: %s]", SigTypeEnumString)
}
