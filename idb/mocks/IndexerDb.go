// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import (
	context "context"

	generated "github.com/algorand/indexer/api/generated/v2"
	idb "github.com/algorand/indexer/idb"

	mock "github.com/stretchr/testify/mock"

	types "github.com/algorand/indexer/types"
)

// IndexerDb is an autogenerated mock type for the IndexerDb type
type IndexerDb struct {
	mock.Mock
}

// AddTransaction provides a mock function with given fields: round, intra, txtypeenum, assetid, txn, participation
func (_m *IndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error {
	ret := _m.Called(round, intra, txtypeenum, assetid, txn, participation)

	var r0 error
	if rf, ok := ret.Get(0).(func(uint64, int, int, uint64, types.SignedTxnWithAD, [][]byte) error); ok {
		r0 = rf(round, intra, txtypeenum, assetid, txn, participation)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// AlreadyImported provides a mock function with given fields: path
func (_m *IndexerDb) AlreadyImported(path string) (bool, error) {
	ret := _m.Called(path)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string) bool); ok {
		r0 = rf(path)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(path)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Applications provides a mock function with given fields: ctx, filter
func (_m *IndexerDb) Applications(ctx context.Context, filter *generated.SearchForApplicationsParams) <-chan idb.ApplicationRow {
	ret := _m.Called(ctx, filter)

	var r0 <-chan idb.ApplicationRow
	if rf, ok := ret.Get(0).(func(context.Context, *generated.SearchForApplicationsParams) <-chan idb.ApplicationRow); ok {
		r0 = rf(ctx, filter)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan idb.ApplicationRow)
		}
	}

	return r0
}

// AssetBalances provides a mock function with given fields: ctx, abq
func (_m *IndexerDb) AssetBalances(ctx context.Context, abq idb.AssetBalanceQuery) <-chan idb.AssetBalanceRow {
	ret := _m.Called(ctx, abq)

	var r0 <-chan idb.AssetBalanceRow
	if rf, ok := ret.Get(0).(func(context.Context, idb.AssetBalanceQuery) <-chan idb.AssetBalanceRow); ok {
		r0 = rf(ctx, abq)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan idb.AssetBalanceRow)
		}
	}

	return r0
}

// Assets provides a mock function with given fields: ctx, filter
func (_m *IndexerDb) Assets(ctx context.Context, filter idb.AssetsQuery) <-chan idb.AssetRow {
	ret := _m.Called(ctx, filter)

	var r0 <-chan idb.AssetRow
	if rf, ok := ret.Get(0).(func(context.Context, idb.AssetsQuery) <-chan idb.AssetRow); ok {
		r0 = rf(ctx, filter)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan idb.AssetRow)
		}
	}

	return r0
}

// CommitBlock provides a mock function with given fields: round, timestamp, rewardslevel, headerbytes
func (_m *IndexerDb) CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
	ret := _m.Called(round, timestamp, rewardslevel, headerbytes)

	var r0 error
	if rf, ok := ret.Get(0).(func(uint64, int64, uint64, []byte) error); ok {
		r0 = rf(round, timestamp, rewardslevel, headerbytes)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CommitRoundAccounting provides a mock function with given fields: updates, round, blockPtr
func (_m *IndexerDb) CommitRoundAccounting(updates idb.RoundUpdates, round uint64, blockPtr *types.Block) error {
	ret := _m.Called(updates, round, blockPtr)

	var r0 error
	if rf, ok := ret.Get(0).(func(idb.RoundUpdates, uint64, *types.Block) error); ok {
		r0 = rf(updates, round, blockPtr)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetAccounts provides a mock function with given fields: ctx, opts
func (_m *IndexerDb) GetAccounts(ctx context.Context, opts idb.AccountQueryOptions) <-chan idb.AccountRow {
	ret := _m.Called(ctx, opts)

	var r0 <-chan idb.AccountRow
	if rf, ok := ret.Get(0).(func(context.Context, idb.AccountQueryOptions) <-chan idb.AccountRow); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan idb.AccountRow)
		}
	}

	return r0
}

