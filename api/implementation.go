package api

import (
	"errors"
	"github.com/algorand/indexer/api/generated"
	"github.com/labstack/echo/v4"
)

type ServerImplementation struct {}

// (GET /account/{account-id})
func (si *ServerImplementation) LookupAccountByID(ctx echo.Context, accountId string, params generated.LookupAccountByIDParams) error {
	/*
	options := AccountQueryOptions {
		//GreaterThanAddress []byte // for paging results
		//EqualToAddress     []byte // return exactly this one account

		//IncludeAssetHoldings bool
		//IncludeAssetParams   bool

		//Limit uint64
	}
	IndexerDb.get
	*/

	//accountchan := IndexerDb.GetAccounts(ctx.Request().Context(), accountId)
	return errors.New("Unimplemented")
}

// (GET /account/{account-id}/transactions)
func (si *ServerImplementation) LookupAccountTransactions(ctx echo.Context, accountId string, params generated.LookupAccountTransactionsParams) error {
	return errors.New("Unimplemented")
}

// (GET /accounts)
func (si *ServerImplementation) SearchAccounts(ctx echo.Context, params generated.SearchAccountsParams) error {
	return errors.New("Unimplemented")
}

// (GET /asset/{asset-id})
func (si *ServerImplementation) LookupAssetByID(ctx echo.Context, assetId uint64) error {
	return errors.New("Unimplemented")
}

// (GET /asset/{asset-id}/balances)
func (si *ServerImplementation) LookupAssetBalances(ctx echo.Context, assetId uint64, params generated.LookupAssetBalancesParams) error {
	return errors.New("Unimplemented")
}

// (GET /asset/{asset-id}/transactions)
func (si *ServerImplementation) LookupAssetTransactions(ctx echo.Context, assetId uint64, params generated.LookupAssetTransactionsParams) error {
	return errors.New("Unimplemented")
}

// (GET /assets)
func (si *ServerImplementation) SearchForAssets(ctx echo.Context, params generated.SearchForAssetsParams) error {
	return errors.New("Unimplemented")
}

// (GET /block/{round-number})
func (si *ServerImplementation) LookupBlock(ctx echo.Context, roundNumber uint64) error {
	return errors.New("Unimplemented")
}

// (GET /blocktimes)
func (si *ServerImplementation) LookupBlockTimes(ctx echo.Context) error {
	return errors.New("Unimplemented")
}

// (GET /transactions)
func (si *ServerImplementation) SearchForTransactions(ctx echo.Context, params generated.SearchForTransactionsParams) error {
	return errors.New("Unimplemented")
}
