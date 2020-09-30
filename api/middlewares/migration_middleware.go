package middlewares

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/algorand/indexer/idb"
)

// InProgressError is the error returned when a migration is in progress.
var InProgressError = "Indexer migration in progress, please wait."

// MigrationMiddleware makes sure a 500 error is returned when the IndexerDb has a migration in progress.
type MigrationMiddleware struct {
	idb idb.IndexerDb
}

// MakeMigrationMiddleware constructs the migration middleware
func MakeMigrationMiddleware(idb idb.IndexerDb) echo.MiddlewareFunc {
	mw := MigrationMiddleware{
		idb: idb,
	}

	return mw.handler
}

// handler returns a 500 if the IndexerDb is migrating.
func (mm *MigrationMiddleware) handler(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		h, err := mm.idb.Health()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Indexer health error: %s", err.Error())
		}

		if h.IsMigrating {
			return echo.NewHTTPError(http.StatusInternalServerError, InProgressError)
		}

		return next(ctx)
	}
}
