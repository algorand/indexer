package middlewares

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// MakePNA constructs the Private Network Access middleware function
func MakePNA() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			req := ctx.Request()
			if req.Method == http.MethodOptions && req.Header.Get("Access-Control-Request-Private-Network") == "true" {
				ctx.Response().Header().Set("Access-Control-Allow-Private-Network", "true")
			}
			return next(ctx)
		}
	}
}
