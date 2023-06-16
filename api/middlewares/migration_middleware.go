package middlewares

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/algorand/indexer/v3/idb"
)

// DBUnavailableError is the error returned when a migration is in progress or required.
var DBUnavailableError = "Indexer DB is not available, try again later."

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
		h, err := mm.idb.Health(ctx.Request().Context())
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Indexer health error: %s", err))
		}

		if !h.DBAvailable {
			return echo.NewHTTPError(http.StatusInternalServerError, DBUnavailableError)
		}

		return next(ctx)
	}
}
