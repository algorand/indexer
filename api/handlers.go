package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/go-algorand/data/basics"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/api/generated/common"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/util"
	"github.com/algorand/indexer/version"
)

// ServerImplementation implements the handler interface used by the generated route definitions.
type ServerImplementation struct {
	// EnableAddressSearchRoundRewind is allows configuring whether or not the
	// 'accounts' endpoint allows specifying a round number. This is done for
	// performance reasons, because requesting many accounts at a particular
	// round could put a lot of strain on the system (especially if the round
	// is from long ago).
	EnableAddressSearchRoundRewind bool

	db idb.IndexerDb

	fetcher error

	timeout time.Duration

	log *log.Logger

	disabledParams *DisabledMap

	opts ExtraOptions
}

//////////////////////
// Helper functions //
//////////////////////

func validateTransactionFilter(filter *idb.TransactionFilter) error {
	var errorArr = make([]string, 0)

	// Round + min/max round
	if filter.Round != nil && (filter.MaxRound != 0 || filter.MinRound != 0) {
		errorArr = append(errorArr, errInvalidRoundAndMinMax)
	}

	// If min/max are mixed up
	if filter.Round == nil && filter.MinRound != 0 && filter.MaxRound != 0 && filter.MinRound > filter.MaxRound {
		errorArr = append(errorArr, errInvalidRoundMinMax)
	}

	{
		var address basics.Address
		copy(address[:], filter.Address)
		if address.IsZero() {
			if filter.AddressRole&idb.AddressRoleCloseRemainderTo != 0 {
				errorArr = append(errorArr, errZeroAddressCloseRemainderToRole)
			}
			if filter.AddressRole&idb.AddressRoleAssetSender != 0 {
				errorArr = append(errorArr, errZeroAddressAssetSenderRole)
			}
			if filter.AddressRole&idb.AddressRoleAssetCloseTo != 0 {
				errorArr = append(errorArr, errZeroAddressAssetCloseToRole)
			}
		}
	}

	if len(errorArr) > 0 {
		return errors.New("invalid input: " + strings.Join(errorArr, ", "))
	}

	return nil
}

////////////////////////////
// Handler implementation //
////////////////////////////

// MakeHealthCheck returns health check information about indexer and the IndexerDb being used.
// Returns 200 if healthy.
// (GET /health)
func (si *ServerImplementation) MakeHealthCheck(ctx echo.Context) error {
	var err error
	var errors []string
	var health idb.Health

	err = callWithTimeout(
		ctx.Request().Context(), si.log, si.timeout, func(ctx context.Context) error {
			var err error
			health, err = si.db.Health(ctx)
			return err
		})
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedLookingUpHealth, err))
	}

	if health.Error != "" {
		errors = append(errors, fmt.Sprintf("database error: %s", health.Error))
	}

	if si.fetcher != nil && si.fetcher.Error() != "" {
		errors = append(errors, fmt.Sprintf("fetcher error: %s", si.fetcher.Error()))
	}

	return ctx.JSON(http.StatusOK, common.HealthCheckResponse{
		Version:     version.Version(),
		Data:        health.Data,
		Round:       health.Round,
		IsMigrating: health.IsMigrating,
		DbAvailable: health.DBAvailable,
		Message:     strconv.FormatUint(health.Round, 10),
		Errors:      strArrayPtr(errors),
	})
}

var errInvalidExcludeParameter = errors.New("invalid exclude argument")

// set query options based on the value of the "exclude" parameter
func setExcludeQueryOptions(exclude []string, opts *idb.AccountQueryOptions) error {
	for _, e := range exclude {
		switch e {
		case "all":
			opts.IncludeAssetHoldings = false
			opts.IncludeAssetParams = false
			opts.IncludeAppLocalState = false
			opts.IncludeAppParams = false
		case "assets":
			opts.IncludeAssetHoldings = false
		case "created-assets":
			opts.IncludeAssetParams = false
		case "apps-local-state":
			opts.IncludeAppLocalState = false
		case "created-apps":
			opts.IncludeAppParams = false
		case "none":
		default:
			return fmt.Errorf(`unknown value "%s": %w`, e, errInvalidExcludeParameter)
		}
	}
	return nil
}

func (si *ServerImplementation) verifyHandler(operationID string, ctx echo.Context) error {
	return Verify(si.disabledParams, operationID, ctx, si.log)
}

