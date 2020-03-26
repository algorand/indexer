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
	"math/big"
	"time"

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
func (db *dummyIndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error {
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

func (db *dummyIndexerDb) GetProto(version string) (proto types.ConsensusParams, err error) {
	err = nil
	return
}

func (db *dummyIndexerDb) GetMetastate(key string) (jsonStrValue string, err error) {
	return "", nil
}

func (db *dummyIndexerDb) SetMetastate(key, jsonStrValue string) (err error) {
	return nil
}

func (db *dummyIndexerDb) GetMaxRound() (round uint64, err error) {
	return 0, nil
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

func (db *dummyIndexerDb) Assets(ctx context.Context, filter AssetsQuery) <-chan AssetRow {
	return nil
}

func (db *dummyIndexerDb) AssetBalances(ctx context.Context, abq AssetBalanceQuery) <-chan AssetBalanceRow {
	return nil
}

type IndexerFactory interface {
	Name() string
	Build(arg string) (IndexerDb, error)
}

type TxnRow struct {
	Round     uint64
	RoundTime time.Time
	Intra     int
	TxnBytes  []byte
	Extra     TxnExtra
	Error     error
}

type TxnExtra struct {
	AssetCloseAmount uint64 `codec:"aca,omitempty"`
}

// TODO: sqlite3 impl
// TODO: cockroachdb impl
type IndexerDb interface {
	// The next few functions define the import interface, functions for loading data into the database. StartBlock() through Get/SetMetastate().

	StartBlock() error
	AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error
	CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error

	AlreadyImported(path string) (imported bool, err error)
	MarkImported(path string) (err error)

	LoadGenesis(genesis types.Genesis) (err error)
	SetProto(version string, proto types.ConsensusParams) (err error)
	GetProto(version string) (proto types.ConsensusParams, err error)

	GetMetastate(key string) (jsonStrValue string, err error)
	SetMetastate(key, jsonStrValue string) (err error)
	GetMaxRound() (round uint64, err error)

	// YieldTxns returns a channel that produces the whole transaction stream after some round forward
	YieldTxns(ctx context.Context, prevRound int64) <-chan TxnRow

	CommitRoundAccounting(updates RoundUpdates, round, rewardsBase uint64) (err error)

	GetBlock(round uint64) (block types.Block, err error)

	Transactions(ctx context.Context, tf TransactionFilter) <-chan TxnRow
	GetAccounts(ctx context.Context, opts AccountQueryOptions) <-chan AccountRow
	Assets(ctx context.Context, filter AssetsQuery) <-chan AssetRow
	AssetBalances(ctx context.Context, abq AssetBalanceQuery) <-chan AssetBalanceRow
}

func GetAccount(idb IndexerDb, addr []byte) (account types.Account, err error) {
	for ar := range idb.GetAccounts(context.Background(), AccountQueryOptions{EqualToAddress: addr}) {
		return ar.Account, ar.Error
	}
	return types.Account{}, nil
}

// TransactionFilter.AddressRole bitfield values
const (
	AddressRoleSender           = 0x01
	AddressRoleReceiver         = 0x02
	AddressRoleCloseRemainderTo = 0x04
	AddressRoleAssetSender      = 0x08
	AddressRoleAssetReceiver    = 0x10
	AddressRoleAssetCloseTo     = 0x20
	AddressRoleFreeze           = 0x40
)

type TransactionFilter struct {
	// Address filtering transactions for one Address will
	// return transactions newest-first proceding into the
	// past. Paging through such results can be achieved by
	// setting a MaxRound to get results before.
	Address []byte

	AddressRole uint64 // 0=Any, otherwise AddressRole* bitfields above

	MinRound   uint64
	MaxRound   uint64
	AfterTime  time.Time
	BeforeTime time.Time
	TypeEnum   int // ["","pay","keyreg","acfg","axfer","afrz"]
	Txid       []byte
	Round      *uint64 // nil for no filter
	Offset     *uint64 // nil for no filter
	SigType    string  // ["", "sig", "msig", "lsig"]
	NotePrefix []byte
	MinAlgos   uint64 // implictly filters on "pay" txns for Algos >= this. This will be a slightly faster query than EffectiveAmountGt.
	MaxAlgos   uint64

	AssetId        uint64 // filter transactions relevant to an asset
	MinAssetAmount uint64
	MaxAssetAmount uint64

	EffectiveAmountGt uint64 // Algo: Amount + CloseAmount > x
	EffectiveAmountLt uint64 // Algo: Amount + CloseAmount < x

	Limit uint64
}

type AccountQueryOptions struct {
	GreaterThanAddress []byte // for paging results
	EqualToAddress     []byte // return exactly this one account

	// Filter on accounts with current balance greater than x
	AlgosGreaterThan uint64
	// Filter on accounts with current balance less than x.
	AlgosLessThan uint64

	IncludeAssetHoldings bool
	IncludeAssetParams   bool

	Limit uint64
}

type AccountRow struct {
	Account types.Account
	Error   error
}

type AssetsQuery struct {
	AssetId            uint64
	AssetIdGreaterThan uint64

	Creator []byte

	// Name checks for exact match for asset name
	Name string
	// Unit checks for exact match of unit name
	Unit string
	// Query checks for fuzzy match against either asset name or unit name
	// (assetname ILIKE '%?%' OR unitname ILIKE '%?%')
	Query string

	Limit uint64
}

type AssetRow struct {
	AssetId uint64
	Creator []byte
	Params  types.AccountAssetParams
	Error   error
}

type AssetBalanceQuery struct {
	AssetId   uint64
	MinAmount uint64 // only rows >= this
	MaxAmount uint64 // only rows <= this

	Limit uint64 // max rows to return

	// PrevAddress for paging, the last item from the previous
	// query (items returned in address order)
	PrevAddress []byte
}

type AssetBalanceRow struct {
	Address []byte
	AssetId uint64
	Amount  uint64
	Frozen  bool
	Error   error
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

type KeyregUpdate struct {
	Addr            types.Address
	Status          int // {Offline:0, Online:1, NotParticipating: 2}
	VoteID          [32]byte
	SelectionID     [32]byte
	VoteFirstValid  uint64
	VoteLastValid   uint64
	VoteKeyDilution uint64
}

type AcfgUpdate struct {
	AssetId uint64
	Creator types.Address
	Params  types.AssetParams
}

type AssetUpdate struct {
	AssetId       uint64
	Delta         big.Int
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
	Round         uint64
	Offset        uint64
	DefaultFrozen bool
}

type TxnAssetUpdate struct {
	Round   uint64
	Offset  int
	AssetId uint64
}

type RoundUpdates struct {
	AlgoUpdates     map[[32]byte]int64
	AccountTypes    map[[32]byte]string
	KeyregUpdates   []KeyregUpdate
	AcfgUpdates     []AcfgUpdate
	TxnAssetUpdates []TxnAssetUpdate
	AssetUpdates    map[[32]byte][]AssetUpdate
	FreezeUpdates   []FreezeUpdate
	AssetCloses     []AssetClose
	AssetDestroys   []uint64
}
