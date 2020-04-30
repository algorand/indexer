// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of go-algorand
//
// go-algorand is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// go-algorand is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with go-algorand.  If not, see <https://www.gnu.org/licenses/>.
package middlewares

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const urlAuthFormatter = "/urlAuth/%s"

// Auth takes a logger and an array of api token and return a middleware function
// that ensures one of the api tokens was provided.
func Auth(tokenHeader string, apiTokens []string) func(echo.HandlerFunc) echo.HandlerFunc {
	apiTokenBytes := make([][]byte, 0)
	for _, token := range apiTokens {
		apiTokenBytes = append(apiTokenBytes, []byte(token))
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			// OPTIONS responses never require auth
			if ctx.Request().Method == "OPTIONS" {
				return next(ctx)
			}

			// Grab the apiToken from the HTTP header, or as a bearer token
			providedToken := []byte(ctx.Request().Header.Get(tokenHeader))
			if len(providedToken) == 0 {
				// Accept tokens provided in a bearer token format.
				authentication := strings.SplitN(ctx.Request().Header.Get("Authorization"), " ", 2)
				if len(authentication) == 2 && strings.EqualFold("Bearer", authentication[0]) {
					providedToken = []byte(authentication[1])
				}
			}

			// Handle debug routes with /urlAuth/:token prefix.
			if ctx.Param("token") != "" {
				// For debug routes, we place the apiToken in the path itself
				providedToken = []byte(ctx.Param("token"))

				// Internally, pprof matches exact routes and won't match our APIToken.
				// We need to rewrite the requested path to exclude the token prefix.
				// https://git.io/fp2NO
				authPrefix := fmt.Sprintf(urlAuthFormatter, providedToken)
				// /urlAuth/[token string]/debug/pprof/ => /debug/pprof/
				newPath := strings.TrimPrefix(ctx.Request().URL.Path, authPrefix)
				ctx.SetPath(newPath)
				ctx.Request().URL.Path = newPath
			}

			// Check the tokens in constant time
			for _, tokenBytes := range apiTokenBytes {
				if subtle.ConstantTimeCompare(providedToken, tokenBytes) == 1 {
					// Token was correct, keep serving request
					return next(ctx)
				}
			}

			return echo.NewHTTPError(http.StatusUnauthorized, "Invalid API Token")
		}
	}
}