// GetBlock provides a mock function with given fields: round
func (_m *IndexerDb) GetBlock(round uint64) (types.Block, error) {
	ret := _m.Called(round)

	var r0 types.Block
	if rf, ok := ret.Get(0).(func(uint64) types.Block); ok {
		r0 = rf(round)
	} else {
		r0 = ret.Get(0).(types.Block)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(uint64) error); ok {
		r1 = rf(round)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetMaxRound provides a mock function with given fields:
func (_m *IndexerDb) GetMaxRound() (uint64, error) {
	ret := _m.Called()

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetMetastate provides a mock function with given fields: key
func (_m *IndexerDb) GetMetastate(key string) (string, error) {
	ret := _m.Called(key)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(key)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(key)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetProto provides a mock function with given fields: version
func (_m *IndexerDb) GetProto(version string) (types.ConsensusParams, error) {
	ret := _m.Called(version)

	var r0 types.ConsensusParams
	if rf, ok := ret.Get(0).(func(string) types.ConsensusParams); ok {
		r0 = rf(version)
	} else {
		r0 = ret.Get(0).(types.ConsensusParams)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(version)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetSpecialAccounts provides a mock function with given fields:
func (_m *IndexerDb) GetSpecialAccounts() (idb.SpecialAccounts, error) {
	ret := _m.Called()

	var r0 idb.SpecialAccounts
	if rf, ok := ret.Get(0).(func() idb.SpecialAccounts); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(idb.SpecialAccounts)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Health provides a mock function with given fields:
func (_m *IndexerDb) Health() (idb.Health, error) {
	ret := _m.Called()

	var r0 idb.Health
	if rf, ok := ret.Get(0).(func() idb.Health); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(idb.Health)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LoadGenesis provides a mock function with given fields: genesis
func (_m *IndexerDb) LoadGenesis(genesis types.Genesis) error {
	ret := _m.Called(genesis)

	var r0 error
	if rf, ok := ret.Get(0).(func(types.Genesis) error); ok {
		r0 = rf(genesis)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MarkImported provides a mock function with given fields: path
func (_m *IndexerDb) MarkImported(path string) error {
	ret := _m.Called(path)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(path)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetMetastate provides a mock function with given fields: key, jsonStrValue
func (_m *IndexerDb) SetMetastate(key string, jsonStrValue string) error {
	ret := _m.Called(key, jsonStrValue)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(key, jsonStrValue)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetProto provides a mock function with given fields: version, proto
func (_m *IndexerDb) SetProto(version string, proto types.ConsensusParams) error {
	ret := _m.Called(version, proto)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, types.ConsensusParams) error); ok {
		r0 = rf(version, proto)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// StartBlock provides a mock function with given fields:
func (_m *IndexerDb) StartBlock() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Transactions provides a mock function with given fields: ctx, tf
func (_m *IndexerDb) Transactions(ctx context.Context, tf idb.TransactionFilter) <-chan idb.TxnRow {
	ret := _m.Called(ctx, tf)

	var r0 <-chan idb.TxnRow
	if rf, ok := ret.Get(0).(func(context.Context, idb.TransactionFilter) <-chan idb.TxnRow); ok {
		r0 = rf(ctx, tf)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan idb.TxnRow)
		}
	}

	return r0
}

// YieldTxns provides a mock function with given fields: ctx, prevRound
func (_m *IndexerDb) YieldTxns(ctx context.Context, prevRound int64) <-chan idb.TxnRow {
	ret := _m.Called(ctx, prevRound)

	var r0 <-chan idb.TxnRow
	if rf, ok := ret.Get(0).(func(context.Context, int64) <-chan idb.TxnRow); ok {
		r0 = rf(ctx, prevRound)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan idb.TxnRow)
		}
	}

	return r0
}
