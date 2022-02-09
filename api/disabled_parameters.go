package api

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
)

// DisabledParameter holds the name and disabled status of a parameter
type DisabledParameter struct {
	Name     string
	Disabled bool
}

// DisabledList is a data structure that contains lists of both
// required and optional parameters.  By storing these, one can
// determine if any disabled parameters have been supplied
type DisabledList struct {
	RequiredParams []DisabledParameter
	OptionalParams []DisabledParameter
}

// DisabledMap a type that holds a map of disabled types
// The key for a disabled map is the handler function name
type DisabledMap struct {
	Data map[string]DisabledList
}

// NewDisabledMap creates a new empty disabled map
func NewDisabledMap() *DisabledMap {
	rval := &DisabledMap{}
	rval.Data = make(map[string]DisabledList)
	return rval
}

// NewDisabledMapFromOA3 Creates a new disabled map from an openapi3 definition
func NewDisabledMapFromOA3(swag *openapi3.Swagger) *DisabledMap {
	rval := NewDisabledMap()
	for _, item := range swag.Paths {
		for _, opItem := range item.Operations() {
			var disabledList DisabledList

			for _, pref := range opItem.Parameters {

				disabledParameter := DisabledParameter{
					Name:     pref.Value.Name,
					Disabled: false,
				}
				// TODO how to enable it to be disabled

				if pref.Value.Required {
					disabledList.RequiredParams = append(disabledList.RequiredParams, disabledParameter)
					fmt.Printf(" -> Param (Req): %s\n", pref.Value.Name)
				} else {
					disabledList.OptionalParams = append(disabledList.OptionalParams, disabledParameter)
					fmt.Printf(" -> Param: %s\n", pref.Value.Name)
				}
			}

			rval.Data[opItem.OperationID] = disabledList

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

	if dm == nil {
		return verifyIsGood, ""
	}

	if dm.Data == nil {
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

func (dl *DisabledList) verify(ctx echo.Context, log DisabledParameterErrorReporter) (VerifyRC, string) {

	// If any of the Required Params are disabled we have to fail the whole
	// endpoint
	for _, dp := range dl.RequiredParams {
		if dp.Disabled {
			return verifyFailedEndpoint, ""
		}
	}

	queryParams := ctx.QueryParams()
	formParams, formErr := ctx.FormParams()

	if formErr != nil {
		log.Errorf("retrieving form parameters for verification resulted in an error: %v", formErr)
	}

	for _, dp := range dl.OptionalParams {

		// No point in checking if it isn't disabled
		if dp.Disabled == false {
			continue
		}

		// The optional param is disabled, check that it wasn't supplied...
		queryValue := queryParams.Get(dp.Name)
		if queryValue != "" {
			// If the query value is non-zero, and it was disabled, we should return false
			return verifyFailedParameter, dp.Name
		}

		if formErr != nil {
			continue
		}

		formValue := formParams.Get(dp.Name)
		if formValue != "" {
			// If the query value is non-zero, and it was disabled, we should return false
			return verifyFailedParameter, dp.Name
		}
	}

	return verifyIsGood, ""
}
