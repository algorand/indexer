package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
)

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
		dm := NewDisabledMap()
		e1 := NewEndpointConfig()
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
		require.Equal(t, tstruct.expectedErrorCount, len(hook.AllEntries()))
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
	dm := NewDisabledMap()

	e1 := NewEndpointConfig()
	e1.EndpointDisabled = true
	e1.DisabledOptionalParameters["1"] = true

	dm.Data["K1"] = e1

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/?", nil)
	ctx := e.NewContext(req, nil)

	logger, hook := test.NewNullLogger()

	err := Verify(dm, "K1", ctx, logger)

	require.Equal(t, ErrVerifyFailedEndpoint, err)
	require.Equal(t, 0, len(hook.AllEntries()))
}

// TestVerifyNonExistentHandler tests that nonexistent endpoint is logged
// but doesn't stop the indexer from functioning
func TestVerifyNonExistentHandler(t *testing.T) {
	dm := NewDisabledMap()

	e1 := NewEndpointConfig()
	e1.EndpointDisabled = false
	e1.DisabledOptionalParameters["1"] = true

	dm.Data["K1"] = e1

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/?", nil)
	ctx := e.NewContext(req, nil)

	logger, hook := test.NewNullLogger()

	err := Verify(dm, "DoesntExist", ctx, logger)

	require.Equal(t, nil, err)
	require.Equal(t, 1, len(hook.AllEntries()))

	hook.Reset()

	err = Verify(dm, "K1", ctx, logger)

	require.Equal(t, nil, err)
	require.Equal(t, 0, len(hook.AllEntries()))
}
