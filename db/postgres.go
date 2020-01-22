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

package db

// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

// import text to contstant setup_postgres_sql
//go:generate go run ../cmd/texttosource/main.go db setup_postgres.sql

import (
	"database/sql"

	_ "github.com/lib/pq"
)

// TODO: move this out of Postgres imlementation when there are other implementations
// TODO: sqlite3 impl
// TODO: cockroachdb impl
type IndexerDb interface {
}

func OpenPostgres(connection string) (idb IndexerDb, err error) {
	db, err := sql.Open("postgres", connection)
	if err != nil {
		return nil, err
	}
	idb = &postgresIndexerDb{db: db}
	err = idb.init()
	return
}

type postgresIndexerDb struct {
	db *sql.DB
}

func (db *postgresIndexerDb) init() error {
	db.db.Exec(setup_postgres_sql)
}
