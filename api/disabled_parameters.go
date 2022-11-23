package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

const (
	disabledStatusStr    = "disabled"
	enabledStatusStr     = "enabled"
	requiredParameterStr = "required"
	optionalParameterStr = "optional"
)

// DisplayDisabledMap is a struct that contains the necessary information
// to output the current config to the screen
type DisplayDisabledMap struct {
	// A complicated map but necessary to output the correct YAML.
	// This is supposed to represent a data structure with a similar YAML
	// representation:
	//  /v2/accounts/{account-id}:
	//    required:
	//      - account-id : enabled
	//  /v2/accounts:
	//    optional:
	//      - auth-addr : enabled
	//      - next: disabled
	Data map[string]map[string][]map[string]string
}

func (ddm *DisplayDisabledMap) String() (string, error) {

	if len(ddm.Data) == 0 {
		return "", nil
	}

	bytes, err := yaml.Marshal(ddm.Data)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func makeDisplayDisabledMap() *DisplayDisabledMap {
	return &DisplayDisabledMap{
		Data: make(map[string]map[string][]map[string]string),
	}
}

func (ddm *DisplayDisabledMap) addEntry(restPath string, requiredOrOptional string, entryName string, status string) {
	if ddm.Data == nil {
		ddm.Data = make(map[string]map[string][]map[string]string)
	}

	if ddm.Data[restPath] == nil {
		ddm.Data[restPath] = make(map[string][]map[string]string)
	}

	mapEntry := map[string]string{entryName: status}

	ddm.Data[restPath][requiredOrOptional] = append(ddm.Data[restPath][requiredOrOptional], mapEntry)
}

// validateSchema takes in a newly loaded DisplayDisabledMap and validates that all the
// "strings" are the correct values.  For instance, it says that the sub-config is either
// "required" or "optional" as well as making sure the string values for the parameters
// are either "enabled" or "disabled".
//
// However, it does not validate whether the values actually exist.  That comes with the "Validate"
// function on a DisabledMapConfig
func (ddm *DisplayDisabledMap) validateSchema() error {
	type innerStruct struct {
		// IllegalParamTypes: list of the mis-spelled parameter types (i.e. not required or optional)
		IllegalParamTypes []string
		// IllegalParamStatus: list of parameter names with mis-spelled parameter status combined as a string
		IllegalParamStatus []string
	}

	illegalSchema := make(map[string]innerStruct)

	for restPath, entries := range ddm.Data {
		tmp := innerStruct{}
		for requiredOrOptional, paramList := range entries {

			if requiredOrOptional != requiredParameterStr && requiredOrOptional != optionalParameterStr {
				tmp.IllegalParamTypes = append(tmp.IllegalParamTypes, requiredOrOptional)
			}

			for _, paramDict := range paramList {
				for paramName, paramStatus := range paramDict {
					if paramStatus != disabledStatusStr && paramStatus != enabledStatusStr {
						errorStr := fmt.Sprintf("%s : %s", paramName, paramStatus)
						tmp.IllegalParamStatus = append(tmp.IllegalParamStatus, errorStr)
					}
				}
			}

			if len(tmp.IllegalParamTypes) != 0 || len(tmp.IllegalParamStatus) != 0 {
				illegalSchema[restPath] = tmp
			}
		}
	}

	// No error if there are no entries
	if len(illegalSchema) == 0 {
		return nil
	}

	var sb strings.Builder

	for restPath, iStruct := range illegalSchema {
		_, _ = sb.WriteString(fmt.Sprintf("REST Path %s contained the following errors:\n", restPath))
		if len(iStruct.IllegalParamTypes) != 0 {
			_, _ = sb.WriteString(fmt.Sprintf("  -> Illegal Parameter Types: %v\n", iStruct.IllegalParamTypes))
		}
		if len(iStruct.IllegalParamStatus) != 0 {
			_, _ = sb.WriteString(fmt.Sprintf("  -> Illegal Parameter Status: %v\n", iStruct.IllegalParamStatus))
		}
	}

	return fmt.Errorf(sb.String())
}

// toDisabledMapConfig creates a disabled map config from a display disabled map.  If the swag pointer
// is nil then no validation is performed on the disabled map config.  This is useful for unit tests
func (ddm *DisplayDisabledMap) toDisabledMapConfig(swag *openapi3.T) (*DisabledMapConfig, error) {
	// Check that all the "strings" are valid
	err := ddm.validateSchema()
	if err != nil {
		return nil, err
	}

	// We now should have a correctly formed DisplayDisabledMap.
	// Let's turn that into a config
	dmc := MakeDisabledMapConfig()

	for restPath, entries := range ddm.Data {
		var disabledParams []string
		for _, paramList := range entries {
			// We don't care if they are required or optional, only if the are disabled
			for _, paramDict := range paramList {
				for paramName, paramStatus := range paramDict {
					if paramStatus != disabledStatusStr {
						continue
					}
					disabledParams = append(disabledParams, paramName)
				}
			}
		}

		// Default to just get for now
		dmc.addEntry(restPath, http.MethodGet, disabledParams)
	}

	if swag != nil {
		err = dmc.validate(swag)
		if err != nil {
			return nil, err
		}
	}

	return dmc, nil
}

// MakeDisabledMapConfigFromFile loads a file containing a disabled map configuration.
func MakeDisabledMapConfigFromFile(swag *openapi3.T, filePath string) (*DisabledMapConfig, error) {
	// First load the file...
	f, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	ddm := makeDisplayDisabledMap()

	err = yaml.Unmarshal(f, &ddm.Data)
	if err != nil {
		return nil, err
	}

	return ddm.toDisabledMapConfig(swag)
}

// MakeDisplayDisabledMapFromConfig will make a DisplayDisabledMap that takes into account the DisabledMapConfig.
// If limited is set to true, then only disabled parameters will be added to the DisplayDisabledMap
func MakeDisplayDisabledMapFromConfig(swag *openapi3.T, mapConfig *DisabledMapConfig, limited bool) *DisplayDisabledMap {

	rval := makeDisplayDisabledMap()
	for restPath, item := range swag.Paths {

		for opName, opItem := range item.Operations() {

			for _, pref := range opItem.Parameters {

				paramName := pref.Value.Name

				parameterIsDisabled := mapConfig.isDisabled(restPath, opName, paramName)
				// If we are limited, then don't bother with enabled parameters
				if !parameterIsDisabled && limited {
					// If the parameter is not disabled, then we don't need
					// to do anything
					continue
				}

				var statusStr string

				if parameterIsDisabled {
					statusStr = disabledStatusStr
				} else {
					statusStr = enabledStatusStr
				}

				if pref.Value.Required {
					rval.addEntry(restPath, requiredParameterStr, paramName, statusStr)
				} else {
					// If the optional parameter is disabled, add it to the map
					rval.addEntry(restPath, optionalParameterStr, paramName, statusStr)
				}
			}

		}

	}

	return rval
}

// EndpointConfig is a data structure that contains whether the
// endpoint is disabled (with a boolean) as well as a set that
// contains disabled optional parameters.  The disabled optional parameter
// set is keyed by the name of the variable
type EndpointConfig struct {
	EndpointDisabled           bool
	DisabledOptionalParameters map[string]bool
}

// makeEndpointConfig creates a new empty endpoint config
func makeEndpointConfig() *EndpointConfig {
	rval := &EndpointConfig{
		EndpointDisabled:           false,
		DisabledOptionalParameters: make(map[string]bool),
	}

	return rval
}

// DisabledMap a type that holds a map of disabled types
// The key for a disabled map is the handler function name
type DisabledMap struct {
	// Key -> Function Name/Operation ID
	Data map[string]*EndpointConfig
}

// MakeDisabledMap creates a new empty disabled map
func MakeDisabledMap() *DisabledMap {
	return &DisabledMap{
		Data: make(map[string]*EndpointConfig),
	}
}

// DisabledMapConfig is a type that holds the configuration for setting up
// a DisabledMap
type DisabledMapConfig struct {
	// Key -> Path of REST endpoint (i.e. /v2/accounts/{account-id}/transactions)
	// Value -> Operation "get, post, etc" -> Sub-value: List of parameters disabled for that endpoint
	Data map[string]map[string][]string
}

// isDisabled Returns true if the parameter is disabled for the given path
func (dmc *DisabledMapConfig) isDisabled(restPath string, operationName string, parameterName string) bool {
	parameterList, exists := dmc.Data[restPath][operationName]
	if !exists {
		return false
	}

	for _, parameter := range parameterList {
		if parameterName == parameter {
			return true
		}
	}
	return false
}

func (dmc *DisabledMapConfig) addEntry(restPath string, operationName string, parameterNames []string) {

	if dmc.Data == nil {
		dmc.Data = make(map[string]map[string][]string)
	}

	if dmc.Data[restPath] == nil {
		dmc.Data[restPath] = make(map[string][]string)
	}

	dmc.Data[restPath][operationName] = parameterNames
}

// MakeDisabledMapConfig creates a new disabled map configuration with everything enabled
func MakeDisabledMapConfig() *DisabledMapConfig {
	return &DisabledMapConfig{
		Data: make(map[string]map[string][]string),
	}
}

// GetDefaultDisabledMapConfigForPostgres will generate a configuration that will block certain
// parameters.  Should be used only for the postgres implementation
func GetDefaultDisabledMapConfigForPostgres() *DisabledMapConfig {
	rval := MakeDisabledMapConfig()

	// Some syntactic sugar
	get := func(restPath string, parameterNames []string) {
		rval.addEntry(restPath, http.MethodGet, parameterNames)
	}

	get("/v2/accounts", []string{"currency-greater-than", "currency-less-than"})
	get("/v2/accounts/{account-id}/transactions", []string{"note-prefix", "tx-type", "sig-type", "asset-id", "before-time", "after-time", "rekey-to"})
	get("/v2/assets", []string{"name", "unit"})
	get("/v2/assets/{asset-id}/balances", []string{"currency-greater-than", "currency-less-than"})
	get("/v2/transactions", []string{"note-prefix", "tx-type", "sig-type", "asset-id", "before-time", "after-time", "currency-greater-than", "currency-less-than", "address-role", "exclude-close-to", "rekey-to", "application-id"})
	get("/v2/assets/{asset-id}/transactions", []string{"note-prefix", "tx-type", "sig-type", "asset-id", "before-time", "after-time", "currency-greater-than", "currency-less-than", "address-role", "exclude-close-to", "rekey-to"})

	return rval
}

// ErrDisabledMapConfig contains any mis-spellings that could be present in a configuration
type ErrDisabledMapConfig struct {
	// Key -> REST Path that was mis-spelled
	// Value -> Operation "get, post, etc" -> Sub-value: Any parameters that were found to be mis-spelled in a valid REST PATH
	BadEntries map[string]map[string][]string
}

func (edmc *ErrDisabledMapConfig) Error() string {
	var sb strings.Builder
	for k, v := range edmc.BadEntries {

		// If the length of the list is zero then it is an unknown REST path
		if len(v) == 0 {
			_, _ = sb.WriteString(fmt.Sprintf("Unknown REST Path: %s\n", k))
			continue
		}

		for op, param := range v {
			_, _ = sb.WriteString(fmt.Sprintf("REST Path %s (Operation: %s) contains unknown parameters: %s\n", k, op, strings.Join(param, ",")))
		}
	}
	return sb.String()
}

// makeErrDisabledMapConfig returns a new disabled map config error
func makeErrDisabledMapConfig() *ErrDisabledMapConfig {
	return &ErrDisabledMapConfig{
		BadEntries: make(map[string]map[string][]string),
	}
}

// validate makes sure that all keys and values in the Disabled Map Configuration
// are actually spelled right.  What might happen is that a user might
// accidentally mis-spell an entry, so we want to make sure to mitigate against
// that by checking the openapi definition
func (dmc *DisabledMapConfig) validate(swag *openapi3.T) error {
	potentialRval := makeErrDisabledMapConfig()

	for recordedPath, recordedOp := range dmc.Data {
		swagPath, exists := swag.Paths[recordedPath]
		if !exists {
			// This means that the rest endpoint itself is mis-spelled
			potentialRval.BadEntries[recordedPath] = map[string][]string{}
			continue
		}

		for opName, recordedParams := range recordedOp {
			// This will panic if it is an illegal name so no need to check for nil
			swagOperation := swagPath.GetOperation(opName)

			for _, recordedParam := range recordedParams {
				found := false

				for _, swagParam := range swagOperation.Parameters {
					if recordedParam == swagParam.Value.Name {
						found = true
						break
					}
				}

				if found {
					continue
				}

				// If we didn't find it then it's time to add it to the entry
				if potentialRval.BadEntries[recordedPath] == nil {
					potentialRval.BadEntries[recordedPath] = make(map[string][]string)
				}

				potentialRval.BadEntries[recordedPath][opName] = append(potentialRval.BadEntries[recordedPath][opName], recordedParam)
			}
		}
	}

	// If we have no entries then don't return an error
	if len(potentialRval.BadEntries) != 0 {
		return potentialRval
	}

	return nil
}

// MakeDisabledMapFromOA3 Creates a new disabled map from an openapi3 definition
func MakeDisabledMapFromOA3(swag *openapi3.T, config *DisabledMapConfig) (*DisabledMap, error) {
	if config == nil {
		return nil, nil
	}

	err := config.validate(swag)

	if err != nil {
		return nil, err
	}

	rval := MakeDisabledMap()
	for restPath, item := range swag.Paths {
		for opName, opItem := range item.Operations() {

			endpointConfig := makeEndpointConfig()

			for _, pref := range opItem.Parameters {

				paramName := pref.Value.Name

				parameterIsDisabled := config.isDisabled(restPath, opName, paramName)
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
					endpointConfig.DisabledOptionalParameters[paramName] = true
				}
			}

			rval.Data[opItem.OperationID] = endpointConfig

		}

	}

	return rval, err
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
