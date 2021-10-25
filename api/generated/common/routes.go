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
