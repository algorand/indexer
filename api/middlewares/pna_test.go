package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestMakePNA(t *testing.T) {
	// Create a new Echo instance
	e := echo.New()

	// Create a handler to be wrapped by the middleware
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	}

	// Create the middleware
	middleware := MakePNA()

	// Test case 1: OPTIONS request with Access-Control-Request-Private-Network header
	t.Run("OPTIONS request with PNA header", func(t *testing.T) {
		// Create a new HTTP request and response recorder
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		rec := httptest.NewRecorder()

		// Set the expected PNA header
		req.Header.Set("Access-Control-Request-Private-Network", "true")

		// Create Echo context
		c := e.NewContext(req, rec)

		// Call our MakePNA middleware
		err := middleware(handler)(c)

		// Assert there's no error and check the PNA header was set correctly
		assert.NoError(t, err)
		assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Private-Network"))
	})

	// Test case 2: Non-OPTIONS request
	t.Run("Non-OPTIONS request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := middleware(handler)(c)

		// Assert there's no error and check the PNA header wasn't set
		assert.NoError(t, err)
		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Private-Network"))
	})

	// Test case 3: OPTIONS request without Access-Control-Request-Private-Network header
	t.Run("OPTIONS request without Private Network header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := middleware(handler)(c)

		// Assert there's no error and check the PNA header wasn't set
		assert.NoError(t, err)
		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Private-Network"))
	})
}
