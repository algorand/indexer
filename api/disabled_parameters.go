package api

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
)

// EndpointConfig is a data structure that contains whether the
// endpoint is disabled (with a boolean) as well as a set that
// contains disabled optional parameters.  The disabled optional parameter
// set is keyed by the name of the variable
type EndpointConfig struct {
	EndpointDisabled           bool
	DisabledOptionalParameters map[string]bool
}

// NewEndpointConfig creates a new empty endpoint config
func NewEndpointConfig() *EndpointConfig {
	rval := &EndpointConfig{
		EndpointDisabled:           false,
		DisabledOptionalParameters: make(map[string]bool),
	}

	return rval
}

// DisabledMap a type that holds a map of disabled types
// The key for a disabled map is the handler function name
type DisabledMap struct {
	Data map[string]*EndpointConfig
}

// NewDisabledMap creates a new empty disabled map
func NewDisabledMap() *DisabledMap {
	return &DisabledMap{
		Data: make(map[string]*EndpointConfig),
	}
}

// NewDisabledMapFromOA3 Creates a new disabled map from an openapi3 definition
func NewDisabledMapFromOA3(swag *openapi3.Swagger) *DisabledMap {
	rval := NewDisabledMap()
	for _, item := range swag.Paths {
		for _, opItem := range item.Operations() {

			endpointConfig := NewEndpointConfig()

			for _, pref := range opItem.Parameters {

				// TODO how to enable it to be disabled
				parameterIsDisabled := false
				if !parameterIsDisabled {
					// If the parameter is not disabled, then we don't need
					// to do anything
					continue
				}

				if pref.Value.Required {
					// If an endpoint config required parameter is disabled, then the whole endpoint is disabled
					endpointConfig.EndpointDisabled = true
				} else {
					// If the optional parameter is disabled, add it to the map
					endpointConfig.DisabledOptionalParameters[pref.Value.Name] = true
				}
			}

			rval.Data[opItem.OperationID] = endpointConfig

		}

	}

	return rval
}

// VerifyRC is the return code for the Verify function
type VerifyRC int

const (
	verifyIsGood VerifyRC = iota
	verifyFailedEndpoint
	verifyFailedParameter
)

// DisabledParameterErrorReporter defines an error reporting interface
// for the Verify functions
type DisabledParameterErrorReporter interface {
	Errorf(format string, args ...interface{})
}

// Verify returns verifyIsGood if the function can continue (i.e. the parameters are valid and disabled
// parameters are not supplied), otherwise verifyFailedEndpoint if the endpoint failed and
// verifyFailedParameter if a disabled parameter was provided.  If verifyFailedParameter is returned
// then the variable name that caused it is provided.  In all other cases, it is empty
func Verify(dm *DisabledMap, nameOfHandlerFunc string, ctx echo.Context, log DisabledParameterErrorReporter) (VerifyRC, string) {

	if dm == nil || dm.Data == nil {
		return verifyIsGood, ""
	}

	if val, ok := dm.Data[nameOfHandlerFunc]; ok {
		return val.verify(ctx, log)
	}

	// If the function name wasn't in the map something got messed up....
	log.Errorf("verify function could not find name of handler function in map: %s", nameOfHandlerFunc)
	// We want to fail-safe to not stop the indexer
	return verifyIsGood, ""
}

func (ec *EndpointConfig) verify(ctx echo.Context, log DisabledParameterErrorReporter) (VerifyRC, string) {

	if ec.EndpointDisabled {
		return verifyFailedEndpoint, ""
	}

	queryParams := ctx.QueryParams()
	formParams, formErr := ctx.FormParams()

	if formErr != nil {
		log.Errorf("retrieving form parameters for verification resulted in an error: %v", formErr)
	}

	for paramName := range ec.DisabledOptionalParameters {

		// The optional param is disabled, check that it wasn't supplied...
		queryValue := queryParams.Get(paramName)
		if queryValue != "" {
			// If the query value is non-zero, and it was disabled, we should return false
			return verifyFailedParameter, paramName
		}

		if formErr != nil {
			continue
		}

		formValue := formParams.Get(paramName)
		if formValue != "" {
			// If the query value is non-zero, and it was disabled, we should return false
			return verifyFailedParameter, paramName
		}
	}

	return verifyIsGood, ""
}
