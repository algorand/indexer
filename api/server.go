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
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/algorand/indexer/idb"
)

// IndexerDb should be set from main()
var IndexerDb idb.IndexerDb

func Serve() {
	r := mux.NewRouter()
	r.HandleFunc("/v1/accounts", ListAccounts)
	r.HandleFunc("/v1/account/{address}/transactions", TransactionsForAddress)
	s := &http.Server{
		Addr:           ":8080",
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(s.ListenAndServe())
}
