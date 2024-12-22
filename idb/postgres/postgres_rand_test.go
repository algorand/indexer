package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/require"

	models "github.com/algorand/indexer/v3/api/generated/v2"
	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/idb/postgres/internal/writer"
	"github.com/algorand/indexer/v3/util/test"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

func generateAddress(t *testing.T) sdk.Address {
	var res sdk.Address
	_, err := rand.Read(res[:])
	require.NoError(t, err)

	return res
}

func generateAccountData() sdk.AccountData {
	// Return empty account data with probability 50%.
	if rand.Uint32()%2 == 0 {
		return sdk.AccountData{}
	}

	res := sdk.AccountData{
		AccountBaseData: sdk.AccountBaseData{
			MicroAlgos: sdk.MicroAlgos(rand.Int63()),
		},
	}

	return res
}

func maybeGetAccount(t *testing.T, db *IndexerDb, address sdk.Address) *models.Account {
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	data := make(map[sdk.Address]sdk.AccountData)
	var delta sdk.LedgerStateDelta
	for i := 0; i < 1000; i++ {
		address := generateAddress(t)

		acctData := generateAccountData()
		data[address] = acctData
		abr := sdk.BalanceRecord{
			AccountData: acctData,
			Addr:        address,
		}
		delta.Accts.Accts = append(delta.Accts.Accts, abr)
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&sdk.Block{}, delta)
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
		if reflect.DeepEqual(expected, sdk.AccountData{}) {
			require.Nil(t, account)
		} else {
			require.Equal(t, uint64(expected.AccountBaseData.MicroAlgos), account.Amount)
		}
	}
}

func generateAssetParams() sdk.AssetParams {
	return sdk.AssetParams{
		Total: rand.Uint64(),
	}
}

func generateAssetParamsDelta() sdk.AssetParamsDelta {
	var res sdk.AssetParamsDelta

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.Deleted = true
	case 1:
		res.Params = new(sdk.AssetParams)
		*res.Params = generateAssetParams()
	case 2:
		// do nothing
	}

	return res
}

func generateAssetHolding() sdk.AssetHolding {
	return sdk.AssetHolding{
		Amount: rand.Uint64(),
	}
}

func generateAssetHoldingDelta() sdk.AssetHoldingDelta {
	var res sdk.AssetHoldingDelta

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.Deleted = true
	case 1:
		res.Holding = new(sdk.AssetHolding)
		*res.Holding = generateAssetHolding()
	case 2:
		// do nothing
	}

	return res
}

func generateAppParams(t *testing.T) sdk.AppParams {
	p := make([]byte, 100)
	_, err := rand.Read(p)
	require.NoError(t, err)

	return sdk.AppParams{
		ApprovalProgram: p,
	}
}

func generateAppParamsDelta(t *testing.T) sdk.AppParamsDelta {
	var res sdk.AppParamsDelta

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.Deleted = true
	case 1:
		res.Params = new(sdk.AppParams)
		*res.Params = generateAppParams(t)
	case 2:
		// do nothing
	}

	return res
}

func generateAppLocalState(t *testing.T) sdk.AppLocalState {
	k := make([]byte, 100)
	_, err := rand.Read(k)
	require.NoError(t, err)

	v := make([]byte, 100)
	_, err = rand.Read(v)
	require.NoError(t, err)

	return sdk.AppLocalState{
		KeyValue: map[string]sdk.TealValue{
			string(k): {
				Bytes: string(v),
			},
		},
	}
}

func generateAppLocalStateDelta(t *testing.T) sdk.AppLocalStateDelta {
	var res sdk.AppLocalStateDelta

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.Deleted = true
	case 1:
		res.LocalState = new(sdk.AppLocalState)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()

	type datum struct {
		assetIndex  sdk.AssetIndex
		assetParams sdk.AssetParamsDelta
		holding     sdk.AssetHoldingDelta
		appIndex    sdk.AppIndex
		appParams   sdk.AppParamsDelta
		localState  sdk.AppLocalStateDelta
	}

	data := make(map[sdk.Address]datum)
	var delta sdk.LedgerStateDelta
	for i := 0; i < 1000; i++ {
		address := generateAddress(t)

		assetIndex := sdk.AssetIndex(rand.Int63())
		assetParams := generateAssetParamsDelta()
		holding := generateAssetHoldingDelta()

		appIndex := sdk.AppIndex(rand.Int63())
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

		assetResRec := sdk.AssetResourceRecord{
			Aidx:    assetIndex,
			Addr:    address,
			Params:  assetParams,
			Holding: holding,
		}

		appsResRec := sdk.AppResourceRecord{
			Aidx:   appIndex,
			Addr:   address,
			Params: appParamsDelta,
			State:  localState,
		}

		delta.Accts.AssetResources = append(delta.Accts.AssetResources, assetResRec)
		delta.Accts.AppResources = append(delta.Accts.AppResources, appsResRec)
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&sdk.Block{}, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := db.txWithRetry(serializable, f)
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), serializable)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	assetMap := getAssetResource(delta.Accts.AssetResources)
	appMap := getAppResource(delta.Accts.AppResources)
	for address, datum := range data {
		fmt.Println(base64.StdEncoding.EncodeToString(address[:]))

		// asset info
		asset := assetMap[key{address, uint64(datum.assetIndex)}]
		assetParams := asset.Params
		require.Equal(t, datum.assetParams, assetParams)
		holding := asset.Holding
		require.Equal(t, datum.holding, holding)

		// app info
		apps := appMap[key{address, uint64(datum.appIndex)}]
		appParams := apps.Params
		require.Equal(t, datum.appParams, appParams)
		localState := apps.State
		require.Equal(t, datum.localState, localState)
	}
}

type key struct {
	address sdk.Address
	idx     uint64
}

func getAssetResource(assetsRecord []sdk.AssetResourceRecord) map[key]sdk.AssetResourceRecord {

	ret := make(map[key]sdk.AssetResourceRecord)
	for _, resource := range assetsRecord {
		k := key{
			address: resource.Addr,
			idx:     uint64(resource.Aidx),
		}
		ret[k] = resource
	}
	return ret
}

func getAppResource(appRecord []sdk.AppResourceRecord) map[key]sdk.AppResourceRecord {

	ret := make(map[key]sdk.AppResourceRecord)
	for _, resource := range appRecord {
		k := key{
			address: resource.Addr,
			idx:     uint64(resource.Aidx),
		}
		ret[k] = resource
	}
	return ret
}
