package middlewares

import (
	"github.com/algorand/indexer/idb"
	"github.com/labstack/echo/v4"
	"net/http"
)

var InProgressError = "Indexer migration in progress, please wait."
type MigrationMiddleware struct {
	idb       idb.IndexerDb
}

// MakeAuth constructs the auth middleware function
func MakeMigrationMiddleware(idb idb.IndexerDb) echo.MiddlewareFunc {
	mw := MigrationMiddleware{
		idb:       idb,
	}

	return mw.handler
}

// Auth takes a logger and an array of api token and return a middleware function
// that ensures one of the api tokens was provided.
func (mm *MigrationMiddleware) handler(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		h, err := mm.idb.Health()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Indexer health error: %s", err.Error())
		}

		if !h.IsMigrating {
			return next(ctx)
		}

		return echo.NewHTTPError(http.StatusInternalServerError, InProgressError)
	}
}
