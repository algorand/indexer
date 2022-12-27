package api

import (
	"context"
	"net"
	"net/http"
	"strings"
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

	// DisabledMapConfig is the disabled map configuration that is being used by the server
	DisabledMapConfig *DisabledMapConfig

	// MaxAPIResourcesPerAccount is the maximum number of combined AppParams, AppLocalState, AssetParams,
	// and AssetHolding resources per address that can be returned by the /v2/accounts endpoints.
	// If an address exceeds this number, a 400 error is returned. Zero means unlimited.
	MaxAPIResourcesPerAccount uint64

	/////////////////////
	// Limit Constants //
	/////////////////////

	// Transactions
	MaxTransactionsLimit     uint64
	DefaultTransactionsLimit uint64

	// Accounts
	MaxAccountsLimit     uint64
	DefaultAccountsLimit uint64

	// Assets
	MaxAssetsLimit     uint64
	DefaultAssetsLimit uint64

	// Asset Balances
	MaxBalancesLimit     uint64
	DefaultBalancesLimit uint64

	// Applications
	MaxApplicationsLimit     uint64
	DefaultApplicationsLimit uint64

	// Boxes
	MaxBoxesLimit     uint64
	DefaultBoxesLimit uint64
}

func (e ExtraOptions) handlerTimeout() time.Duration {
	// Basically, if write timeout is 2 seconds or greater, subtract a second.
	// If less, subtract 10% as a safety valve.
	if e.WriteTimeout >= 2*time.Second {
		return e.WriteTimeout - time.Second
	}

	return e.WriteTimeout - time.Duration(0.1*float64(e.WriteTimeout))
}

// Serve starts an http server for the indexer API. This call blocks.
func Serve(ctx context.Context, serveAddr string, db idb.IndexerDb, dataError func() error, log *log.Logger, options ExtraOptions) {
	e := echo.New()
	e.HideBanner = true

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
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		// we currently support compressed result only for GET /v2/blocks/ API
		Skipper: func(c echo.Context) bool {
			return !strings.Contains(c.Path(), "/v2/blocks/")
		},
		Level: -1,
	}))

	middleware := make([]echo.MiddlewareFunc, 0)

	middleware = append(middleware, middlewares.MakeMigrationMiddleware(db))

	if len(options.Tokens) > 0 {
		middleware = append(middleware, middlewares.MakeAuth("X-Indexer-API-Token", options.Tokens))
	}

	swag, err := generated.GetSwagger()

	if err != nil {
		log.Fatal(err)
	}

	disabledMap, err := MakeDisabledMapFromOA3(swag, options.DisabledMapConfig)
	if err != nil {
		log.Fatal(err)
	}

	api := ServerImplementation{
		EnableAddressSearchRoundRewind: options.DeveloperMode,
		db:                             db,
		dataError:                      dataError,
		timeout:                        options.handlerTimeout(),
		log:                            log,
		disabledParams:                 disabledMap,
		opts:                           options,
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

	go func() {
		if err := e.StartServer(s); err != nil {
			log.Fatalf("Serve() err: %s", err)
		}
	}()

	<-ctx.Done()
	// Allow one second for graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}