// LookupAccountByID queries indexer for a given account.
// (GET /v2/accounts/{account-id})
func (si *ServerImplementation) LookupAccountByID(ctx echo.Context, accountID string, params generated.LookupAccountByIDParams) error {
	if err := si.verifyHandler("LookupAccountByID", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	addr, decodeErrors := decodeAddress(&accountID, "account-id", make([]string, 0))
	if len(decodeErrors) != 0 {
		return badRequest(ctx, decodeErrors[0])
	}

	options := idb.AccountQueryOptions{
		EqualToAddress:       addr[:],
		IncludeAssetHoldings: true,
		IncludeAssetParams:   true,
		IncludeAppLocalState: true,
		IncludeAppParams:     true,
		Limit:                1,
		IncludeDeleted:       boolOrDefault(params.IncludeAll),
		MaxResources:         uint64(si.opts.MaxAPIResourcesPerAccount),
	}

	if params.Exclude != nil {
		err := setExcludeQueryOptions(*params.Exclude, &options)
		if err != nil {
			return badRequest(ctx, err.Error())
		}
	}

	accounts, round, err := si.fetchAccounts(ctx.Request().Context(), options, params.Round)
	if err != nil {
		var maxErr idb.MaxAPIResourcesPerAccountError
		if errors.As(err, &maxErr) {
			return ctx.JSON(http.StatusBadRequest, si.maxAccountsErrorToAccountsErrorResponse(maxErr))
		}
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingAccount, err))
	}

	if len(accounts) == 0 {
		return notFound(ctx, fmt.Sprintf("%s: %s", errNoAccountsFound, accountID))
	}

	if len(accounts) > 1 {
		return indexerError(ctx, fmt.Errorf("%s: %s", errMultipleAccounts, accountID))
	}

	return ctx.JSON(http.StatusOK, generated.AccountResponse{
		CurrentRound: round,
		Account:      accounts[0],
	})
}

// LookupAccountAppLocalStates queries indexer for AppLocalState for a given account, and optionally a given app ID.
// (GET /v2/accounts/{account-id}/apps-local-state)
func (si *ServerImplementation) LookupAccountAppLocalStates(ctx echo.Context, accountID string, params generated.LookupAccountAppLocalStatesParams) error {
	if err := si.verifyHandler("LookupAccountAppLocalStates", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	search := generated.SearchForApplicationsParams{
		Creator:       &accountID,
		ApplicationId: params.ApplicationId,
		IncludeAll:    params.IncludeAll,
		Limit:         params.Limit,
		Next:          params.Next,
	}
	options, err := si.appParamsToApplicationQuery(search)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	apps, round, err := si.fetchAppLocalStates(ctx.Request().Context(), options)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingApplication, err))
	}

	var next *string
	if len(apps) > 0 {
		next = strPtr(strconv.FormatUint(apps[len(apps)-1].Id, 10))
	}

	out := generated.ApplicationLocalStatesResponse{
		AppsLocalStates: apps,
		CurrentRound:    round,
		NextToken:       next,
	}
	return ctx.JSON(http.StatusOK, out)
}

// LookupAccountAssets queries indexer for AssetHolding for a given account, and optionally a given asset ID.
// (GET /v2/accounts/{account-id}/assets)
func (si *ServerImplementation) LookupAccountAssets(ctx echo.Context, accountID string, params generated.LookupAccountAssetsParams) error {
	if err := si.verifyHandler("LookupAccountAssets", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	addr, errors := decodeAddress(&accountID, "account-id", make([]string, 0))
	if len(errors) != 0 {
		return badRequest(ctx, errors[0])
	}

	var assetGreaterThan uint64 = 0
	if params.Next != nil {
		agt, err := strconv.ParseUint(*params.Next, 10, 64)
		if err != nil {
			return badRequest(ctx, fmt.Sprintf("%s: %v", errUnableToParseNext, err))
		}
		assetGreaterThan = agt
	}

	query := idb.AssetBalanceQuery{
		Address:        addr,
		AssetID:        uintOrDefault(params.AssetId),
		AssetIDGT:      assetGreaterThan,
		IncludeDeleted: boolOrDefault(params.IncludeAll),
		Limit:          min(uintOrDefaultValue(params.Limit, si.opts.DefaultBalancesLimit), si.opts.MaxBalancesLimit),
	}

	assets, round, err := si.fetchAssetHoldings(ctx.Request().Context(), query)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingAssetBalances, err))
	}

	var next *string
	if len(assets) > 0 {
		next = strPtr(strconv.FormatUint(assets[len(assets)-1].AssetId, 10))
	}

	return ctx.JSON(http.StatusOK, generated.AssetHoldingsResponse{
		CurrentRound: round,
		NextToken:    next,
		Assets:       assets,
	})
}

