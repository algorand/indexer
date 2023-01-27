package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	models2 "github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/require"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util/test"
)

func generateAddress(t *testing.T) sdk.Address {
	var res sdk.Address
	_, err := rand.Read(res[:])
	require.NoError(t, err)

	return res
}

func generateAccountData() models2.Account {
	// Return empty account data with probability 50%.
	if rand.Uint32()%2 == 0 {
		return models2.Account{}
	}

	res := models2.Account{
		Amount: uint64(rand.Int63()),
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesisV2())
	defer shutdownFunc()

	data := make(map[sdk.Address]models2.Account)
	var delta models2.LedgerStateDelta
	for i := 0; i < 1000; i++ {
		address := generateAddress(t)

		acctData := generateAccountData()
		data[address] = acctData
		//delta.Accts.Upsert(address, acctData)
		abr := models2.AccountBalanceRecord{
			AccountData: acctData,
			Address:     address.String(),
		}
		delta.Accts.Accounts = append(delta.Accts.Accounts, abr)
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

		if reflect.DeepEqual(expected, models2.Account{}) {
			require.Nil(t, account)
		} else {
			require.Equal(t, expected.Amount, account.Amount)
		}
	}
}

func generateAssetParams() models2.AssetParams {
	return models2.AssetParams{
		Total: rand.Uint64(),
	}
}

func generateAssetParamsDelta() models2.AssetResourceRecord {
	var res models2.AssetResourceRecord

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.AssetDeleted = true
	case 1:
		res.AssetParams = models2.AssetParams{}
		res.AssetParams = generateAssetParams()
	case 2:
		// do nothing
	}

	return res
}

func generateAssetHolding() models2.AssetHolding {
	return models2.AssetHolding{
		Amount: rand.Uint64(),
	}
}

func generateAssetHoldingDelta() models2.AssetResourceRecord {
	//var res ledgercore.AssetHoldingDelta
	var res models2.AssetResourceRecord

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.AssetHoldingDeleted = true
	case 1:
		res.AssetHolding = models2.AssetHolding{}
		res.AssetHolding = generateAssetHolding()
	case 2:
		// do nothing
	}

	return res
}

func generateAppParams(t *testing.T) models2.ApplicationParams {
	p := make([]byte, 100)
	_, err := rand.Read(p)
	require.NoError(t, err)

	return models2.ApplicationParams{
		ApprovalProgram: p,
	}
}

func generateAppParamsDelta(t *testing.T) models2.AppResourceRecord {
	var res models2.AppResourceRecord

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.AppDeleted = true
	case 1:
		res.AppParams = models2.ApplicationParams{}
		res.AppParams = generateAppParams(t)
	case 2:
		// do nothing
	}

	return res
}

func generateAppLocalState(t *testing.T) models2.ApplicationLocalState {
	k := make([]byte, 100)
	_, err := rand.Read(k)
	require.NoError(t, err)

	v := make([]byte, 100)
	_, err = rand.Read(v)
	require.NoError(t, err)

	return models2.ApplicationLocalState{
		//KeyValue: map[string]basics.TealValue{
		//	string(k): {
		//		Bytes: string(v),
		//	},
		//},
		KeyValue: []models2.TealKeyValue{
			{
				Key: string(k),
				Value: models2.TealValue{
					Bytes: string(v),
				},
			},
		},
	}
}

func generateAppLocalStateDelta(t *testing.T) models2.AppResourceRecord {
	var res models2.AppResourceRecord

	r := rand.Uint32() % 3
	switch r {
	case 0:
		res.AppLocalState.Deleted = true
	case 1:
		res.AppLocalState = models2.ApplicationLocalState{}
		res.AppLocalState = generateAppLocalState(t)
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
	db, shutdownFunc := setupIdb(t, test.MakeGenesisV2())
	defer shutdownFunc()

	// TODO: generate a resource record?
	type datum struct {
		assetIndex  sdk.AssetIndex
		assetParams models2.AssetResourceRecord
		holding     models2.AssetResourceRecord
		appIndex    sdk.AppIndex
		appParams   models2.AppResourceRecord
		localState  models2.AppResourceRecord
	}

	data := make(map[sdk.Address]datum)
	var delta models2.LedgerStateDelta
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

		assetResRec := models2.AssetResourceRecord{
			Address:             address.String(),
			AssetDeleted:        assetParams.AssetDeleted,
			AssetHolding:        holding.AssetHolding,
			AssetHoldingDeleted: holding.AssetHolding.Deleted,
			AssetIndex:          uint64(assetIndex),
			AssetParams:         assetParams.AssetParams,
		}

		appsResRec := models2.AppResourceRecord{
			Address:              address.String(),
			AppParams:            appParamsDelta.AppParams,
			AppLocalState:        localState.AppLocalState,
			AppLocalStateDeleted: appParamsDelta.AppLocalStateDeleted,
			AppDeleted:           appParamsDelta.AppDeleted,
			AppIndex:             uint64(appIndex),
		}

		//delta.Accts.UpsertAssetResource(address, assetIndex, assetParams, holding)
		delta.Accts.Assets = append(delta.Accts.Assets, assetResRec)
		//delta.Accts.UpsertAppResource(address, appIndex, appParamsDelta, localState)
		delta.Accts.Apps = append(delta.Accts.Apps, appsResRec)
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

	assetMap := getAssetResource(delta.Accts.Assets)
	appMap := getAppResource(delta.Accts.Apps)
	for address, datum := range data {
		fmt.Println(base64.StdEncoding.EncodeToString(address[:]))

		// asset info
		//assetParams, _ := delta.Accts.GetAssetParams(address, datum.assetIndex)
		//require.Equal(t, datum.assetParams, assetParams)
		//holding, _ := delta.Accts.GetAssetHolding(address, datum.assetIndex)
		//require.Equal(t, datum.holding, holding)
		asset := assetMap[Key{address.String(), uint64(datum.assetIndex)}]
		assetParams := asset.AssetParams
		require.Equal(t, datum.assetParams.AssetParams, assetParams)
		holding := asset.AssetHolding
		require.Equal(t, datum.holding.AssetHolding, holding)

		// app info
		//appParams, _ := delta.Accts.GetAppParams(address, datum.appIndex)
		//require.Equal(t, datum.appParams, appParams)
		//localState, _ := delta.Accts.GetAppLocalState(address, datum.appIndex)
		//require.Equal(t, datum.localState, localState)
		apps := appMap[Key{address.String(), uint64(datum.appIndex)}]
		appParams := apps.AppParams
		require.Equal(t, datum.appParams.AppParams, appParams)
		localState := apps.AppLocalState
		require.Equal(t, datum.localState.AppLocalState, localState)
	}
}

type Key struct {
	address string
	idx     uint64
}

func getAssetResource(assetsRecord []models2.AssetResourceRecord) map[Key]models2.AssetResourceRecord {

	ret := make(map[Key]models2.AssetResourceRecord)
	for _, resource := range assetsRecord {
		k := Key{
			address: resource.Address,
			idx:     resource.AssetIndex,
		}
		ret[k] = resource
	}
	return ret
}

func getAppResource(appRecord []models2.AppResourceRecord) map[Key]models2.AppResourceRecord {

	ret := make(map[Key]models2.AppResourceRecord)
	for _, resource := range appRecord {
		k := Key{
			address: resource.Address,
			idx:     resource.AppIndex,
		}
		ret[k] = resource
	}
	return ret
}
