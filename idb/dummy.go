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

package idb

import (
	"context"
	"fmt"
	"time"

	"github.com/algorand/go-algorand-sdk/client/algod/models"

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
func (db *dummyIndexerDb) CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
	fmt.Printf("CommitBlock %d %d %d header bytes\n", round, timestamp, len(headerbytes))
	return nil
}

func (db *dummyIndexerDb) AlreadyImported(path string) (imported bool, err error) {
	return false, nil
}
func (db *dummyIndexerDb) MarkImported(path string) (err error) {
	return nil
}

func (db *dummyIndexerDb) LoadGenesis(genesis types.Genesis) (err error) {
	return nil
}

func (db *dummyIndexerDb) SetProto(version string, proto types.ConsensusParams) (err error) {
	return nil
}

func (db *dummyIndexerDb) GetMetastate(key string) (jsonStrValue string, err error) {
	return "", nil
}

func (db *dummyIndexerDb) SetMetastate(key, jsonStrValue string) (err error) {
	return nil
}

func (db *dummyIndexerDb) YieldTxns(ctx context.Context, prevRound int64) <-chan TxnRow {
	return nil
}

func (db *dummyIndexerDb) CommitRoundAccounting(updates RoundUpdates, round, rewardsBase uint64) (err error) {
	return nil
}

func (db *dummyIndexerDb) GetBlock(round uint64) (block types.Block, err error) {
	err = nil
	return
}
func (db *dummyIndexerDb) Transactions(ctx context.Context, tf TransactionFilter) <-chan TxnRow {
	return nil
}

func (db *dummyIndexerDb) GetAccounts(ctx context.Context, opts AccountQueryOptions) <-chan AccountRow {
	return nil
}

type IndexerFactory interface {
	Name() string
	Build(arg string) (IndexerDb, error)
}

type TxnRow struct {
	Round    uint64
	Intra    int
	TxnBytes []byte
	Error    error
}

// TODO: sqlite3 impl
// TODO: cockroachdb impl
type IndexerDb interface {
	// The next few functions define the import interface, functions for loading data into the database. StartBlock() through Get/SetMetastate().

	StartBlock() error
	AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txnbytes []byte, txn types.SignedTxnInBlock, participation [][]byte) error
	CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error

	AlreadyImported(path string) (imported bool, err error)
	MarkImported(path string) (err error)

	LoadGenesis(genesis types.Genesis) (err error)
	SetProto(version string, proto types.ConsensusParams) (err error)

	GetMetastate(key string) (jsonStrValue string, err error)
	SetMetastate(key, jsonStrValue string) (err error)

	// YieldTxns returns a channel that produces the whole transaction stream after some round forward
	YieldTxns(ctx context.Context, prevRound int64) <-chan TxnRow

	CommitRoundAccounting(updates RoundUpdates, round, rewardsBase uint64) (err error)

	GetBlock(round uint64) (block types.Block, err error)

	Transactions(ctx context.Context, tf TransactionFilter) <-chan TxnRow
	//GetAccounts(ctx context.Context, greaterThan types.Address, limit int) (accounts []models.Account, err error)
	GetAccounts(ctx context.Context, opts AccountQueryOptions) <-chan AccountRow
}

type TransactionFilter struct {
	Address    []byte
	MinRound   uint64
	MaxRound   uint64
	AfterTime  time.Time
	BeforeTime time.Time
	AssetId    uint64
	TypeEnum   int // ["","pay","keyreg","acfg","axfer","afrz"]
	Txid       []byte
	Round      int64  // -1 for no filter
	Offset     int64  // -1 for no filter
	SigType    string // ["", "sig", "msig", "lsig"]
	NotePrefix []byte
	MinAlgos   uint64 // implictly filters on "pay" txns for Algos >= this

	Limit uint64
}

type AccountQueryOptions struct {
	GreaterThanAddress []byte // for paging results
	EqualToAddress     []byte // return exactly this one account

	IncludeAssetHoldings bool
	IncludeAssetParams   bool

	Limit uint64
}

type AccountRow struct {
	Account models.Account
	Error   error
}

func (tf *TransactionFilter) Init() {
	tf.Round = -1
	tf.Offset = -1
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

type AcfgUpdate struct {
	AssetId uint64
	Creator types.Address
	Params  types.AssetParams
}

type AssetUpdate struct {
	Addr          types.Address
	AssetId       uint64
	Delta         int64
	DefaultFrozen bool
}

type FreezeUpdate struct {
	Addr    types.Address
	AssetId uint64
	Frozen  bool
}

type AssetClose struct {
	CloseTo       types.Address
	AssetId       uint64
	Sender        types.Address
	DefaultFrozen bool
}

type RoundUpdates struct {
	AlgoUpdates   map[[32]byte]int64
	AcfgUpdates   []AcfgUpdate
	AssetUpdates  []AssetUpdate
	FreezeUpdates []FreezeUpdate
	AssetCloses   []AssetClose
	AssetDestroys []uint64
}