// LookupAccountCreatedApplications queries indexer for AppParams for a given account, and optionally a given app ID.
// (GET /v2/accounts/{account-id}/created-applications)
func (si *ServerImplementation) LookupAccountCreatedApplications(ctx echo.Context, accountID string, params generated.LookupAccountCreatedApplicationsParams) error {
	if err := si.verifyHandler("LookupAccountCreatedApplications", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	search := generated.SearchForApplicationsParams{
		Creator:       &accountID,
		ApplicationId: params.ApplicationId,
		IncludeAll:    params.IncludeAll,
		Limit:         params.Limit,
		Next:          params.Next,
	}
	return si.SearchForApplications(ctx, search)
}

// LookupAccountCreatedAssets queries indexer for AssetParams for a given account, and optionally a given asset ID.
// (GET /v2/accounts/{account-id}/created-assets)
func (si *ServerImplementation) LookupAccountCreatedAssets(ctx echo.Context, accountID string, params generated.LookupAccountCreatedAssetsParams) error {
	if err := si.verifyHandler("LookupAccountCreatedAssets", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	search := generated.SearchForAssetsParams{
		Creator:    &accountID,
		AssetId:    params.AssetId,
		IncludeAll: params.IncludeAll,
		Limit:      params.Limit,
		Next:       params.Next,
	}
	return si.SearchForAssets(ctx, search)
}

// SearchForAccounts returns accounts matching the provided parameters
// (GET /v2/accounts)
func (si *ServerImplementation) SearchForAccounts(ctx echo.Context, params generated.SearchForAccountsParams) error {
	if err := si.verifyHandler("SearchForAccounts", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	if !si.EnableAddressSearchRoundRewind && params.Round != nil {
		return badRequest(ctx, errMultiAcctRewind)
	}

	spendingAddr, decodeErrors := decodeAddress(params.AuthAddr, "account-id", make([]string, 0))
	if len(decodeErrors) != 0 {
		return badRequest(ctx, decodeErrors[0])
	}

	options := idb.AccountQueryOptions{
		IncludeAssetHoldings: true,
		IncludeAssetParams:   true,
		IncludeAppLocalState: true,
		IncludeAppParams:     true,
		Limit:                min(uintOrDefaultValue(params.Limit, si.opts.DefaultAccountsLimit), si.opts.MaxAccountsLimit),
		HasAssetID:           uintOrDefault(params.AssetId),
		HasAppID:             uintOrDefault(params.ApplicationId),
		EqualToAuthAddr:      spendingAddr[:],
		IncludeDeleted:       boolOrDefault(params.IncludeAll),
		MaxResources:         uint64(si.opts.MaxAPIResourcesPerAccount),
	}

	if params.Exclude != nil {
		err := setExcludeQueryOptions(*params.Exclude, &options)
		if err != nil {
			return badRequest(ctx, err.Error())
		}
	}

	// Set GT/LT on Algos or Asset depending on whether or not an assetID was specified
	if options.HasAssetID == 0 {
		options.AlgosGreaterThan = params.CurrencyGreaterThan
		options.AlgosLessThan = params.CurrencyLessThan
	} else {
		options.AssetGT = params.CurrencyGreaterThan
		options.AssetLT = params.CurrencyLessThan
	}

	if params.Next != nil {
		addr, err := basics.UnmarshalChecksumAddress(*params.Next)
		if err != nil {
			return badRequest(ctx, errUnableToParseNext)
		}
		options.GreaterThanAddress = addr[:]
	}

	accounts, round, err := si.fetchAccounts(ctx.Request().Context(), options, params.Round)
	if err != nil {
		var maxErr idb.MaxAPIResourcesPerAccountError
		if errors.As(err, &maxErr) {
			return ctx.JSON(http.StatusBadRequest, si.maxAccountsErrorToAccountsErrorResponse(maxErr))
		}
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingAccount, err))
	}

	var next *string
	if len(accounts) > 0 {
		next = strPtr(accounts[len(accounts)-1].Address)
	}

	response := generated.AccountsResponse{
		CurrentRound: round,
		NextToken:    next,
		Accounts:     accounts,
	}

	return ctx.JSON(http.StatusOK, response)
}

// LookupAccountTransactions looks up transactions associated with a particular account.
// (GET /v2/accounts/{account-id}/transactions)
func (si *ServerImplementation) LookupAccountTransactions(ctx echo.Context, accountID string, params generated.LookupAccountTransactionsParams) error {
	if err := si.verifyHandler("LookupAccountTransactions", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}
	// Check that a valid account was provided
	_, errors := decodeAddress(strPtr(accountID), "account-id", make([]string, 0))
	if len(errors) != 0 {
		return badRequest(ctx, errors[0])
	}

	searchParams := generated.SearchForTransactionsParams{
		Address: strPtr(accountID),
		// not applicable to this endpoint
		//AddressRole:         params.AddressRole,
		//ExcludeCloseTo:      params.ExcludeCloseTo,
		AssetId:             params.AssetId, // This probably shouldn't have been included
		ApplicationId:       nil,
		Limit:               params.Limit,
		Next:                params.Next,
		NotePrefix:          params.NotePrefix,
		TxType:              params.TxType,
		SigType:             params.SigType,
		Txid:                params.Txid,
		Round:               params.Round,
		MinRound:            params.MinRound,
		MaxRound:            params.MaxRound,
		BeforeTime:          params.BeforeTime,
		AfterTime:           params.AfterTime,
		CurrencyGreaterThan: params.CurrencyGreaterThan,
		CurrencyLessThan:    params.CurrencyLessThan,
		RekeyTo:             params.RekeyTo,
	}

	return si.SearchForTransactions(ctx, searchParams)
}

// SearchForApplications returns applications for the provided parameters.
// (GET /v2/applications)
func (si *ServerImplementation) SearchForApplications(ctx echo.Context, params generated.SearchForApplicationsParams) error {
	if err := si.verifyHandler("SearchForApplications", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	options, err := si.appParamsToApplicationQuery(params)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	apps, round, err := si.fetchApplications(ctx.Request().Context(), options)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingApplication, err))
	}

	var next *string
	if len(apps) > 0 {
		next = strPtr(strconv.FormatUint(apps[len(apps)-1].Id, 10))
	}

	out := generated.ApplicationsResponse{
		Applications: apps,
		CurrentRound: round,
		NextToken:    next,
	}
	return ctx.JSON(http.StatusOK, out)
}

