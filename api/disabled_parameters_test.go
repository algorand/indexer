package api

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type testingErrorReporter struct {
	ErrorFCalledCount int
}

func (er *testingErrorReporter) Errorf(string, ...interface{}) {
	er.ErrorFCalledCount = er.ErrorFCalledCount + 1
}

func (er *testingErrorReporter) Reset() {
	er.ErrorFCalledCount = 0
}

type testingEchoContext struct {
	InternalQueryParams  url.Values
	InternalFormParams   url.Values
	ForceFormParamsError bool
}

func (t testingEchoContext) Request() *http.Request {
	panic("implement me")
}

func (t testingEchoContext) SetRequest(r *http.Request) {
	panic("implement me")
}

func (t testingEchoContext) SetResponse(r *echo.Response) {
	panic("implement me")
}

func (t testingEchoContext) Response() *echo.Response {
	panic("implement me")
}

func (t testingEchoContext) IsTLS() bool {
	panic("implement me")
}

func (t testingEchoContext) IsWebSocket() bool {
	panic("implement me")
}

func (t testingEchoContext) Scheme() string {
	panic("implement me")
}

func (t testingEchoContext) RealIP() string {
	panic("implement me")
}

func (t testingEchoContext) Path() string {
	panic("implement me")
}

func (t testingEchoContext) SetPath(p string) {
	panic("implement me")
}

func (t testingEchoContext) Param(name string) string {
	panic("implement me")
}

func (t testingEchoContext) ParamNames() []string {
	panic("implement me")
}

func (t testingEchoContext) SetParamNames(names ...string) {
	panic("implement me")
}

func (t testingEchoContext) ParamValues() []string {
	panic("implement me")
}

func (t testingEchoContext) SetParamValues(values ...string) {
	panic("implement me")
}

func (t testingEchoContext) QueryParam(name string) string {
	panic("implement me")
}

func (t testingEchoContext) QueryParams() url.Values {
	return t.InternalQueryParams
}

func (t testingEchoContext) QueryString() string {
	panic("implement me")
}

func (t testingEchoContext) FormValue(name string) string {
	panic("implement me")
}

func (t testingEchoContext) FormParams() (url.Values, error) {
	if t.ForceFormParamsError {
		return url.Values{}, errors.New("FormParamsError")
	}
	return t.InternalFormParams, nil
}

func (t testingEchoContext) FormFile(name string) (*multipart.FileHeader, error) {
	panic("implement me")
}

func (t testingEchoContext) MultipartForm() (*multipart.Form, error) {
	panic("implement me")
}

func (t testingEchoContext) Cookie(name string) (*http.Cookie, error) {
	panic("implement me")
}

func (t testingEchoContext) SetCookie(cookie *http.Cookie) {
	panic("implement me")
}

func (t testingEchoContext) Cookies() []*http.Cookie {
	panic("implement me")
}

func (t testingEchoContext) Get(key string) interface{} {
	panic("implement me")
}

func (t testingEchoContext) Set(key string, val interface{}) {
	panic("implement me")
}

func (t testingEchoContext) Bind(i interface{}) error {
	panic("implement me")
}

func (t testingEchoContext) Validate(i interface{}) error {
	panic("implement me")
}

func (t testingEchoContext) Render(code int, name string, data interface{}) error {
	panic("implement me")
}

func (t testingEchoContext) HTML(code int, html string) error {
	panic("implement me")
}

func (t testingEchoContext) HTMLBlob(code int, b []byte) error {
	panic("implement me")
}

func (t testingEchoContext) String(code int, s string) error {
	panic("implement me")
}

func (t testingEchoContext) JSON(code int, i interface{}) error {
	panic("implement me")
}

func (t testingEchoContext) JSONPretty(code int, i interface{}, indent string) error {
	panic("implement me")
}

func (t testingEchoContext) JSONBlob(code int, b []byte) error {
	panic("implement me")
}

func (t testingEchoContext) JSONP(code int, callback string, i interface{}) error {
	panic("implement me")
}

func (t testingEchoContext) JSONPBlob(code int, callback string, b []byte) error {
	panic("implement me")
}

func (t testingEchoContext) XML(code int, i interface{}) error {
	panic("implement me")
}

func (t testingEchoContext) XMLPretty(code int, i interface{}, indent string) error {
	panic("implement me")
}

func (t testingEchoContext) XMLBlob(code int, b []byte) error {
	panic("implement me")
}

func (t testingEchoContext) Blob(code int, contentType string, b []byte) error {
	panic("implement me")
}

func (t testingEchoContext) Stream(code int, contentType string, r io.Reader) error {
	panic("implement me")
}

func (t testingEchoContext) File(file string) error {
	panic("implement me")
}

func (t testingEchoContext) Attachment(file string, name string) error {
	panic("implement me")
}

func (t testingEchoContext) Inline(file string, name string) error {
	panic("implement me")
}

func (t testingEchoContext) NoContent(code int) error {
	panic("implement me")
}

func (t testingEchoContext) Redirect(code int, url string) error {
	panic("implement me")
}

func (t testingEchoContext) Error(err error) {
	panic("implement me")
}

func (t testingEchoContext) Handler() echo.HandlerFunc {
	panic("implement me")
}

func (t testingEchoContext) SetHandler(h echo.HandlerFunc) {
	panic("implement me")
}

func (t testingEchoContext) Logger() echo.Logger {
	panic("implement me")
}

