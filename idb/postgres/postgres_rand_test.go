package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"testing"

	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util/test"
)

func generateAddress(t *testing.T) basics.Address {
	var res basics.Address
	rand.Seed(1234)
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

func maybeGetAccount(t *testing.T, db *IndexerDb, address basics.Address) *models.Account {
	resultCh, _ := db.GetAccounts(context.Background(), idb.AccountQueryOptions{EqualToAddress: address[:]})
	num := 0
	var result *models.Account
	for row := range resultCh {
		num++
		require.NoError(t, row.Error)
		acct := row.Account
		result = &acct
	}
	require.LessOrEqual(t, num, 1, "There should be at most one result for the address.")
	return result
}

// Write random account data for many random accounts, then read it and compare.
// Tests in particular that batch writing and reading is done in the same order
// and that there are no problems around passing account address pointers to the postgres
// driver which could be the same pointer if we are not careful.
func TestWriteReadAccountData(t *testing.T) {
	db, shutdownFunc, _, ld := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	defer ld.Close()

	data := make(map[basics.Address]ledgercore.AccountData)
	var delta ledgercore.StateDelta
	for i := 0; i < 1000; i++ {
		address := generateAddress(t)

		acctData := generateAccountData()
		data[address] = acctData
		delta.Accts.Upsert(address, acctData)
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

	for address, expected := range data {
		account := maybeGetAccount(t, db, address)

		if expected.IsZero() {
			require.Nil(t, account)
		} else {
			require.Equal(t, expected.AccountBaseData.MicroAlgos.Raw, account.Amount)
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

	type datum struct {
		assetIndex  basics.AssetIndex
		assetParams ledgercore.AssetParamsDelta
		holding     ledgercore.AssetHoldingDelta
		appIndex    basics.AppIndex
		appParams   ledgercore.AppParamsDelta
		localState  ledgercore.AppLocalStateDelta
	}

	data := make(map[basics.Address]datum)
	var delta ledgercore.StateDelta
	for i := 0; i < 1000; i++ {
		address := generateAddress(t)

		assetIndex := basics.AssetIndex(rand.Int63())
		assetParams := generateAssetParamsDelta()
		holding := generateAssetHoldingDelta()

		appIndex := basics.AppIndex(rand.Int63())
		appParamsDelta := generateAppParamsDelta(t)
		localState := generateAppLocalStateDelta(t)

		data[address] = datum{
			assetIndex:  assetIndex,
			assetParams: assetParams,
			holding:     holding,
			appIndex:    appIndex,
			appParams:   appParamsDelta,
			localState:  localState,
		}

		delta.Accts.UpsertAssetResource(address, assetIndex, assetParams, holding)
		delta.Accts.UpsertAppResource(address, appIndex, appParamsDelta, localState)
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

	for address, datum := range data {
		fmt.Println(base64.StdEncoding.EncodeToString(address[:]))

		// asset info
		assetParams, _ := delta.Accts.GetAssetParams(address, datum.assetIndex)
		require.Equal(t, datum.assetParams, assetParams)
		holding, _ := delta.Accts.GetAssetHolding(address, datum.assetIndex)
		require.Equal(t, datum.holding, holding)

		// app info
		appParams, _ := delta.Accts.GetAppParams(address, datum.appIndex)
		require.Equal(t, datum.appParams, appParams)
		localState, _ := delta.Accts.GetAppLocalState(address, datum.appIndex)
		require.Equal(t, datum.localState, localState)
	}
}
