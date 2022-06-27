package postgres

import (
	"context"
	"math/rand"
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	ledgerforevaluator "github.com/algorand/indexer/idb/postgres/internal/ledger_for_evaluator"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util/test"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateAddress(t *testing.T) basics.Address {
	var res basics.Address

	_, err := rand.Read(res[:])
	require.NoError(t, err)

	return res
}

func generateAccountData() ledgercore.AccountData {
	// Return empty account data with probability 50%.
	if rand.Uint32()%2 == 0 {
		return ledgercore.AccountData{}
	}

	res := ledgercore.AccountData{
		AccountBaseData: ledgercore.AccountBaseData{
			MicroAlgos: basics.MicroAlgos{Raw: uint64(rand.Int63())},
		},
	}

	return res
}

// Write random account data for many random accounts, then read it and compare.
// Tests in particular that batch writing and reading is done in the same order
// and that there are no problems around passing account address pointers to the postgres
// driver which could be the same pointer if we are not careful.
func TestWriteReadAccountData(t *testing.T) {
	db, shutdownFunc, _, ld := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	defer ld.Close()

	addresses := make(map[basics.Address]struct{})
	var delta ledgercore.StateDelta
	for i := 0; i < 1000; i++ {
		address := generateAddress(t)

		addresses[address] = struct{}{}
		delta.Accts.Upsert(address, generateAccountData())
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&bookkeeping.Block{}, transactions.Payset{}, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := db.txWithRetry(serializable, f)
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), serializable)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledgerforevaluator.MakeLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.LookupWithoutRewards(addresses)
	require.NoError(t, err)

	for address := range addresses {
		expected, ok := delta.Accts.GetData(address)
		require.True(t, ok)

		ret, ok := ret[address]
		require.True(t, ok)

		if ret == nil {
			require.True(t, expected.IsZero())
		} else {
			require.Equal(t, &expected, ret)
		}
	}
}

func generateAssetParams() basics.AssetParams {
	return basics.AssetParams{
		Total: rand.Uint64(),
	}
}

func generateAssetParamsDelta() ledgercore.AssetParamsDelta {
	var res ledgercore.AssetParamsDelta

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.Deleted = true
	case 1:
		res.Params = new(basics.AssetParams)
		*res.Params = generateAssetParams()
	case 2:
		// do nothing
	}

	return res
}

func generateAssetHolding() basics.AssetHolding {
	return basics.AssetHolding{
		Amount: rand.Uint64(),
	}
}

func generateAssetHoldingDelta() ledgercore.AssetHoldingDelta {
	var res ledgercore.AssetHoldingDelta

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.Deleted = true
	case 1:
		res.Holding = new(basics.AssetHolding)
		*res.Holding = generateAssetHolding()
	case 2:
		// do nothing
	}

	return res
}

func generateAppParams(t *testing.T) basics.AppParams {
	p := make([]byte, 100)
	_, err := rand.Read(p)
	require.NoError(t, err)

	return basics.AppParams{
		ApprovalProgram: p,
	}
}

func generateAppParamsDelta(t *testing.T) ledgercore.AppParamsDelta {
	var res ledgercore.AppParamsDelta

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.Deleted = true
	case 1:
		res.Params = new(basics.AppParams)
		*res.Params = generateAppParams(t)
	case 2:
		// do nothing
	}

	return res
}

func generateAppLocalState(t *testing.T) basics.AppLocalState {
	k := make([]byte, 100)
	_, err := rand.Read(k)
	require.NoError(t, err)

	v := make([]byte, 100)
	_, err = rand.Read(v)
	require.NoError(t, err)

	return basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			string(k): {
				Bytes: string(v),
			},
		},
	}
}

func generateAppLocalStateDelta(t *testing.T) ledgercore.AppLocalStateDelta {
	var res ledgercore.AppLocalStateDelta

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.Deleted = true
	case 1:
		res.LocalState = new(basics.AppLocalState)
		*res.LocalState = generateAppLocalState(t)
	case 2:
		// do nothing
	}

	return res
}

// Write random assets and apps, then read it and compare.
// Tests in particular that batch writing and reading is done in the same order
// and that there are no problems around passing account address pointers to the postgres
// driver which could be the same pointer if we are not careful.
func TestWriteReadResources(t *testing.T) {
	db, shutdownFunc, _, ld := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	defer ld.Close()

	resources := make(map[basics.Address]map[ledger.Creatable]struct{})
	var delta ledgercore.StateDelta
	for i := 0; i < 1000; i++ {
		address := generateAddress(t)
		assetIndex := basics.AssetIndex(rand.Int63())
		appIndex := basics.AppIndex(rand.Int63())

		{
			c := make(map[ledger.Creatable]struct{})
			resources[address] = c

			creatable := ledger.Creatable{
				Index: basics.CreatableIndex(assetIndex),
				Type:  basics.AssetCreatable,
			}
			c[creatable] = struct{}{}

			creatable = ledger.Creatable{
				Index: basics.CreatableIndex(appIndex),
				Type:  basics.AppCreatable,
			}
			c[creatable] = struct{}{}
		}

		delta.Accts.UpsertAssetResource(
			address, assetIndex, generateAssetParamsDelta(),
			generateAssetHoldingDelta())
		delta.Accts.UpsertAppResource(
			address, appIndex, generateAppParamsDelta(t),
			generateAppLocalStateDelta(t))
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&bookkeeping.Block{}, transactions.Payset{}, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := db.txWithRetry(serializable, f)
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), serializable)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledgerforevaluator.MakeLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.LookupResources(resources)
	require.NoError(t, err)

	for address, creatables := range resources {
		ret, ok := ret[address]
		require.True(t, ok)

		for creatable := range creatables {
			ret, ok := ret[creatable]
			require.True(t, ok)

			switch creatable.Type {
			case basics.AssetCreatable:
				assetParamsDelta, _ :=
					delta.Accts.GetAssetParams(address, basics.AssetIndex(creatable.Index))
				assert.Equal(t, assetParamsDelta.Params, ret.AssetParams)

				assetHoldingDelta, _ :=
					delta.Accts.GetAssetHolding(address, basics.AssetIndex(creatable.Index))
				assert.Equal(t, assetHoldingDelta.Holding, ret.AssetHolding)
			case basics.AppCreatable:
				appParamsDelta, _ :=
					delta.Accts.GetAppParams(address, basics.AppIndex(creatable.Index))
				assert.Equal(t, appParamsDelta.Params, ret.AppParams)

				appLocalStateDelta, _ :=
					delta.Accts.GetAppLocalState(address, basics.AppIndex(creatable.Index))
				assert.Equal(t, appLocalStateDelta.LocalState, ret.AppLocalState)
			}
		}
	}
}
