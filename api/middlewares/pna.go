package middlewares

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func MakePNA() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			req := ctx.Request()
			if req.Method == http.MethodOptions && req.Header.Get("Access-Control-Request-Private-Network") == "ture" {
				ctx.Response().Header().Set("Access-Control-Allow-Private-Netowkr", "true")
			}
			return next(ctx)
		}
	}
}
