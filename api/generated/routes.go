// Package generated provides primitives to interact the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen DO NOT EDIT.
package generated

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"github.com/deepmap/oapi-codegen/pkg/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

// ServerInterface represents all server handlers.
type ServerInterface interface {

	// (GET /account/{account-id})
	LookupAccountByID(ctx echo.Context, accountId string, params LookupAccountByIDParams) error

	// (GET /account/{account-id}/transactions)
	LookupAccountTransactions(ctx echo.Context, accountId string, params LookupAccountTransactionsParams) error

	// (GET /accounts)
	SearchAccounts(ctx echo.Context, params SearchAccountsParams) error

	// (GET /asset/{asset-id})
	LookupAssetByID(ctx echo.Context, assetId uint64) error

	// (GET /asset/{asset-id}/balances)
	LookupAssetBalances(ctx echo.Context, assetId uint64, params LookupAssetBalancesParams) error

	// (GET /asset/{asset-id}/transactions)
	LookupAssetTransactions(ctx echo.Context, assetId uint64, params LookupAssetTransactionsParams) error

	// (GET /assets)
	SearchForAssets(ctx echo.Context, params SearchForAssetsParams) error

	// (GET /block/{round-number})
	LookupBlock(ctx echo.Context, roundNumber uint64) error

	// (GET /blocktimes)
	LookupBlockTimes(ctx echo.Context) error

	// (GET /transactions)
	SearchForTransactions(ctx echo.Context, params SearchForTransactionsParams) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// LookupAccountByID converts echo context to params.
func (w *ServerInterfaceWrapper) LookupAccountByID(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "account-id" -------------
	var accountId string

	err = runtime.BindStyledParameter("simple", false, "account-id", ctx.Param("account-id"), &accountId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter account-id: %s", err))
	}

	// Parameter object where we will unmarshal all parameters from the context
	var params LookupAccountByIDParams
	// ------------- Optional query parameter "round" -------------
	if paramValue := ctx.QueryParam("round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "round", ctx.QueryParams(), &params.Round)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter round: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.LookupAccountByID(ctx, accountId, params)
	return err
}

// LookupAccountTransactions converts echo context to params.
func (w *ServerInterfaceWrapper) LookupAccountTransactions(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "account-id" -------------
	var accountId string

	err = runtime.BindStyledParameter("simple", false, "account-id", ctx.Param("account-id"), &accountId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter account-id: %s", err))
	}

	// Parameter object where we will unmarshal all parameters from the context
	var params LookupAccountTransactionsParams
	// ------------- Optional query parameter "min-round" -------------
	if paramValue := ctx.QueryParam("min-round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "min-round", ctx.QueryParams(), &params.MinRound)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter min-round: %s", err))
	}

	// ------------- Optional query parameter "max-round" -------------
	if paramValue := ctx.QueryParam("max-round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "max-round", ctx.QueryParams(), &params.MaxRound)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max-round: %s", err))
	}

	// ------------- Optional query parameter "max-ts" -------------
	if paramValue := ctx.QueryParam("max-ts"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "max-ts", ctx.QueryParams(), &params.MaxTs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max-ts: %s", err))
	}

	// ------------- Optional query parameter "min-ts" -------------
	if paramValue := ctx.QueryParam("min-ts"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "min-ts", ctx.QueryParams(), &params.MinTs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter min-ts: %s", err))
	}

	// ------------- Optional query parameter "asset" -------------
	if paramValue := ctx.QueryParam("asset"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "asset", ctx.QueryParams(), &params.Asset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset: %s", err))
	}

	// ------------- Optional query parameter "offset" -------------
	if paramValue := ctx.QueryParam("offset"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "offset", ctx.QueryParams(), &params.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter offset: %s", err))
	}

	// ------------- Optional query parameter "limit" -------------
	if paramValue := ctx.QueryParam("limit"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "limit", ctx.QueryParams(), &params.Limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter limit: %s", err))
	}

	// ------------- Optional query parameter "gt" -------------
	if paramValue := ctx.QueryParam("gt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "gt", ctx.QueryParams(), &params.Gt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter gt: %s", err))
	}

	// ------------- Optional query parameter "lt" -------------
	if paramValue := ctx.QueryParam("lt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "lt", ctx.QueryParams(), &params.Lt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter lt: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.LookupAccountTransactions(ctx, accountId, params)
	return err
}

