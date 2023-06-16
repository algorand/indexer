package api

import (
	"fmt"

	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/util"
)

// constant error messages.
const (
	errInvalidRoundAndMinMax           = "cannot specify round and min-round/max-round"
	errInvalidRoundMinMax              = "min-round must be less than max-round"
	errUnableToParseAddress            = "unable to parse address"
	errInvalidCreatorAddress           = "found an invalid creator address"
	errUnableToParseBase64             = "unable to parse base64 data"
	errUnableToParseDigest             = "unable to parse base32 digest data"
	errUnableToParseNext               = "unable to parse next token"
	errUnableToDecodeTransaction       = "unable to decode transaction bytes"
	errFailedSearchingAccount          = "failed while searching for account"
	errFailedSearchingAsset            = "failed while searching for asset"
	errFailedSearchingAssetBalances    = "failed while searching for asset balances"
	errFailedSearchingApplication      = "failed while searching for application"
	errFailedSearchingBoxes            = "failed while searching for application boxes"
	errFailedLookingUpHealth           = "failed while getting indexer health"
	errNoApplicationsFound             = "no application found for application-id"
	ErrNoBoxesFound                    = "no application boxes found"
	ErrWrongAppidFound                 = "the wrong application-id was found, please contact us, this shouldn't happen"
	ErrWrongBoxFound                   = "a box with an unexpected name was found, please contact us, this shouldn't happen"
	ErrNoAccountsFound                 = "no accounts found for address"
	errNoAssetsFound                   = "no assets found for asset-id"
	errNoTransactionFound              = "no transaction found for transaction id"
	errMultipleTransactions            = "multiple transactions found for this txid, please contact us, this shouldn't happen"
	errMultipleAccounts                = "multiple accounts found for this address, please contact us, this shouldn't happen"
	errMultipleAssets                  = "multiple assets found for this id, please contact us, this shouldn't happen"
	errMultipleApplications            = "multiple applications found for this id, please contact us, this shouldn't happen"
	ErrMultipleBoxes                   = "multiple application boxes found for this app id and box name, please contact us, this shouldn't happen"
	ErrFailedLookingUpBoxes            = "failed while looking up application boxes"
	errMultiAcctRewind                 = "multiple accounts rewind is not supported by this server"
	errRewindingAccount                = "error while rewinding account"
	errLookingUpBlockForRound          = "error while looking up block for round"
	errTransactionSearch               = "error while searching for transaction"
	errZeroAddressCloseRemainderToRole = "searching transactions by zero address with close address role is not supported"
	errZeroAddressAssetSenderRole      = "searching transactions by zero address with asset sender role is not supported"
	errZeroAddressAssetCloseToRole     = "searching transactions by zero address with asset close address role is not supported"
	ErrResultLimitReached              = "Result limit exceeded"
	errValueExceedingInt64             = "searching by round or application-id or asset-id or filter by value greater than 9223372036854775807 is not supported"
	errTransactionsLimitReached        = "Max transactions limit exceeded. header-only flag should be enabled"
	ErrBoxNotFound                     = "box not found"
)

var errUnknownAddressRole string
var errUnknownTxType string
var errUnknownSigType string

func init() {
	errUnknownAddressRole = fmt.Sprintf(
		"unknown address role [valid roles: %s]", util.KeysStringBool(addressRoleEnumMap))
	errUnknownTxType = fmt.Sprintf(
		"unknown tx-type [valid types: %s]", idb.TxnTypeEnumString)
	errUnknownSigType = fmt.Sprintf(
		"unknown sig-type [valid types: %s]", idb.SigTypeEnumString)
}
