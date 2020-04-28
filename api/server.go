// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of the Algorand Indexer
//
// Algorand Indexer is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// Algorand Indexer is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with Algorand Indexer.  If not, see <https://www.gnu.org/licenses/>.

package api

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/api/generated"
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

	e.Use(middlewares.Logger(log))
	e.Use(middleware.CORS())

	auth := make([]echo.MiddlewareFunc, 0)
	if (len(tokens) > 0) {
		auth = append(auth, middlewares.Auth(log, "X-Indexer-API-Token", tokens))
	}

	api := ServerImplementation{
		EnableAddressSearchRoundRewind: developerMode,
		db:                             db,
	}
	generated.RegisterHandlers(e, &api, auth...)

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