// SearchAccounts converts echo context to params.
func (w *ServerInterfaceWrapper) SearchAccounts(ctx echo.Context) error {
	var err error

	// Parameter object where we will unmarshal all parameters from the context
	var params SearchAccountsParams
	// ------------- Optional query parameter "asset-id" -------------
	if paramValue := ctx.QueryParam("asset-id"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "asset-id", ctx.QueryParams(), &params.AssetId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset-id: %s", err))
	}

	// ------------- Optional query parameter "assetParams" -------------
	if paramValue := ctx.QueryParam("assetParams"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "assetParams", ctx.QueryParams(), &params.AssetParams)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter assetParams: %s", err))
	}

	// ------------- Optional query parameter "limit" -------------
	if paramValue := ctx.QueryParam("limit"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "limit", ctx.QueryParams(), &params.Limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter limit: %s", err))
	}

	// ------------- Optional query parameter "offset" -------------
	if paramValue := ctx.QueryParam("offset"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "offset", ctx.QueryParams(), &params.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter offset: %s", err))
	}

	// ------------- Optional query parameter "gt" -------------
	if paramValue := ctx.QueryParam("gt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "gt", ctx.QueryParams(), &params.Gt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter gt: %s", err))
	}

	// ------------- Optional query parameter "lt" -------------
	if paramValue := ctx.QueryParam("lt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "lt", ctx.QueryParams(), &params.Lt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter lt: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.SearchAccounts(ctx, params)
	return err
}

// LookupAssetByID converts echo context to params.
func (w *ServerInterfaceWrapper) LookupAssetByID(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "asset-id" -------------
	var assetId uint64

	err = runtime.BindStyledParameter("simple", false, "asset-id", ctx.Param("asset-id"), &assetId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset-id: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.LookupAssetByID(ctx, assetId)
	return err
}

// LookupAssetBalances converts echo context to params.
func (w *ServerInterfaceWrapper) LookupAssetBalances(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "asset-id" -------------
	var assetId uint64

	err = runtime.BindStyledParameter("simple", false, "asset-id", ctx.Param("asset-id"), &assetId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset-id: %s", err))
	}

	// Parameter object where we will unmarshal all parameters from the context
	var params LookupAssetBalancesParams
	// ------------- Optional query parameter "limit" -------------
	if paramValue := ctx.QueryParam("limit"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "limit", ctx.QueryParams(), &params.Limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter limit: %s", err))
	}

	// ------------- Optional query parameter "offset" -------------
	if paramValue := ctx.QueryParam("offset"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "offset", ctx.QueryParams(), &params.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter offset: %s", err))
	}

	// ------------- Optional query parameter "round" -------------
	if paramValue := ctx.QueryParam("round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "round", ctx.QueryParams(), &params.Round)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter round: %s", err))
	}

	// ------------- Optional query parameter "gt" -------------
	if paramValue := ctx.QueryParam("gt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "gt", ctx.QueryParams(), &params.Gt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter gt: %s", err))
	}

	// ------------- Optional query parameter "lt" -------------
	if paramValue := ctx.QueryParam("lt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "lt", ctx.QueryParams(), &params.Lt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter lt: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.LookupAssetBalances(ctx, assetId, params)
	return err
}

