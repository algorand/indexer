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
