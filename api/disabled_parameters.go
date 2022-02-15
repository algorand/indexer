package api

import (
	"fmt"

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

// ErrVerifyFailedEndpoint an error that signifies that the entire endpoint is disabled
var ErrVerifyFailedEndpoint error = fmt.Errorf("endpoint is disabled")

// ErrVerifyFailedParameter an error that signifies that a parameter was provided when it was disabled
type ErrVerifyFailedParameter struct {
	ParameterName string
}

func (evfp ErrVerifyFailedParameter) Error() string {
	return fmt.Sprintf("provided disabled parameter: %s", evfp.ParameterName)
}

// DisabledParameterErrorReporter defines an error reporting interface
// for the Verify functions
type DisabledParameterErrorReporter interface {
	Errorf(format string, args ...interface{})
}

// Verify returns nil if the function can continue (i.e. the parameters are valid and disabled
// parameters are not supplied), otherwise VerifyFailedEndpoint if the endpoint failed and
// VerifyFailedParameter if a disabled parameter was provided.
func Verify(dm *DisabledMap, nameOfHandlerFunc string, ctx echo.Context, log DisabledParameterErrorReporter) error {

	if dm == nil || dm.Data == nil {
		return nil
	}

	if val, ok := dm.Data[nameOfHandlerFunc]; ok {
		return val.verify(ctx, log)
	}

	// If the function name wasn't in the map something got messed up....
	log.Errorf("verify function could not find name of handler function in map: %s", nameOfHandlerFunc)
	// We want to fail-safe to not stop the indexer
	return nil
}

func (ec *EndpointConfig) verify(ctx echo.Context, log DisabledParameterErrorReporter) error {

	if ec.EndpointDisabled {
		return ErrVerifyFailedEndpoint
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
			return ErrVerifyFailedParameter{paramName}
		}

		if formErr != nil {
			continue
		}

		formValue := formParams.Get(paramName)
		if formValue != "" {
			// If the query value is non-zero, and it was disabled, we should return false
			return ErrVerifyFailedParameter{paramName}
		}
	}

	return nil
}
