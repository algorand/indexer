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