// LookupApplicationByID returns one application for the requested ID.
// (GET /v2/applications/{application-id})
func (si *ServerImplementation) LookupApplicationByID(ctx echo.Context, applicationID uint64, params generated.LookupApplicationByIDParams) error {
	if err := si.verifyHandler("LookupApplicationByID", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}
	q := idb.ApplicationQuery{
		ApplicationID:  applicationID,
		IncludeDeleted: boolOrDefault(params.IncludeAll),
		Limit:          1,
	}

	apps, round, err := si.fetchApplications(ctx.Request().Context(), q)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingApplication, err))
	}

	if len(apps) == 0 {
		return notFound(ctx, fmt.Sprintf("%s: %d", errNoApplicationsFound, applicationID))
	}

	if len(apps) > 1 {
		return indexerError(ctx, fmt.Errorf("%s: %d", errMultipleApplications, applicationID))
	}

	return ctx.JSON(http.StatusOK, generated.ApplicationResponse{
		Application:  &(apps[0]),
		CurrentRound: round,
	})
}

// LookupApplicationLogsByID returns one application logs
// (GET /v2/applications/{application-id}/logs)
func (si *ServerImplementation) LookupApplicationLogsByID(ctx echo.Context, applicationID uint64, params generated.LookupApplicationLogsByIDParams) error {
	if err := si.verifyHandler("LookupApplicationLogsByID", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	searchParams := generated.SearchForTransactionsParams{
		AssetId:       nil,
		ApplicationId: uint64Ptr(applicationID),
		Limit:         params.Limit,
		Next:          params.Next,
		Txid:          params.Txid,
		MinRound:      params.MinRound,
		MaxRound:      params.MaxRound,
		Address:       params.SenderAddress,
	}

	filter, err := si.transactionParamsToTransactionFilter(searchParams)
	if err != nil {
		return badRequest(ctx, err.Error())
	}
	filter.AddressRole = idb.AddressRoleSender
	// If there is a match on an inner transaction, return the inner txn's logs
	// instead of the root txn's logs.
	filter.ReturnInnerTxnOnly = true

	err = validateTransactionFilter(&filter)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	// Fetch the transactions
	txns, next, round, err := si.fetchTransactions(ctx.Request().Context(), filter)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errTransactionSearch, err))
	}

	var logData []generated.ApplicationLogData
	for _, txn := range txns {
		if txn.Logs != nil && len(*txn.Logs) > 0 {
			logData = append(logData, generated.ApplicationLogData{
				Txid: *txn.Id,
				Logs: *txn.Logs,
			})
		}
	}

	var logDataResult *[]generated.ApplicationLogData
	if len(logData) > 0 {
		logDataResult = &logData
	}

	response := generated.ApplicationLogsResponse{
		ApplicationId: applicationID,
		CurrentRound:  round,
		NextToken:     strPtr(next),
		LogData:       logDataResult,
	}

	return ctx.JSON(http.StatusOK, response)
}

// LookupAssetByID looks up a particular asset
// (GET /v2/assets/{asset-id})
func (si *ServerImplementation) LookupAssetByID(ctx echo.Context, assetID uint64, params generated.LookupAssetByIDParams) error {
	if err := si.verifyHandler("LookupAssetByID", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	search := generated.SearchForAssetsParams{
		AssetId:    uint64Ptr(assetID),
		Limit:      uint64Ptr(1),
		IncludeAll: params.IncludeAll,
	}
	options, err := si.assetParamsToAssetQuery(search)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	assets, round, err := si.fetchAssets(ctx.Request().Context(), options)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingAsset, err))
	}

	if len(assets) == 0 {
		return notFound(ctx, fmt.Sprintf("%s: %d", errNoAssetsFound, assetID))
	}

	if len(assets) > 1 {
		return indexerError(ctx, fmt.Errorf("%s: %d", errMultipleAssets, assetID))
	}

	return ctx.JSON(http.StatusOK, generated.AssetResponse{
		Asset:        assets[0],
		CurrentRound: round,
	})
}