// LookupAssetTransactions converts echo context to params.
func (w *ServerInterfaceWrapper) LookupAssetTransactions(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "asset-id" -------------
	var assetId uint64

	err = runtime.BindStyledParameter("simple", false, "asset-id", ctx.Param("asset-id"), &assetId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset-id: %s", err))
	}

	// Parameter object where we will unmarshal all parameters from the context
	var params LookupAssetTransactionsParams
	// ------------- Optional query parameter "limit" -------------
	if paramValue := ctx.QueryParam("limit"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "limit", ctx.QueryParams(), &params.Limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter limit: %s", err))
	}

	// ------------- Optional query parameter "min-round" -------------
	if paramValue := ctx.QueryParam("min-round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "min-round", ctx.QueryParams(), &params.MinRound)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter min-round: %s", err))
	}

	// ------------- Optional query parameter "max-round" -------------
	if paramValue := ctx.QueryParam("max-round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "max-round", ctx.QueryParams(), &params.MaxRound)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max-round: %s", err))
	}

	// ------------- Optional query parameter "min-ts" -------------
	if paramValue := ctx.QueryParam("min-ts"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "min-ts", ctx.QueryParams(), &params.MinTs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter min-ts: %s", err))
	}

	// ------------- Optional query parameter "max-ts" -------------
	if paramValue := ctx.QueryParam("max-ts"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "max-ts", ctx.QueryParams(), &params.MaxTs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max-ts: %s", err))
	}

	// ------------- Optional query parameter "offset" -------------
	if paramValue := ctx.QueryParam("offset"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "offset", ctx.QueryParams(), &params.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter offset: %s", err))
	}

	// ------------- Optional query parameter "gt" -------------
	if paramValue := ctx.QueryParam("gt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "gt", ctx.QueryParams(), &params.Gt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter gt: %s", err))
	}

	// ------------- Optional query parameter "lt" -------------
	if paramValue := ctx.QueryParam("lt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "lt", ctx.QueryParams(), &params.Lt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter lt: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.LookupAssetTransactions(ctx, assetId, params)
	return err
}

// SearchForAssets converts echo context to params.
func (w *ServerInterfaceWrapper) SearchForAssets(ctx echo.Context) error {
	var err error

	// Parameter object where we will unmarshal all parameters from the context
	var params SearchForAssetsParams
	// ------------- Optional query parameter "gt" -------------
	if paramValue := ctx.QueryParam("gt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "gt", ctx.QueryParams(), &params.Gt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter gt: %s", err))
	}

	// ------------- Optional query parameter "limit" -------------
	if paramValue := ctx.QueryParam("limit"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "limit", ctx.QueryParams(), &params.Limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter limit: %s", err))
	}

	// ------------- Optional query parameter "creator" -------------
	if paramValue := ctx.QueryParam("creator"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "creator", ctx.QueryParams(), &params.Creator)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter creator: %s", err))
	}

	// ------------- Optional query parameter "name" -------------
	if paramValue := ctx.QueryParam("name"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "name", ctx.QueryParams(), &params.Name)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter name: %s", err))
	}

	// ------------- Optional query parameter "unit" -------------
	if paramValue := ctx.QueryParam("unit"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "unit", ctx.QueryParams(), &params.Unit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter unit: %s", err))
	}

	// ------------- Optional query parameter "offset" -------------
	if paramValue := ctx.QueryParam("offset"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "offset", ctx.QueryParams(), &params.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter offset: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.SearchForAssets(ctx, params)
	return err
}

// LookupBlock converts echo context to params.
func (w *ServerInterfaceWrapper) LookupBlock(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "round-number" -------------
	var roundNumber uint64

	err = runtime.BindStyledParameter("simple", false, "round-number", ctx.Param("round-number"), &roundNumber)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter round-number: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.LookupBlock(ctx, roundNumber)
	return err
}

// LookupBlockTimes converts echo context to params.
func (w *ServerInterfaceWrapper) LookupBlockTimes(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.LookupBlockTimes(ctx)
	return err
}

