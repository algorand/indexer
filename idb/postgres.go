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

// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package idb

// import text to contstant setup_postgres_sql
//go:generate go run ../cmd/texttosource/main.go idb setup_postgres.sql

import (
	"context"
	"database/sql"
	"time"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	_ "github.com/lib/pq"

	"github.com/algorand/indexer/types"
)

func OpenPostgres(connection string) (idb IndexerDb, err error) {
	db, err := sql.Open("postgres", connection)
	if err != nil {
		return nil, err
	}
	pdb := &postgresIndexerDb{db: db}
	err = pdb.init()
	idb = pdb
	return
}

type postgresIndexerDb struct {
	db *sql.DB
	tx *sql.Tx
}

func (db *postgresIndexerDb) init() (err error) {
	_, err = db.db.Exec(setup_postgres_sql)
	return
}

func (db *postgresIndexerDb) AlreadyImported(path string) (imported bool, err error) {
	row := db.db.QueryRow(`SELECT COUNT(path) FROM imported WHERE path = $1`, path)
	numpath := 0
	err = row.Scan(&numpath)
	return numpath == 1, err
}

func (db *postgresIndexerDb) MarkImported(path string) (err error) {
	_, err = db.db.Exec(`INSERT INTO imported (path) VALUES ($1)`, path)
	return err
}

func (db *postgresIndexerDb) StartBlock() (err error) {
	db.tx, err = db.db.BeginTx(context.Background(), nil)
	return
}

func (db *postgresIndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txnbytes []byte, txn types.SignedTxnInBlock, participation [][]byte) error {
	// TODO: set txn_participation
	var err error
	_, err = db.tx.Exec(`INSERT INTO txn (round, intra, typeenum, asset, txnbytes, txn) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, round, intra, txtypeenum, assetid, txnbytes, string(json.Encode(txn)))
	if err != nil {
		return err
	}
	stmt, err := db.tx.Prepare(`INSERT INTO txn_participation (addr, round, intra) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}
	for _, paddr := range participation {
		_, err = stmt.Exec(paddr, round, intra)
		if err != nil {
			return err
		}
	}
	return err
}
func (db *postgresIndexerDb) CommitBlock(round uint64, timestamp int64, headerbytes []byte) error {
	var err error
	_, err = db.tx.Exec(`INSERT INTO block_header (round, realtime, header) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, round, time.Unix(timestamp, 0), headerbytes)
	if err != nil {
		return err
	}
	err = db.tx.Commit()
	db.tx = nil
	return err
}

type postgresFactory struct {
}

func (df postgresFactory) Name() string {
	return "postgres"
}
func (df postgresFactory) Build(arg string) (IndexerDb, error) {
	return OpenPostgres(arg)
}

func init() {
	indexerFactories = append(indexerFactories, &postgresFactory{})
}