// LookupAssetBalances looks up balances for a particular asset
// (GET /v2/assets/{asset-id}/balances)
func (si *ServerImplementation) LookupAssetBalances(ctx echo.Context, assetID uint64, params generated.LookupAssetBalancesParams) error {
	if err := si.verifyHandler("LookupAssetBalances", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	query := idb.AssetBalanceQuery{
		AssetID:        assetID,
		AmountGT:       params.CurrencyGreaterThan,
		AmountLT:       params.CurrencyLessThan,
		IncludeDeleted: boolOrDefault(params.IncludeAll),
		Limit:          min(uintOrDefaultValue(params.Limit, si.opts.DefaultBalancesLimit), si.opts.MaxBalancesLimit),
	}

	if params.Next != nil {
		addr, err := basics.UnmarshalChecksumAddress(*params.Next)
		if err != nil {
			return badRequest(ctx, errUnableToParseNext)
		}
		query.PrevAddress = addr[:]
	}

	balances, round, err := si.fetchAssetBalances(ctx.Request().Context(), query)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingAssetBalances, err))
	}

	var next *string
	if len(balances) > 0 {
		next = strPtr(balances[len(balances)-1].Address)
	}

	return ctx.JSON(http.StatusOK, generated.AssetBalancesResponse{
		CurrentRound: round,
		NextToken:    next,
		Balances:     balances,
	})
}

// LookupAssetTransactions looks up transactions associated with a particular asset
// (GET /v2/assets/{asset-id}/transactions)
func (si *ServerImplementation) LookupAssetTransactions(ctx echo.Context, assetID uint64, params generated.LookupAssetTransactionsParams) error {
	if err := si.verifyHandler("LookupAssetTransactions", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	searchParams := generated.SearchForTransactionsParams{
		AssetId:             uint64Ptr(assetID),
		ApplicationId:       nil,
		Limit:               params.Limit,
		Next:                params.Next,
		NotePrefix:          params.NotePrefix,
		TxType:              params.TxType,
		SigType:             params.SigType,
		Txid:                params.Txid,
		Round:               params.Round,
		MinRound:            params.MinRound,
		MaxRound:            params.MaxRound,
		BeforeTime:          params.BeforeTime,
		AfterTime:           params.AfterTime,
		CurrencyGreaterThan: params.CurrencyGreaterThan,
		CurrencyLessThan:    params.CurrencyLessThan,
		Address:             params.Address,
		AddressRole:         params.AddressRole,
		ExcludeCloseTo:      params.ExcludeCloseTo,
		RekeyTo:             params.RekeyTo,
	}

	return si.SearchForTransactions(ctx, searchParams)
}

// SearchForAssets returns assets matching the provided parameters
// (GET /v2/assets)
func (si *ServerImplementation) SearchForAssets(ctx echo.Context, params generated.SearchForAssetsParams) error {
	if err := si.verifyHandler("SearchForAssets", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	options, err := si.assetParamsToAssetQuery(params)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	assets, round, err := si.fetchAssets(ctx.Request().Context(), options)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errFailedSearchingAsset, err))
	}

	var next *string
	if len(assets) > 0 {
		next = strPtr(strconv.FormatUint(assets[len(assets)-1].Index, 10))
	}

	return ctx.JSON(http.StatusOK, generated.AssetsResponse{
		CurrentRound: round,
		NextToken:    next,
		Assets:       assets,
	})
}

// LookupBlock returns the block for a given round number
// (GET /v2/blocks/{round-number})
func (si *ServerImplementation) LookupBlock(ctx echo.Context, roundNumber uint64) error {
	if err := si.verifyHandler("LookupBlock", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	blk, err := si.fetchBlock(ctx.Request().Context(), roundNumber)
	if errors.Is(err, idb.ErrorBlockNotFound) {
		return notFound(ctx, fmt.Sprintf("%s '%d': %v", errLookingUpBlockForRound, roundNumber, err))
	}
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s '%d': %w", errLookingUpBlockForRound, roundNumber, err))
	}

	return ctx.JSON(http.StatusOK, generated.BlockResponse(blk))
}

// LookupTransaction searches for the requested transaction ID.
func (si *ServerImplementation) LookupTransaction(ctx echo.Context, txid string) error {
	if err := si.verifyHandler("LookupTransaction", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	filter, err := si.transactionParamsToTransactionFilter(generated.SearchForTransactionsParams{
		Txid: strPtr(txid),
	})
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	err = validateTransactionFilter(&filter)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	// Fetch the transactions
	txns, _, round, err := si.fetchTransactions(ctx.Request().Context(), filter)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errTransactionSearch, err))
	}

	if len(txns) == 0 {
		return notFound(ctx, fmt.Sprintf("%s: %s", errNoTransactionFound, txid))
	}

	if len(txns) > 1 {
		return indexerError(ctx, fmt.Errorf("%s: %s", errMultipleTransactions, txid))
	}

	response := generated.TransactionResponse{
		CurrentRound: round,
		Transaction:  txns[0],
	}

	return ctx.JSON(http.StatusOK, response)
}

