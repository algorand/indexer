// Package common provides primitives to interact the openapi HTTP API.
//
// Code generated by github.com/algorand/oapi-codegen DO NOT EDIT.
package common

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// Returns 200 if healthy.
	// (GET /health)
	MakeHealthCheck(ctx echo.Context) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// MakeHealthCheck converts echo context to params.
func (w *ServerInterfaceWrapper) MakeHealthCheck(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.MakeHealthCheck(ctx)
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
}, si ServerInterface, m ...echo.MiddlewareFunc) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.GET("/health", wrapper.MakeHealthCheck, m...)

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
