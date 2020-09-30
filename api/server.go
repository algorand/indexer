package api

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/api/generated/common"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/api/middlewares"
	"github.com/algorand/indexer/idb"
)

// TODO: Get rid of this global
var indexerDb idb.IndexerDb

// Serve starts an http server for the indexer API. This call blocks.
func Serve(ctx context.Context, serveAddr string, db idb.IndexerDb, log *log.Logger, tokens []string, developerMode bool) {
	indexerDb = db

	e := echo.New()
	e.HideBanner = true

	e.Use(middlewares.MakeLogger(log))
	e.Use(middleware.CORS())

	middleware := make([]echo.MiddlewareFunc, 0)

	middleware = append(middleware, middlewares.MakeMigrationMiddleware(db))

	if len(tokens) > 0 {
		middleware = append(middleware, middlewares.MakeAuth("X-Indexer-API-Token", tokens))
	}

	api := ServerImplementation{
		EnableAddressSearchRoundRewind: developerMode,
		db:                             db,
	}

	generated.RegisterHandlers(e, &api, middleware...)
	common.RegisterHandlers(e, &api)

	if ctx == nil {
		ctx = context.Background()
	}
	getctx := func(l net.Listener) context.Context {
		return ctx
	}
	s := &http.Server{
		Addr:           serveAddr,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
		BaseContext:    getctx,
	}

	log.Fatal(e.StartServer(s))
}
