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
	LookupAccountByID(ctx echo.Context, accountId uint64, params LookupAccountByIDParams) error

	// (GET /account/{account-id}/transactions)
	LookupAccountTransactions(ctx echo.Context, accountId string, params LookupAccountTransactionsParams) error

	// (GET /accounts)
	SearchAccounts(ctx echo.Context, params SearchAccountsParams) error

	// (GET /asset/{asset-id})
	LookupAssetByID(ctx echo.Context, assetId string) error

	// (GET /asset/{asset-id}/balances)
	LookupAssetBalances(ctx echo.Context, assetId string, params LookupAssetBalancesParams) error

	// (GET /asset/{asset-id}/transactions)
	LookupAssetTransactions(ctx echo.Context, assetId string, params LookupAssetTransactionsParams) error

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
	var accountId uint64

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
	var assetId string

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
	var assetId string

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
	var assetId string

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
