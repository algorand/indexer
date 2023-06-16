package middlewares

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/idb/mocks"
)

var errSuccess = errors.New("unexpected success")
var e = echo.New()

// success is the "next" handler, it is only called when a request is allowed to continue
func success(ctx echo.Context) error {
	return errSuccess
}

func TestMigrationMiddlewareWaiting(t *testing.T) {
	mockIndexer := &mocks.IndexerDb{}

	hMigrating := idb.Health{
		IsMigrating: true,
	}

	mockIndexer.On("Health", mock.Anything, mock.Anything).Return(hMigrating, nil)

	handler := MakeMigrationMiddleware(mockIndexer)(success)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := e.NewContext(req, nil)
	err := handler(c)

	require.Error(t, err, DBUnavailableError, `'IsMigrating' is true, so we should see an DBUnavailableError`)
}

func TestMigrationMiddlewareDone(t *testing.T) {
	mockIndexer := &mocks.IndexerDb{}

	hReady := idb.Health{
		IsMigrating: false,
	}

	mockIndexer.On("Health", mock.Anything, mock.Anything).Return(hReady, nil)

	handler := MakeMigrationMiddleware(mockIndexer)(success)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := e.NewContext(req, nil)
	err := handler(c)

	require.Error(t, err, errSuccess.Error(), `'IsMigrating' is false, so errSuccess should pass through`)
}
