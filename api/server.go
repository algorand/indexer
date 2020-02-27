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
	"github.com/algorand/indexer/api/generated"
	"github.com/algorand/indexer/idb"
	"github.com/labstack/echo/v4"
	"log"
	"net"
	"net/http"
	"time"
)

// IndexerDb should be set from main()
var IndexerDb idb.IndexerDb

func Serve(ctx context.Context, serveAddr string) {
	e := echo.New()
	api := ServerImplementation{}
	generated.RegisterHandlers(e, &api)

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