func (t testingEchoContext) SetLogger(l echo.Logger) {
	panic("implement me")
}

func (t testingEchoContext) Echo() *echo.Echo {
	panic("implement me")
}

func (t testingEchoContext) Reset(r *http.Request, w http.ResponseWriter) {
	panic("implement me")
}

// TestFailingFormParam tests that disabled parameters provided via
// the FormParams() function of the context are appropriately handled
func TestFailingFormParam(t *testing.T) {
	dm := NewDisabledMap()

	dm.Data["K1"] = DisabledList{
		RequiredParams: []DisabledParameter{
			{"1", false},
			{"2", false},
		},
		OptionalParams: []DisabledParameter{
			{"1", true},
			{"3", false},
		},
	}

	ctx := testingEchoContext{
		InternalQueryParams:  nil,
		InternalFormParams:   make(map[string][]string),
		ForceFormParamsError: false,
	}

	ctx.InternalFormParams["3"] = []string{"Provided"}

	er := &testingErrorReporter{0}

	rval, rstr := Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyIsGood, rval)
	require.Equal(t, 0, er.ErrorFCalledCount)
	require.Empty(t, rstr)

	er.Reset()

	// Disabled parameter is "provided" but empty...should be ok
	ctx.InternalFormParams["1"] = []string{}

	rval, rstr = Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyIsGood, rval)
	require.Equal(t, 0, er.ErrorFCalledCount)
	require.Empty(t, rstr)

	er.Reset()

	// Disabled parameter is now actually provided
	ctx.InternalFormParams["1"] = []string{"Provided"}

	rval, rstr = Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyFailedParameter, rval)
	require.Equal(t, 0, er.ErrorFCalledCount)
	require.Equal(t, "1", rstr)

	// The forms parameter will now error out and we won't get
	// an error because we wont be able to check it
	ctx.ForceFormParamsError = true
	er.Reset()

	rval, rstr = Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyIsGood, rval)
	require.Equal(t, 1, er.ErrorFCalledCount)
	require.Empty(t, rstr)

}

// TestFailingQueryParam tests that disabled parameters provided via
// the QueryParams() function of the context are appropriately handled
func TestFailingQueryParam(t *testing.T) {
	dm := NewDisabledMap()

	dm.Data["K1"] = DisabledList{
		RequiredParams: []DisabledParameter{
			{"1", false},
			{"2", false},
		},
		OptionalParams: []DisabledParameter{
			{"1", true},
			{"3", false},
		},
	}

	ctx := testingEchoContext{
		InternalQueryParams:  make(map[string][]string),
		InternalFormParams:   nil,
		ForceFormParamsError: false,
	}

	ctx.InternalQueryParams["3"] = []string{"Provided"}

	er := &testingErrorReporter{0}

	rval, rstr := Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyIsGood, rval)
	require.Equal(t, 0, er.ErrorFCalledCount)
	require.Empty(t, rstr)

	er.Reset()

	// Disabled parameter is "provided" but empty...should be ok
	ctx.InternalQueryParams["1"] = []string{}

	rval, rstr = Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyIsGood, rval)
	require.Equal(t, 0, er.ErrorFCalledCount)
	require.Empty(t, rstr)

	er.Reset()

	// Disabled parameter is now actually provided
	ctx.InternalQueryParams["1"] = []string{"Provided"}

	rval, rstr = Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyFailedParameter, rval)
	require.Equal(t, 0, er.ErrorFCalledCount)
	require.Equal(t, "1", rstr)

}

// TestFailingEndpoint tests that an endpoint which has a disabled required parameter
// returns a failed endpoint error
func TestFailingEndpoint(t *testing.T) {
	dm := NewDisabledMap()

	dm.Data["K1"] = DisabledList{
		RequiredParams: []DisabledParameter{
			{"1", false},
			{"2", true},
		},
		OptionalParams: []DisabledParameter{
			{"1", true},
			{"3", false},
		},
	}

	ctx := testingEchoContext{
		InternalQueryParams:  nil,
		InternalFormParams:   nil,
		ForceFormParamsError: false,
	}

	er := &testingErrorReporter{0}

	rval, rstr := Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyFailedEndpoint, rval)
	require.Equal(t, 0, er.ErrorFCalledCount)
	require.Empty(t, rstr)
}

// TestVerifyNonExistentHandler tests that nonexistent endpoint is logged
// but doesn't stop the indexer from functioning
func TestVerifyNonExistentHandler(t *testing.T) {
	dm := NewDisabledMap()

	dm.Data["K1"] = DisabledList{
		RequiredParams: []DisabledParameter{
			{"1", false},
			{"2", false},
		},
		OptionalParams: []DisabledParameter{
			{"1", true},
			{"3", false},
		},
	}

	ctx := testingEchoContext{
		InternalQueryParams:  nil,
		InternalFormParams:   nil,
		ForceFormParamsError: false,
	}

	er := &testingErrorReporter{0}

	rval, rstr := Verify(dm, "DoesntExist", ctx, er)

	require.Equal(t, verifyIsGood, rval)
	require.Equal(t, 1, er.ErrorFCalledCount)
	require.Empty(t, rstr)

	er.Reset()

	rval, rstr = Verify(dm, "K1", ctx, er)

	require.Equal(t, verifyIsGood, rval)
	require.Equal(t, 0, er.ErrorFCalledCount)
	require.Empty(t, rstr)
}
