package api

import (
	"fmt"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util"
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
	errSpecialAccounts           = "indexer doesn't support fee sink and rewards pool accounts, please refer to algod for relevant information"
	errFailedLoadSpecialAccounts = "failed to retrieve special accounts"
)

var errUnknownAddressRole string
var errUnknownTxType string
var errUnknownSigType string

func init() {
	AddressRoleEnumString := util.KeysStringBool(addressRoleEnumMap)
	errUnknownAddressRole = fmt.Sprintf("unknown address role [valid roles: %s]", AddressRoleEnumString)

	errUnknownTxType = fmt.Sprintf("unknown tx-type [valid types: %s]", idb.TypeEnumString())

	errUnknownSigType = fmt.Sprintf("unknown sig-type [valid types: %s]", idb.SigTypeString())
}
