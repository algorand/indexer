package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/v3/api/generated/v2"
)

func TestToDisabledMapConfigFromFile(t *testing.T) {
	expectedValue := DisabledMapConfig{Data: map[string]map[string][]string{
		"/sampleEndpoint": {http.MethodGet: {"p2"}},
	}}

	configFile := filepath.Join("test_resources", "mock_disabled_map_config.yaml")

	// Nil pointer for openapi3.swagger because we don't want any validation
	// to be run on the config (they are made up endpoints)
	loadedConfig, err := MakeDisabledMapConfigFromFile(nil, configFile)
	require.NoError(t, err)
	require.NotNil(t, loadedConfig)
	require.Equal(t, expectedValue, *loadedConfig)
}

func TestToDisabledMapConfig(t *testing.T) {
	type testingStruct struct {
		name        string
		ddm         *DisplayDisabledMap
		dmc         *DisabledMapConfig
		expectError string
	}

	tests := []testingStruct{
		{"test 1",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required": {{"p1": "enabled"}, {"p2": "disabled"}},
					"optional": {{"p3": "enabled"}},
				}}},
			&DisabledMapConfig{Data: map[string]map[string][]string{
				"/sampleEndpoint": {http.MethodGet: {"p2"}},
			}},

			"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			// Nil pointer for openapi3.swagger because we don't want any validation
			// to be run on the config
			dmc, err := test.ddm.toDisabledMapConfig(nil)

			if test.expectError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.expectError)
			} else {
				require.NoError(t, err)
				require.True(t, reflect.DeepEqual(*dmc, *test.dmc))
			}
		})
	}

}