// SearchForTransactions returns transactions matching the provided parameters
// (GET /v2/transactions)
func (si *ServerImplementation) SearchForTransactions(ctx echo.Context, params generated.SearchForTransactionsParams) error {
	if err := si.verifyHandler("SearchForTransactions", ctx); err != nil {
		return badRequest(ctx, err.Error())
	}

	filter, err := si.transactionParamsToTransactionFilter(params)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	err = validateTransactionFilter(&filter)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	// Fetch the transactions
	txns, next, round, err := si.fetchTransactions(ctx.Request().Context(), filter)
	if err != nil {
		return indexerError(ctx, fmt.Errorf("%s: %w", errTransactionSearch, err))
	}

	response := generated.TransactionsResponse{
		CurrentRound: round,
		NextToken:    strPtr(next),
		Transactions: txns,
	}

	return ctx.JSON(http.StatusOK, response)
}

///////////////////
// Error Helpers //
///////////////////

// return a 400
func badRequest(ctx echo.Context, err string) error {
	return ctx.JSON(http.StatusBadRequest, generated.ErrorResponse{
		Message: err,
	})
}

// return a 503
func timeoutError(ctx echo.Context, err string) error {
	return ctx.JSON(http.StatusServiceUnavailable, generated.ErrorResponse{
		Message: err,
	})
}

// return a 500, or 503 if it is a timeout error
func indexerError(ctx echo.Context, err error) error {
	if isTimeoutError(err) {
		return timeoutError(ctx, err.Error())
	}

	return ctx.JSON(http.StatusInternalServerError, generated.ErrorResponse{
		Message: err.Error(),
	})
}

// return a 404
func notFound(ctx echo.Context, err string) error {
	return ctx.JSON(http.StatusNotFound, generated.ErrorResponse{
		Message: err,
	})
}

///////////////////////
// IndexerDb helpers //
///////////////////////

