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

import (
	"fmt"

	"github.com/algorand/indexer/types"
)

func DummyIndexerDb() IndexerDb {
	return &dummyIndexerDb{}
}

type dummyIndexerDb struct {
}

func (db *dummyIndexerDb) StartBlock() (err error) {
	fmt.Printf("StartBlock\n")
	return nil
}
func (db *dummyIndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txnbytes []byte, txn types.SignedTxnInBlock, participation [][]byte) error {
	fmt.Printf("\ttxn %d %d %d %d\n", round, intra, txtypeenum, assetid)
	return nil
}
func (db *dummyIndexerDb) CommitBlock(round uint64, timestamp int64, headerbytes []byte) error {
	fmt.Printf("CommitBlock %d %d %d header bytes\n", round, timestamp, len(headerbytes))
	return nil
}

func (db *dummyIndexerDb) AlreadyImported(path string) (imported bool, err error) {
	return false, nil
}
func (db *dummyIndexerDb) MarkImported(path string) (err error) {
	return nil
}

type IndexerFactory interface {
	Name() string
	Build(arg string) (IndexerDb, error)
}

// TODO: move this out of Postgres imlementation when there are other implementations
// TODO: sqlite3 impl
// TODO: cockroachdb impl
type IndexerDb interface {
	StartBlock() error
	AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txnbytes []byte, txn types.SignedTxnInBlock, participation [][]byte) error
	CommitBlock(round uint64, timestamp int64, headerbytes []byte) error

	AlreadyImported(path string) (imported bool, err error)
	MarkImported(path string) (err error)
}

type dummyFactory struct {
}

func (df dummyFactory) Name() string {
	return "dummy"
}
func (df dummyFactory) Build(arg string) (IndexerDb, error) {
	return &dummyIndexerDb{}, nil
}

// This layer of indirection allows for different db integrations to be compiled in or compiled out by `go build --tags ...`
var indexerFactories []IndexerFactory

func init() {
	indexerFactories = append(indexerFactories, &dummyFactory{})
}

func IndexerDbByName(factoryname, arg string) (IndexerDb, error) {
	for _, ifac := range indexerFactories {
		if ifac.Name() == factoryname {
			return ifac.Build(arg)
		}
	}
	return nil, fmt.Errorf("no IndexerDb factory for %s", factoryname)
}
