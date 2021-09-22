package api

import (
	"context"
	"net"
	"net/http"
	"time"

	echo_contrib "github.com/labstack/echo-contrib/prometheus"
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

	// MetricsEndpointVerbose generates separate histograms based on query parameters on the /metrics endpoint.
	MetricsEndpointVerbose bool

	// Maximum amount of time to wait before timing out writes to a response. Note that handler timeout is computed
	//off of this.
	WriteTimeout time.Duration

	// ReadTimeout is the maximum duration for reading the entire request, including the body.
	ReadTimeout time.Duration
}

func (e ExtraOptions) handlerTimeout() time.Duration {
	// Basically, if write timeout is 2 seconds or greater, subtract a second.
	// If less, subtract 10% as a safety valve.
	if e.WriteTimeout >= 2 * time.Second {
		return e.WriteTimeout - time.Second
	} else {
		return e.WriteTimeout - time.Duration(0.1 * float64(e.WriteTimeout))
	}
}

// Serve starts an http server for the indexer API. This call blocks.
func Serve(ctx context.Context, serveAddr string, db idb.IndexerDb, fetcherError error, log *log.Logger, options ExtraOptions) {
	e := echo.New()
	e.HideBanner = true

	// To ensure everything uses the correct context this must be specified first.
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		ErrorMessage: `{"message":"Request Timeout"}`,
		Timeout:      options.handlerTimeout(),
	}))

	if options.MetricsEndpoint {
		p := echo_contrib.NewPrometheus("indexer", nil, nil)
		if options.MetricsEndpointVerbose {
			p.RequestCounterURLLabelMappingFunc = middlewares.PrometheusPathMapperVerbose
		} else {
			p.RequestCounterURLLabelMappingFunc = middlewares.PrometheusPathMapper404Sink
		}
		// This call installs the prometheus metrics collection middleware and
		// the "/metrics" handler.
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
		ReadTimeout:    options.ReadTimeout,
		WriteTimeout:   options.WriteTimeout,
		MaxHeaderBytes: 1 << 20,
		BaseContext:    getctx,
	}

	log.Fatal(e.StartServer(s))
}
