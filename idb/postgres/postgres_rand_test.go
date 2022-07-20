package postgres

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/data/transactions/logic"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/protocol"

	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	ledgerforevaluator "github.com/algorand/indexer/processor/eval"
	"github.com/algorand/indexer/util/test"
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

	l := ledgerforevaluator.MakeLedgerForEvaluator(ld)
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

	l := ledgerforevaluator.MakeLedgerForEvaluator(ld)
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

// generateBoxes generates a random slice of box keys and values for an app using future consensus params for guidance.
// NOTE: no attempt is made to adhere to the constraints BytesPerBoxReference etc.
func generateBoxes(t *testing.T, appIdx basics.AppIndex, maxBoxes int) map[string]string {
	future := config.Consensus[protocol.ConsensusFuture]

	numBoxes := rand.Intn(maxBoxes + 1)
	boxes := make(map[string]string)
	for i := 0; i < numBoxes; i++ {
		nameLen := rand.Intn(future.MaxAppKeyLen + 1)
		size := rand.Intn(int(future.MaxBoxSize) + 1)

		nameBytes := make([]byte, nameLen)
		_, err := rand.Read(nameBytes)
		require.NoError(t, err)
		key := logic.MakeBoxKey(appIdx, string(nameBytes))

		require.Positive(t, len(key))

		valueBytes := make([]byte, size)
		_, err = rand.Read(valueBytes)
		require.NoError(t, err)

		boxes[key] = string(valueBytes)
	}
	return boxes
}

func createBoxesWithDelta(t *testing.T) (map[basics.AppIndex]map[string]string, ledgercore.StateDelta) {
	numApps, maxBoxes := 10, 2500
	appBoxes := make(map[basics.AppIndex]map[string]string)

	delta := ledgercore.StateDelta{
		KvMods: map[string]*string{},
		Accts:  ledgercore.MakeAccountDeltas(numApps),
	}

	for i := 0; i < numApps; i++ {
		appIndex := basics.AppIndex(rand.Int63())
		boxes := generateBoxes(t, appIndex, maxBoxes)
		appBoxes[appIndex] = boxes

		// totalBoxes := len(boxes)
		totalBoxBytes := 0

		for key, value := range boxes {
			embeddedAppIdx, name, err := logic.SplitBoxKey(key)
			require.NoError(t, err)
			require.Equal(t, appIndex, embeddedAppIdx)

			val := string([]byte(value)[:])
			delta.KvMods[key] = &val

			totalBoxBytes += len(name) + len(value)
		}

	}
	return appBoxes, delta
}

func mutateSomeBoxesWithDelta(t *testing.T, appBoxes map[basics.AppIndex]map[string]string) ledgercore.StateDelta {
	var delta ledgercore.StateDelta
	delta.KvMods = make(map[string]*string)

	for _, boxes := range appBoxes {
		for key, value := range boxes {
			if rand.Intn(2) == 0 {
				continue
			}
			valueBytes := make([]byte, len(value))
			_, err := rand.Read(valueBytes)
			require.NoError(t, err)
			boxes[key] = string(valueBytes)

			val := string([]byte(boxes[key])[:])
			delta.KvMods[key] = &val
		}
	}

	return delta
}

func deleteSomeBoxesWithDelta(t *testing.T, appBoxes map[basics.AppIndex]map[string]string) (map[basics.AppIndex]map[string]bool, ledgercore.StateDelta) {
	deletedBoxes := make(map[basics.AppIndex]map[string]bool, len(appBoxes))

	var delta ledgercore.StateDelta
	delta.KvMods = make(map[string]*string)

	for appIndex, boxes := range appBoxes {
		deletedBoxes[appIndex] = map[string]bool{}
		for key := range boxes {
			if rand.Intn(2) == 0 {
				continue
			}
			deletedBoxes[appIndex][key] = true
			delta.KvMods[key] = nil
		}
	}

	return deletedBoxes, delta
}

func addAppBoxesBlock(t *testing.T, db *IndexerDb, delta ledgercore.StateDelta) {
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
}

func CompareAppBoxesAgainstDB(t *testing.T, db *IndexerDb,
	appBoxes map[basics.AppIndex]map[string]string, extras ...map[basics.AppIndex]map[string]bool) {
	require.LessOrEqual(t, len(extras), 1)
	var deletedBoxes map[basics.AppIndex]map[string]bool
	if len(extras) == 1 {
		deletedBoxes = extras[0]
	}

	appBoxSQL := `SELECT app, name, value FROM app_box WHERE app = $1 AND name = $2`
	appAccountSummarySQL := "SELECT account_data FROM account WHERE addr = $1"

	caseNum := 1
	for appIdx, boxes := range appBoxes {
		var totalBoxes, totalBoxBytes int

		// compare expected against db contents one box at a time
		for key, expectedValue := range boxes {
			msg := fmt.Sprintf("caseNum=%d, appIdx=%d, key=%#v", caseNum, appIdx, key)
			expectedAppIdx, boxName, err := logic.SplitBoxKey(key)
			require.NoError(t, err, msg)
			require.Equal(t, appIdx, expectedAppIdx, msg)

			row := db.db.QueryRow(context.Background(), appBoxSQL, appIdx, []byte(boxName))

			boxDeleted := false
			if deletedBoxes != nil {
				if _, ok := deletedBoxes[appIdx][key]; ok {
					boxDeleted = true
				}
			}

			var app basics.AppIndex
			var name, value []byte
			err = row.Scan(&app, &name, &value)
			if !boxDeleted {
				require.NoError(t, err, msg)
				require.Equal(t, expectedAppIdx, app, msg)
				require.Equal(t, boxName, string(name), msg)
				require.Equal(t, expectedValue, string(value), msg)

				totalBoxes++
				totalBoxBytes += len(boxName) + len(expectedValue)
			} else {
				require.ErrorContains(t, err, "no rows in result set", msg)
			}
		}

		// compare the summary box info for the app
		address := appIdx.Address()
		row := db.db.QueryRow(context.Background(), appAccountSummarySQL, address[:])

		var buf []byte
		err := row.Scan(&buf)
		require.NoError(t, err)

		ret, err := encoding.DecodeTrimmedLcAccountData(buf)
		require.NoError(t, err)

		msg := fmt.Sprintf("error in totalling for appIdx=%d and caseNum=%d", appIdx, caseNum)
		require.Equal(t, uint64(totalBoxes), ret.TotalBoxes, msg)
		require.Equal(t, uint64(totalBoxBytes), ret.TotalBoxBytes, msg)

		caseNum++
	}
}

// Write random apps with random box names and values, then read them from indexer DB and compare.
// NOTE: this does not populate TotalBoxes nor TotalBoxBytes deep under StateDeltas.Accts and therefore
// no query is taken to compare the summary box information in `account.account_data`
// Mutate some boxes and repeat the comparison.
// Delete some boxes and repeat the comparison.
func TestWriteReadBoxes(t *testing.T) {
	db, shutdownFunc, _, ld := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	defer ld.Close()

	appBoxes, delta := createBoxesWithDelta(t)
	addAppBoxesBlock(t, db, delta)
	CompareAppBoxesAgainstDB(t, db, appBoxes)

	delta = mutateSomeBoxesWithDelta(t, appBoxes)
	addAppBoxesBlock(t, db, delta)
	CompareAppBoxesAgainstDB(t, db, appBoxes)

	deletedBoxes, delta := deleteSomeBoxesWithDelta(t, appBoxes)
	addAppBoxesBlock(t, db, delta)
	CompareAppBoxesAgainstDB(t, db, appBoxes, deletedBoxes)
}