// SearchForTransactions converts echo context to params.
func (w *ServerInterfaceWrapper) SearchForTransactions(ctx echo.Context) error {
	var err error

	// Parameter object where we will unmarshal all parameters from the context
	var params SearchForTransactionsParams
	// ------------- Optional query parameter "noteprefix" -------------
	if paramValue := ctx.QueryParam("noteprefix"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "noteprefix", ctx.QueryParams(), &params.Noteprefix)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter noteprefix: %s", err))
	}

	// ------------- Optional query parameter "type" -------------
	if paramValue := ctx.QueryParam("type"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "type", ctx.QueryParams(), &params.Type)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter type: %s", err))
	}

	// ------------- Optional query parameter "sigtype" -------------
	if paramValue := ctx.QueryParam("sigtype"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "sigtype", ctx.QueryParams(), &params.Sigtype)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter sigtype: %s", err))
	}

	// ------------- Optional query parameter "txid" -------------
	if paramValue := ctx.QueryParam("txid"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "txid", ctx.QueryParams(), &params.Txid)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter txid: %s", err))
	}

	// ------------- Optional query parameter "round" -------------
	if paramValue := ctx.QueryParam("round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "round", ctx.QueryParams(), &params.Round)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter round: %s", err))
	}

	// ------------- Optional query parameter "offset" -------------
	if paramValue := ctx.QueryParam("offset"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "offset", ctx.QueryParams(), &params.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter offset: %s", err))
	}

	// ------------- Optional query parameter "min-round" -------------
	if paramValue := ctx.QueryParam("min-round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "min-round", ctx.QueryParams(), &params.MinRound)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter min-round: %s", err))
	}

	// ------------- Optional query parameter "max-round" -------------
	if paramValue := ctx.QueryParam("max-round"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "max-round", ctx.QueryParams(), &params.MaxRound)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max-round: %s", err))
	}

	// ------------- Optional query parameter "asset-id" -------------
	if paramValue := ctx.QueryParam("asset-id"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "asset-id", ctx.QueryParams(), &params.AssetId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset-id: %s", err))
	}

	// ------------- Optional query parameter "format" -------------
	if paramValue := ctx.QueryParam("format"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// ------------- Optional query parameter "limit" -------------
	if paramValue := ctx.QueryParam("limit"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "limit", ctx.QueryParams(), &params.Limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter limit: %s", err))
	}

	// ------------- Optional query parameter "max-ts" -------------
	if paramValue := ctx.QueryParam("max-ts"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "max-ts", ctx.QueryParams(), &params.MaxTs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max-ts: %s", err))
	}

	// ------------- Optional query parameter "min-ts" -------------
	if paramValue := ctx.QueryParam("min-ts"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "min-ts", ctx.QueryParams(), &params.MinTs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter min-ts: %s", err))
	}

	// ------------- Optional query parameter "gt" -------------
	if paramValue := ctx.QueryParam("gt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "gt", ctx.QueryParams(), &params.Gt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter gt: %s", err))
	}

	// ------------- Optional query parameter "lt" -------------
	if paramValue := ctx.QueryParam("lt"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "lt", ctx.QueryParams(), &params.Lt)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter lt: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.SearchForTransactions(ctx, params)
	return err
}

// RegisterHandlers adds each server route to the EchoRouter.
func RegisterHandlers(router interface {
	CONNECT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	DELETE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	HEAD(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	OPTIONS(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PATCH(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	POST(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PUT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	TRACE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
}, si ServerInterface) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.GET("/account/:account-id", wrapper.LookupAccountByID)
	router.GET("/account/:account-id/transactions", wrapper.LookupAccountTransactions)
	router.GET("/accounts", wrapper.SearchAccounts)
	router.GET("/asset/:asset-id", wrapper.LookupAssetByID)
	router.GET("/asset/:asset-id/balances", wrapper.LookupAssetBalances)
	router.GET("/asset/:asset-id/transactions", wrapper.LookupAssetTransactions)
	router.GET("/assets", wrapper.SearchForAssets)
	router.GET("/block/:round-number", wrapper.LookupBlock)
	router.GET("/blocktimes", wrapper.LookupBlockTimes)
	router.GET("/transactions", wrapper.SearchForTransactions)

}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file.
func GetSwagger() (*openapi3.Swagger, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %s", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}

	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error loading Swagger: %s", err)
	}
	return swagger, nil
}
