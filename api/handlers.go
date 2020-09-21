package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/api/generated/common"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
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

	log *log.Logger
}

/////////////////////
// Limit Constants //
/////////////////////

// Transactions
const maxTransactionsLimit = 10000
const defaultTransactionsLimit = 1000

// Accounts
const maxAccountsLimit = 1000
const defaultAccountsLimit = 100

// Assets
const maxAssetsLimit = 1000
const defaultAssetsLimit = 100

// Asset Balances
const maxBalancesLimit = 10000
const defaultBalancesLimit = 1000

////////////////////////////
// Handler implementation //
////////////////////////////

// Returns 200 if healthy.
// (GET /health)
func (si *ServerImplementation) MakeHealthCheck(ctx echo.Context) error {
	maxRound, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, fmt.Sprintf("get max round: %v", err))
	}
	return ctx.JSON(http.StatusOK, common.HealthCheckResponse{
		Message: strconv.FormatUint(maxRound, 10),
	})
}

// LookupAccountByID queries indexer for a given account.
// (GET /v2/accounts/{account-id})
func (si *ServerImplementation) LookupAccountByID(ctx echo.Context, accountID string, params generated.LookupAccountByIDParams) error {
	addr, errors := decodeAddress(&accountID, "account-id", make([]string, 0))
	if len(errors) != 0 {
		return badRequest(ctx, errors[0])
	}

	options := idb.AccountQueryOptions{
		EqualToAddress:       addr[:],
		IncludeAssetHoldings: true,
		IncludeAssetParams:   true,
		Limit:                1,
	}

	accounts, err := si.fetchAccounts(ctx.Request().Context(), options, params.Round)

	if err != nil {
		return indexerError(ctx, fmt.Sprintf("%s: %v", errFailedSearchingAccount, err))
	}

	if len(accounts) == 0 {
		return notFound(ctx, fmt.Sprintf("%s: %s", errNoAccountsFound, accountID))
	}

	if len(accounts) > 1 {
		return indexerError(ctx, fmt.Sprintf("%s: %s", errMultipleAccounts, accountID))
	}

	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	return ctx.JSON(http.StatusOK, generated.AccountResponse{
		CurrentRound: round,
		Account:      accounts[0],
	})
}

// SearchForAccounts returns accounts matching the provided parameters
// (GET /v2/accounts)
func (si *ServerImplementation) SearchForAccounts(ctx echo.Context, params generated.SearchForAccountsParams) error {
	if !si.EnableAddressSearchRoundRewind && params.Round != nil {
		return badRequest(ctx, errMultiAcctRewind)
	}

	spendingAddr, errors := decodeAddress(params.AuthAddr, "account-id", make([]string, 0))
	if len(errors) != 0 {
		return badRequest(ctx, errors[0])
	}

	options := idb.AccountQueryOptions{
		IncludeAssetHoldings: true,
		IncludeAssetParams:   true,
		Limit:                min(uintOrDefaultValue(params.Limit, defaultAccountsLimit), maxAccountsLimit),
		HasAssetId:           uintOrDefault(params.AssetId),
		HasAppId:             uintOrDefault(params.ApplicationId),
		EqualToAuthAddr:      spendingAddr[:],
	}

	// Set GT/LT on Algos or Asset depending on whether or not an assetID was specified
	if options.HasAssetId == 0 {
		options.AlgosGreaterThan = uintOrDefault(params.CurrencyGreaterThan)
		options.AlgosLessThan = uintOrDefault(params.CurrencyLessThan)
	} else {
		options.AssetGT = uintOrDefault(params.CurrencyGreaterThan)
		options.AssetLT = uintOrDefault(params.CurrencyLessThan)
	}

	if params.Next != nil {
		addr, err := types.DecodeAddress(*params.Next)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, errUnableToParseNext)
		}
		options.GreaterThanAddress = addr[:]
	}

	accounts, err := si.fetchAccounts(ctx.Request().Context(), options, params.Round)

	if err != nil {
		return indexerError(ctx, fmt.Sprintf("%s: %v", errFailedSearchingAccount, err))
	}

	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
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

// (GET /v2/applications)
func (si *ServerImplementation) SearchForApplications(ctx echo.Context, params generated.SearchForApplicationsParams) error {
	results := si.db.Applications(ctx.Request().Context(), &params)
	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}
	apps := make([]generated.Application, 0)
	for result := range results {
		if result.Error != nil {
			return indexerError(ctx, result.Error.Error())
		}
		apps = append(apps, result.Application)
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

// (GET /v2/applications/{application-id})
func (si *ServerImplementation) LookupApplicationByID(ctx echo.Context, applicationId uint64) error {
	var params generated.SearchForApplicationsParams
	params.ApplicationId = &applicationId
	results := si.db.Applications(ctx.Request().Context(), &params)
	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}
	out := generated.ApplicationResponse{
		CurrentRound: round,
	}
	for result := range results {
		if result.Error != nil {
			return indexerError(ctx, result.Error.Error())
		}
		out.Application = &result.Application
		return ctx.JSON(http.StatusOK, out)
	}
	return ctx.JSON(http.StatusNotFound, out)
}

