package postgres

import (
	"context"
	"math/rand"
	"testing"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	ledgerforevaluator "github.com/algorand/indexer/idb/postgres/internal/ledger_for_evaluator"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util/test"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateAssetParams() basics.AssetParams {
	return basics.AssetParams{
		Total: rand.Uint64(),
	}
}

func generateAssetHolding() basics.AssetHolding {
	return basics.AssetHolding{
		Amount: rand.Uint64(),
	}
}

func generateAppParams(t *testing.T) basics.AppParams {
	p := make([]byte, 100)
	_, err := rand.Read(p)
	require.NoError(t, err)

	return basics.AppParams{
		ApprovalProgram: p,
	}
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

func generateAccountData(t *testing.T) basics.AccountData {
	// Return empty account data with probability 50%.
	if rand.Uint32()%2 == 0 {
		return basics.AccountData{}
	}

	const numCreatables = 20

	res := basics.AccountData{
		MicroAlgos:     basics.MicroAlgos{Raw: uint64(rand.Int63())},
		AssetParams:    make(map[basics.AssetIndex]basics.AssetParams),
		Assets:         make(map[basics.AssetIndex]basics.AssetHolding),
		AppLocalStates: make(map[basics.AppIndex]basics.AppLocalState),
		AppParams:      make(map[basics.AppIndex]basics.AppParams),
	}

	for i := 0; i < numCreatables; i++ {
		{
			index := basics.AssetIndex(rand.Int63())
			res.AssetParams[index] = generateAssetParams()
		}
		{
			index := basics.AssetIndex(rand.Int63())
			res.Assets[index] = generateAssetHolding()
		}
		{
			index := basics.AppIndex(rand.Int63())
			res.AppLocalStates[index] = generateAppLocalState(t)
		}
		{
			index := basics.AppIndex(rand.Int63())
			res.AppParams[index] = generateAppParams(t)
		}
	}

	return res
}

// Write random account data for many random accounts, then read it and compare.
// Tests in particular that batch writing and reading is done in the same order
// and that there are no problems around passing account address pointers to the postgres
// driver which could be the same pointer if we are not careful.
func TestWriteReadAccountData(t *testing.T) {
	db, shutdownFunc := setupIdb(t, test.MakeGenesis(), test.MakeGenesisBlock())
	defer shutdownFunc()

	addresses := make(map[basics.Address]struct{})
	var delta ledgercore.StateDelta
	for i := 0; i < 1000; i++ {
		var address basics.Address
		_, err := rand.Read(address[:])
		require.NoError(t, err)

		addresses[address] = struct{}{}
		delta.Accts.Upsert(address, generateAccountData(t))
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&bookkeeping.Block{}, transactions.Payset{}, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err := db.txWithRetry(serializable, f)
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), serializable)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledgerforevaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
	require.NoError(t, err)

	// Load all accounts in a batch.
	err = l.PreloadAccounts(addresses)
	require.NoError(t, err)

	for address := range addresses {
		ret, _, err := l.LookupWithoutRewards(basics.Round(0), address)
		require.NoError(t, err)

		expected, ok := delta.Accts.Get(address)
		require.True(t, ok)

		assert.Equal(t, expected, ret)
	}
}