func TestSchemaCheck(t *testing.T) {
	type testingStruct struct {
		name        string
		ddm         *DisplayDisabledMap
		expectError []string
	}
	tests := []testingStruct{
		{"test param types - good",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required": {{"p1": "enabled"}, {"p2": "disabled"}},
					"optional": {{"p3": "enabled"}},
				}},
			},
			nil,
		},

		{"test param types - bad required",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required-FAKE": {{"p1": "enabled"}, {"p2": "disabled"}},
					"optional":      {{"p3": "enabled"}},
				}},
			},
			[]string{"required-FAKE"},
		},

		{"test param types - bad optional",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required":      {{"p1": "enabled"}, {"p2": "disabled"}},
					"optional-FAKE": {{"p3": "enabled"}},
				}},
			},
			[]string{"optional-FAKE"},
		},

		{"test param types - bad both",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required-FAKE": {{"p1": "enabled"}, {"p2": "disabled"}},
					"optional-FAKE": {{"p3": "enabled"}},
				}},
			},
			[]string{"required-FAKE", "optional-FAKE"},
		},

		{"test param status - good",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required": {{"p1": "enabled"}, {"p2": "disabled"}},
					"optional": {{"p3": "enabled"}},
				}},
			},
			nil,
		},

		{"test param status - bad required",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required": {{"p1": "enabled"}, {"p2": "disabled-FAKE"}},
					"optional": {{"p3": "enabled"}},
				}},
			},
			[]string{"p2"},
		},

		{"test param status - bad optional",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required": {{"p1": "enabled"}, {"p2": "disabled"}},
					"optional": {{"p3": "enabled-FAKE"}},
				}},
			},
			[]string{"p3"},
		},

		{"test param status - bad both",
			&DisplayDisabledMap{Data: map[string]map[string][]map[string]string{
				"/sampleEndpoint": {
					"required": {{"p1": "enabled-FAKE"}, {"p2": "disabled"}},
					"optional": {{"p3": "enabled-FAKE"}},
				}},
			},
			[]string{"p1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.ddm.validateSchema()

			if len(test.expectError) != 0 {
				require.Error(t, err)
				for _, str := range test.expectError {
					require.Contains(t, err.Error(), str)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}

}

func TestValidate(t *testing.T) {
	// Validates that the default config is correctly spelled
	dmc := GetDefaultDisabledMapConfigForPostgres()

	swag, err := generated.GetSwagger()
	require.NoError(t, err)

	require.NoError(t, dmc.validate(swag))
}

// TestFailingParam tests that disabled parameters provided via
// the FormParams() and QueryParams() functions of the context are appropriately handled
func TestFailingParam(t *testing.T) {
	type testingStruct struct {
		name               string
		setFormValues      func(*url.Values)
		expectedError      error
		expectedErrorCount int
		mimeType           string
	}
	tests := []testingStruct{
		{
			"non-disabled param provided",
			func(f *url.Values) {
				f.Set("3", "Provided")
			}, nil, 0, echo.MIMEApplicationForm,
		},
		{
			"disabled param provided but empty",
			func(f *url.Values) {
				f.Set("1", "")
			}, nil, 0, echo.MIMEApplicationForm,
		},
		{
			"disabled param provided",
			func(f *url.Values) {
				f.Set("1", "Provided")
			}, ErrVerifyFailedParameter{"1"}, 0, echo.MIMEApplicationForm,
		},
	}

	testsPostOnly := []testingStruct{
		{
			"Error encountered for Form Params",
			func(f *url.Values) {
				f.Set("1", "Provided")
			}, nil, 1, echo.MIMEMultipartForm,
		},
	}

	ctxFactoryGet := func(e *echo.Echo, f *url.Values, t *testingStruct) *echo.Context {
		req := httptest.NewRequest(http.MethodGet, "/?"+f.Encode(), nil)
		ctx := e.NewContext(req, nil)
		return &ctx
	}

	ctxFactoryPost := func(e *echo.Echo, f *url.Values, t *testingStruct) *echo.Context {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(f.Encode()))
		req.Header.Add(echo.HeaderContentType, t.mimeType)
		ctx := e.NewContext(req, nil)
		return &ctx
	}

	runner := func(t *testing.T, tstruct *testingStruct, ctxFactory func(*echo.Echo, *url.Values, *testingStruct) *echo.Context) {
		dm := MakeDisabledMap()
		e1 := makeEndpointConfig()
		e1.EndpointDisabled = false
		e1.DisabledOptionalParameters["1"] = true

		dm.Data["K1"] = e1

		e := echo.New()

		f := make(url.Values)
		tstruct.setFormValues(&f)

		ctx := ctxFactory(e, &f, tstruct)

		logger, hook := test.NewNullLogger()

		err := Verify(dm, "K1", *ctx, logger)

		require.Equal(t, tstruct.expectedError, err)
		require.Len(t, hook.AllEntries(), tstruct.expectedErrorCount)
	}

	for _, test := range tests {
		t.Run("Post-"+test.name, func(t *testing.T) {
			runner(t, &test, ctxFactoryPost)
		})

		t.Run("Get-"+test.name, func(t *testing.T) {
			runner(t, &test, ctxFactoryGet)
		})

	}

	for _, test := range testsPostOnly {
		t.Run("Post-"+test.name, func(t *testing.T) {
			runner(t, &test, ctxFactoryPost)
		})

	}
}

// TestFailingEndpoint tests that an endpoint which has a disabled required parameter
// returns a failed endpoint error
func TestFailingEndpoint(t *testing.T) {
	dm := MakeDisabledMap()

	e1 := makeEndpointConfig()
	e1.EndpointDisabled = true
	e1.DisabledOptionalParameters["1"] = true

	dm.Data["K1"] = e1

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/?", nil)
	ctx := e.NewContext(req, nil)

	logger, hook := test.NewNullLogger()

	err := Verify(dm, "K1", ctx, logger)

	require.Equal(t, ErrVerifyFailedEndpoint, err)

	require.Len(t, hook.AllEntries(), 0)
}

// TestVerifyNonExistentHandler tests that nonexistent endpoint is logged
// but doesn't stop the indexer from functioning
func TestVerifyNonExistentHandler(t *testing.T) {
	dm := MakeDisabledMap()

	e1 := makeEndpointConfig()
	e1.EndpointDisabled = false
	e1.DisabledOptionalParameters["1"] = true

	dm.Data["K1"] = e1

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/?", nil)
	ctx := e.NewContext(req, nil)

	logger, hook := test.NewNullLogger()

	err := Verify(dm, "DoesntExist", ctx, logger)

	require.Equal(t, nil, err)
	require.Len(t, hook.AllEntries(), 1)

	hook.Reset()

	err = Verify(dm, "K1", ctx, logger)

	require.Equal(t, nil, err)

	require.Len(t, hook.AllEntries(), 0)
}
