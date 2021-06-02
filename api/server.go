package api

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/api/generated/common"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/api/middlewares"
	"github.com/algorand/indexer/idb"
)

// ExtraOptions are options which change the behavior or the HTTP server.
type ExtraOptions struct {
	// Tokens are the access tokens which can access the API.
	Tokens []string

	// DeveloperMode turns on features like AddressSearchRoundRewind
	DeveloperMode bool

	// MetricsEndpoint turns on the /metrics endpoint for prometheus metrics.
	MetricsEndpoint bool
}

// Serve starts an http server for the indexer API. This call blocks.
func Serve(ctx context.Context, serveAddr string, db idb.IndexerDb, fetcherError error, log *log.Logger, options ExtraOptions) {
	e := echo.New()
	e.HideBanner = true

	if options.MetricsEndpoint {
		p := prometheus.NewPrometheus("indexer", nil, nil)
		p.RequestCounterURLLabelMappingFunc = middlewares.PrometheusPathMapper
		p.Use(e)
	}

	e.Use(middlewares.MakeLogger(log))
	e.Use(middleware.CORS())

	middleware := make([]echo.MiddlewareFunc, 0)

	middleware = append(middleware, middlewares.MakeMigrationMiddleware(db))

	if len(options.Tokens) > 0 {
		middleware = append(middleware, middlewares.MakeAuth("X-Indexer-API-Token", options.Tokens))
	}

	api := ServerImplementation{
		EnableAddressSearchRoundRewind: options.DeveloperMode,
		db:                             db,
		fetcher:                        fetcherError,
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