// LookupAssetByID looks up a particular asset
// (GET /v2/assets/{asset-id})
func (si *ServerImplementation) LookupAssetByID(ctx echo.Context, assetID uint64) error {
	search := generated.SearchForAssetsParams{
		AssetId: uint64Ptr(assetID),
		Limit:   uint64Ptr(1),
	}
	options, err := assetParamsToAssetQuery(search)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	assets, err := si.fetchAssets(ctx.Request().Context(), options)
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	if len(assets) == 0 {
		return notFound(ctx, fmt.Sprintf("%s: %d", errNoAssetsFound, assetID))
	}

	if len(assets) > 1 {
		return indexerError(ctx, fmt.Sprintf("%s: %d", errMultipleAssets, assetID))
	}

	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	return ctx.JSON(http.StatusOK, generated.AssetResponse{
		Asset:        assets[0],
		CurrentRound: round,
	})
}

// LookupAssetBalances looks up balances for a particular asset
// (GET /v2/assets/{asset-id}/balances)
func (si *ServerImplementation) LookupAssetBalances(ctx echo.Context, assetID uint64, params generated.LookupAssetBalancesParams) error {
	query := idb.AssetBalanceQuery{
		AssetId:  assetID,
		AmountGT: uintOrDefault(params.CurrencyGreaterThan),
		AmountLT: uintOrDefault(params.CurrencyLessThan),
		Limit:    min(uintOrDefaultValue(params.Limit, defaultBalancesLimit), maxBalancesLimit),
	}

	if params.Next != nil {
		addr, err := types.DecodeAddress(*params.Next)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, errUnableToParseNext)
		}
		query.PrevAddress = addr[:]
	}

	balances, err := si.fetchAssetBalances(ctx.Request().Context(), query)
	if err != nil {
		indexerError(ctx, err.Error())
	}

	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
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
	options, err := assetParamsToAssetQuery(params)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	assets, err := si.fetchAssets(ctx.Request().Context(), options)
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
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
	blk, err := si.fetchBlock(roundNumber)
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	// Lookup transactions
	filter := idb.TransactionFilter{Round: uint64Ptr(roundNumber)}
	txns, _, err := si.fetchTransactions(ctx.Request().Context(), filter)
	if err != nil {
		return indexerError(ctx, fmt.Sprintf("%s for round '%d': %v", errTransactionSearch, roundNumber, err))
	}

	blk.Transactions = &txns
	return ctx.JSON(http.StatusOK, generated.BlockResponse(blk))
}

func (si *ServerImplementation) LookupTransactions(ctx echo.Context, txid string) error {
	filter, err := transactionParamsToTransactionFilter(generated.SearchForTransactionsParams{
		Txid: strPtr(txid),
	})
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	// Fetch the transactions
	txns, _, err := si.fetchTransactions(ctx.Request().Context(), filter)
	if err != nil {
		return indexerError(ctx, fmt.Sprintf("%s: %v", errTransactionSearch, err))
	}

	if len(txns) == 0 {
		return notFound(ctx, fmt.Sprintf("%s: %s", errNoTransactionFound, txid))
	}

	if len(txns) > 1 {
		return indexerError(ctx, fmt.Sprintf("%s: %s", errMultipleTransactions, txid))
	}

	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	response := generated.TransactionResponse{
		CurrentRound: round,
		Transaction: txns[0],
	}

	return ctx.JSON(http.StatusOK, response)
}