// fetchApplications fetches all results
func (si *ServerImplementation) fetchApplications(ctx context.Context, params idb.ApplicationQuery) ([]generated.Application, uint64, error) {
	var apps []generated.Application
	var round uint64
	err := callWithTimeout(ctx, si.log, si.timeout, func(ctx context.Context) error {
		var results <-chan idb.ApplicationRow
		results, round = si.db.Applications(ctx, params)

		for result := range results {
			if result.Error != nil {
				return result.Error
			}
			apps = append(apps, result.Application)
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return apps, round, nil
}

// fetchAppLocalStates fetches all generated.AppLocalState from a query
func (si *ServerImplementation) fetchAppLocalStates(ctx context.Context, params idb.ApplicationQuery) ([]generated.ApplicationLocalState, uint64, error) {
	var als []generated.ApplicationLocalState
	var round uint64
	err := callWithTimeout(ctx, si.log, si.timeout, func(ctx context.Context) error {
		var results <-chan idb.AppLocalStateRow
		results, round = si.db.AppLocalState(ctx, params)

		for result := range results {
			if result.Error != nil {
				return result.Error
			}
			als = append(als, result.AppLocalState)
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return als, round, nil
}

// fetchAssets fetches all results and converts them into generated.Asset objects
func (si *ServerImplementation) fetchAssets(ctx context.Context, options idb.AssetsQuery) ([]generated.Asset, uint64 /*round*/, error) {
	var round uint64
	assets := make([]generated.Asset, 0)
	err := callWithTimeout(ctx, si.log, si.timeout, func(ctx context.Context) error {
		var assetchan <-chan idb.AssetRow
		assetchan, round = si.db.Assets(ctx, options)
		for row := range assetchan {
			if row.Error != nil {
				return row.Error
			}

			creator := basics.Address{}
			if len(row.Creator) != len(creator) {
				return fmt.Errorf(errInvalidCreatorAddress)
			}
			copy(creator[:], row.Creator[:])

			mdhash := make([]byte, 32)
			copy(mdhash, row.Params.MetadataHash[:])

			asset := generated.Asset{
				Index:            row.AssetID,
				CreatedAtRound:   row.CreatedRound,
				DestroyedAtRound: row.ClosedRound,
				Deleted:          row.Deleted,
				Params: generated.AssetParams{
					Creator:       creator.String(),
					Name:          strPtr(util.PrintableUTF8OrEmpty(row.Params.AssetName)),
					UnitName:      strPtr(util.PrintableUTF8OrEmpty(row.Params.UnitName)),
					Url:           strPtr(util.PrintableUTF8OrEmpty(row.Params.URL)),
					NameB64:       byteSlicePtr([]byte(row.Params.AssetName)),
					UnitNameB64:   byteSlicePtr([]byte(row.Params.UnitName)),
					UrlB64:        byteSlicePtr([]byte(row.Params.URL)),
					Total:         row.Params.Total,
					Decimals:      uint64(row.Params.Decimals),
					DefaultFrozen: boolPtr(row.Params.DefaultFrozen),
					MetadataHash:  byteSliceOmitZeroPtr(mdhash),
					Clawback:      strPtr(row.Params.Clawback.String()),
					Reserve:       strPtr(row.Params.Reserve.String()),
					Freeze:        strPtr(row.Params.Freeze.String()),
					Manager:       strPtr(row.Params.Manager.String()),
				},
			}

			assets = append(assets, asset)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return assets, round, nil
}

// fetchAssetBalances fetches all balances from a query and converts them into
// generated.MiniAssetHolding objects
func (si *ServerImplementation) fetchAssetBalances(ctx context.Context, options idb.AssetBalanceQuery) ([]generated.MiniAssetHolding, uint64 /*round*/, error) {
	var round uint64
	balances := make([]generated.MiniAssetHolding, 0)
	err := callWithTimeout(ctx, si.log, si.timeout, func(ctx context.Context) error {
		var assetbalchan <-chan idb.AssetBalanceRow
		assetbalchan, round = si.db.AssetBalances(ctx, options)

		for row := range assetbalchan {
			if row.Error != nil {
				return row.Error
			}

			addr := basics.Address{}
			if len(row.Address) != len(addr) {
				return fmt.Errorf(errInvalidCreatorAddress)
			}
			copy(addr[:], row.Address[:])

			bal := generated.MiniAssetHolding{
				Address:         addr.String(),
				Amount:          row.Amount,
				IsFrozen:        row.Frozen,
				OptedInAtRound:  row.CreatedRound,
				OptedOutAtRound: row.ClosedRound,
				Deleted:         row.Deleted,
			}

			balances = append(balances, bal)
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return balances, round, nil
}

// fetchAssetHoldings fetches all balances from a query and converts them into
// generated.AssetHolding objects
func (si *ServerImplementation) fetchAssetHoldings(ctx context.Context, options idb.AssetBalanceQuery) ([]generated.AssetHolding, uint64 /*round*/, error) {
	var round uint64
	balances := make([]generated.AssetHolding, 0)
	err := callWithTimeout(ctx, si.log, si.timeout, func(ctx context.Context) error {
		var assetbalchan <-chan idb.AssetBalanceRow
		assetbalchan, round = si.db.AssetBalances(ctx, options)

		for row := range assetbalchan {
			if row.Error != nil {
				return row.Error
			}

			addr := basics.Address{}
			if len(row.Address) != len(addr) {
				return fmt.Errorf(errInvalidCreatorAddress)
			}
			copy(addr[:], row.Address[:])

			bal := generated.AssetHolding{
				Amount:          row.Amount,
				AssetId:         row.AssetID,
				IsFrozen:        row.Frozen,
				OptedInAtRound:  row.CreatedRound,
				OptedOutAtRound: row.ClosedRound,
				Deleted:         row.Deleted,
			}

			balances = append(balances, bal)
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return balances, round, nil
}

// fetchBlock looks up a block and converts it into a generated.Block object
// the method also loads the transactions into the returned block object.
func (si *ServerImplementation) fetchBlock(ctx context.Context, round uint64) (generated.Block, error) {
	var ret generated.Block
	err := callWithTimeout(ctx, si.log, si.timeout, func(ctx context.Context) error {
		blockHeader, transactions, err :=
			si.db.GetBlock(ctx, round, idb.GetBlockOptions{Transactions: true})
		if err != nil {
			return err
		}

		rewards := generated.BlockRewards{
			FeeSink:                 blockHeader.FeeSink.String(),
			RewardsCalculationRound: uint64(blockHeader.RewardsRecalculationRound),
			RewardsLevel:            blockHeader.RewardsLevel,
			RewardsPool:             blockHeader.RewardsPool.String(),
			RewardsRate:             blockHeader.RewardsRate,
			RewardsResidue:          blockHeader.RewardsResidue,
		}

		upgradeState := generated.BlockUpgradeState{
			CurrentProtocol:        string(blockHeader.CurrentProtocol),
			NextProtocol:           strPtr(string(blockHeader.NextProtocol)),
			NextProtocolApprovals:  uint64Ptr(blockHeader.NextProtocolApprovals),
			NextProtocolSwitchOn:   uint64Ptr(uint64(blockHeader.NextProtocolSwitchOn)),
			NextProtocolVoteBefore: uint64Ptr(uint64(blockHeader.NextProtocolVoteBefore)),
		}

		upgradeVote := generated.BlockUpgradeVote{
			UpgradeApprove: boolPtr(blockHeader.UpgradeApprove),
			UpgradeDelay:   uint64Ptr(uint64(blockHeader.UpgradeDelay)),
			UpgradePropose: strPtr(string(blockHeader.UpgradePropose)),
		}

		ret = generated.Block{
			GenesisHash:       blockHeader.GenesisHash[:],
			GenesisId:         blockHeader.GenesisID,
			PreviousBlockHash: blockHeader.Branch[:],
			Rewards:           &rewards,
			Round:             uint64(blockHeader.Round),
			Seed:              blockHeader.Seed[:],
			Timestamp:         uint64(blockHeader.TimeStamp),
			Transactions:      nil,
			TransactionsRoot:  blockHeader.TxnRoot[:],
			TxnCounter:        uint64Ptr(blockHeader.TxnCounter),
			UpgradeState:      &upgradeState,
			UpgradeVote:       &upgradeVote,
		}

		results := make([]generated.Transaction, 0)
		for _, txrow := range transactions {
			// Do not include inner transactions.
			if txrow.RootTxn != nil {
				continue
			}

			tx, err := txnRowToTransaction(txrow)
			if err != nil {
				return err
			}

			results = append(results, tx)
		}

		ret.Transactions = &results
		return err
	})
	if err != nil {
		return generated.Block{}, err
	}
	return ret, nil
}

// fetchAccounts queries for accounts and converts them into generated.Account
// objects, optionally rewinding their value back to a particular round.
func (si *ServerImplementation) fetchAccounts(ctx context.Context, options idb.AccountQueryOptions, atRound *uint64) ([]generated.Account, uint64 /*round*/, error) {
	var round uint64
	accounts := make([]generated.Account, 0)
	err := callWithTimeout(ctx, si.log, si.timeout, func(ctx context.Context) error {
		var accountchan <-chan idb.AccountRow
		accountchan, round = si.db.GetAccounts(ctx, options)

		// Make sure accountchan is empty at the end of processing.
		defer func() {
			for range accountchan {
			}
		}()

		if (atRound != nil) && (*atRound > round) {
			return fmt.Errorf("%s: the requested round %d > the current round %d",
				errRewindingAccount, *atRound, round)
		}

		for row := range accountchan {
			if row.Error != nil {
				return row.Error
			}

			// Compute for a given round if requested.
			var account generated.Account
			if atRound != nil {
				acct, err := accounting.AccountAtRound(ctx, row.Account, *atRound, si.db)
				if err != nil {
					// Ignore the error if this is an account search rewind error
					_, isSpecialAccountRewindError := err.(*accounting.SpecialAccountRewindError)
					if len(options.EqualToAddress) != 0 || !isSpecialAccountRewindError {
						return fmt.Errorf("%s: %v", errRewindingAccount, err)
					}
					// If we didn't return, continue to the next account
					continue
				}
				account = acct
			} else {
				account = row.Account
			}

			// match the algod equivalent which includes pending rewards
			account.Rewards += account.PendingRewards
			accounts = append(accounts, account)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return accounts, round, nil
}

// fetchTransactions is used to query the backend for transactions, and compute the next token
// If returnInnerTxnOnly is false, then the root txn is returned for a inner txn match.
func (si *ServerImplementation) fetchTransactions(ctx context.Context, filter idb.TransactionFilter) ([]generated.Transaction, string, uint64 /*round*/, error) {
	var round uint64
	var nextToken string
	results := make([]generated.Transaction, 0)
	err := callWithTimeout(ctx, si.log, si.timeout, func(ctx context.Context) error {
		var txchan <-chan idb.TxnRow
		txchan, round = si.db.Transactions(ctx, filter)

		rootTxnDedupeMap := make(map[string]struct{})
		var lastTxrow idb.TxnRow
		for txrow := range txchan {
			tx, err := txnRowToTransaction(txrow)
			if err != nil {
				return err
			}

			// Do not return inner transactions.
			if tx.Id == nil {
				continue
			}

			// The root txn has already been added.
			// If we also want to return inner txns, we cannot deduplicate the
			// results as inner txns all share the same txn ID as its root txn.
			if _, ok := rootTxnDedupeMap[*tx.Id]; ok && !filter.ReturnInnerTxnOnly {
				continue
			}

			rootTxnDedupeMap[*tx.Id] = struct{}{}
			results = append(results, tx)
			lastTxrow = txrow
		}

		// No next token if there were no results.
		if len(results) == 0 {
			return nil
		}

		// The sort order depends on whether the address filter is used.
		var err error
		nextToken, err = lastTxrow.Next(filter.Address == nil)

		return err
	})
	if err != nil {
		return nil, "", 0, err
	}

	return results, nextToken, round, nil
}

//////////////////////
// Helper functions //
//////////////////////

func min(x, y uint64) uint64 {
	if x < y {
		return x
	}
	return y
}