// SearchForTransactions returns transactions matching the provided parameters
// (GET /v2/transactions)
func (si *ServerImplementation) SearchForTransactions(ctx echo.Context, params generated.SearchForTransactionsParams) error {
	filter, err := transactionParamsToTransactionFilter(params)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	// Fetch the transactions
	txns, next, err := si.fetchTransactions(ctx.Request().Context(), filter)
	if err != nil {
		return indexerError(ctx, fmt.Sprintf("%s: %v", errTransactionSearch, err))
	}

	round, err := si.db.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
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

// return a 500
func indexerError(ctx echo.Context, err string) error {
	return ctx.JSON(http.StatusInternalServerError, generated.ErrorResponse{
		Message: err,
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

// fetchAssets fetches all results and converts them into generated.Asset objects
func (si *ServerImplementation) fetchAssets(ctx context.Context, options idb.AssetsQuery) ([]generated.Asset, error) {
	assetchan := si.db.Assets(ctx, options)
	assets := make([]generated.Asset, 0)
	for row := range assetchan {
		if row.Error != nil {
			return nil, row.Error
		}

		creator := types.Address{}
		if len(row.Creator) != len(creator) {
			return nil, fmt.Errorf(errInvalidCreatorAddress)
		}
		copy(creator[:], row.Creator[:])

		asset := generated.Asset{
			Index: row.AssetId,
			Params: generated.AssetParams{
				Creator:       creator.String(),
				Name:          strPtr(row.Params.AssetName),
				UnitName:      strPtr(row.Params.UnitName),
				Url:           strPtr(row.Params.URL),
				Total:         row.Params.Total,
				Decimals:      uint64(row.Params.Decimals),
				DefaultFrozen: boolPtr(row.Params.DefaultFrozen),
				MetadataHash:  bytePtr(row.Params.MetadataHash[:]),
				Clawback:      strPtr(row.Params.Clawback.String()),
				Reserve:       strPtr(row.Params.Reserve.String()),
				Freeze:        strPtr(row.Params.Freeze.String()),
				Manager:       strPtr(row.Params.Manager.String()),
			},
		}

		assets = append(assets, asset)
	}
	return assets, nil
}

// fetchAssetBalances fetches all balances from a query and converts them into
// generated.MiniAssetHolding objects
func (si *ServerImplementation) fetchAssetBalances(ctx context.Context, options idb.AssetBalanceQuery) ([]generated.MiniAssetHolding, error) {
	assetbalchan := si.db.AssetBalances(ctx, options)
	balances := make([]generated.MiniAssetHolding, 0)
	for row := range assetbalchan {
		if row.Error != nil {
			return nil, row.Error
		}

		addr := types.Address{}
		if len(row.Address) != len(addr) {
			return nil, fmt.Errorf(errInvalidCreatorAddress)
		}
		copy(addr[:], row.Address[:])

		bal := generated.MiniAssetHolding{
			Address:  addr.String(),
			Amount:   row.Amount,
			IsFrozen: row.Frozen,
		}

		balances = append(balances, bal)
	}

	return balances, nil
}

// fetchBlock looks up a block and converts it into a generated.Block object
func (si *ServerImplementation) fetchBlock(round uint64) (generated.Block, error) {
	blk, err := si.db.GetBlock(round)
	if err != nil {
		return generated.Block{}, fmt.Errorf("%s '%d': %v", errLookingUpBlock, round, err)
	}

	rewards := generated.BlockRewards{
		FeeSink:                 "",
		RewardsCalculationRound: uint64(blk.RewardsRecalculationRound),
		RewardsLevel:            blk.RewardsLevel,
		RewardsPool:             blk.RewardsPool.String(),
		RewardsRate:             blk.RewardsRate,
		RewardsResidue:          blk.RewardsResidue,
	}

	upgradeState := generated.BlockUpgradeState{
		CurrentProtocol:        string(blk.CurrentProtocol),
		NextProtocol:           strPtr(string(blk.NextProtocol)),
		NextProtocolApprovals:  uint64Ptr(blk.NextProtocolApprovals),
		NextProtocolSwitchOn:   uint64Ptr(uint64(blk.NextProtocolSwitchOn)),
		NextProtocolVoteBefore: uint64Ptr(uint64(blk.NextProtocolVoteBefore)),
	}

	upgradeVote := generated.BlockUpgradeVote{
		UpgradeApprove: boolPtr(blk.UpgradeApprove),
		UpgradeDelay:   uint64Ptr(uint64(blk.UpgradeDelay)),
		UpgradePropose: strPtr(string(blk.UpgradePropose)),
	}

	ret := generated.Block{
		GenesisHash:       blk.GenesisHash[:],
		GenesisId:         blk.GenesisID,
		PreviousBlockHash: blk.Branch[:],
		Rewards:           &rewards,
		Round:             uint64(blk.Round),
		Seed:              blk.Seed[:],
		Timestamp:         uint64(blk.TimeStamp),
		Transactions:      nil,
		TransactionsRoot:  blk.TxnRoot[:],
		TxnCounter:        uint64Ptr(blk.TxnCounter),
		UpgradeState:      &upgradeState,
		UpgradeVote:       &upgradeVote,
	}

	return ret, nil
}

// fetchAccounts queries for accounts and converts them into generated.Account
// objects, optionally rewinding their value back to a particular round.
func (si *ServerImplementation) fetchAccounts(ctx context.Context, options idb.AccountQueryOptions, atRound *uint64) ([]generated.Account, error) {
	accountchan := si.db.GetAccounts(ctx, options)

	accounts := make([]generated.Account, 0)
	for row := range accountchan {
		if row.Error != nil {
			return nil, row.Error
		}

		// Compute for a given round if requested.
		var account generated.Account
		if atRound != nil {
			acct, err := accounting.AccountAtRound(row.Account, *atRound, si.db)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", errRewindingAccount, err)
			}
			account = acct
		} else {
			account = row.Account
		}

		accounts = append(accounts, account)
	}

	return accounts, nil
}

// fetchTransactions is used to query the backend for transactions, and compute the next token
func (si *ServerImplementation) fetchTransactions(ctx context.Context, filter idb.TransactionFilter) ([]generated.Transaction, string, error) {
	results := make([]generated.Transaction, 0)
	txchan := si.db.Transactions(ctx, filter)
	nextToken := ""
	for txrow := range txchan {
		tx, err := txnRowToTransaction(txrow)
		if err != nil {
			return nil, "", err
		}
		results = append(results, tx)
		nextToken = txrow.Next()
	}

	return results, nextToken, nil
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

func max(x, y uint64) uint64 {
	if x > y {
		return x
	}
	return y
}
