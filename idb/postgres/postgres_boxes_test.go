package postgres

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/require"

	"github.com/algorand/avm-abi/apps"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util/test"
)

type boxTestComparator func(t *testing.T, db *IndexerDb, appBoxes map[basics.AppIndex]map[string]string,
	deletedBoxes map[basics.AppIndex]map[string]bool, verifyTotals bool)

// compareAppBoxesAgainstDB is of type testing.BoxTestComparator
func compareAppBoxesAgainstDB(t *testing.T, db *IndexerDb,
	appBoxes map[basics.AppIndex]map[string]string,
	deletedBoxes map[basics.AppIndex]map[string]bool, verifyTotals bool) {

	numQueries := 0
	sumOfBoxes := 0
	sumOfBoxBytes := 0

	appBoxSQL := `SELECT app, name, value FROM app_box WHERE app = $1 AND name = $2`
	acctDataSQL := `SELECT account_data FROM account WHERE addr = $1`

	caseNum := 1
	var totalBoxes, totalBoxBytes int
	for appIdx, boxes := range appBoxes {
		totalBoxes = 0
		totalBoxBytes = 0

		// compare expected against db contents one box at a time
		for key, expectedValue := range boxes {
			msg := fmt.Sprintf("caseNum=%d, appIdx=%d, key=%#v", caseNum, appIdx, key)
			expectedAppIdx, boxName, err := apps.SplitBoxKey(key)
			require.NoError(t, err, msg)
			require.Equal(t, uint64(appIdx), expectedAppIdx, msg)

			row := db.db.QueryRow(context.Background(), appBoxSQL, appIdx, []byte(boxName))
			numQueries++

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
				require.Equal(t, expectedAppIdx, uint64(app), msg)
				require.Equal(t, boxName, string(name), msg)
				require.Equal(t, expectedValue, string(value), msg)

				totalBoxes++
				totalBoxBytes += len(boxName) + len(expectedValue)
			} else {
				require.ErrorContains(t, err, "no rows in result set", msg)
			}
		}
		if verifyTotals {
			addr := appIdx.Address()
			msg := fmt.Sprintf("caseNum=%d, appIdx=%d", caseNum, appIdx)

			row := db.db.QueryRow(context.Background(), acctDataSQL, addr[:])

			var buf []byte
			err := row.Scan(&buf)
			require.NoError(t, err, msg)

			ret, err := encoding.DecodeTrimmedLcAccountData(buf)
			require.NoError(t, err, msg)
			require.Equal(t, uint64(totalBoxes), ret.TotalBoxes, msg)
			require.Equal(t, uint64(totalBoxBytes), ret.TotalBoxBytes, msg)
		}

		sumOfBoxes += totalBoxes
		sumOfBoxBytes += totalBoxBytes
		caseNum++
	}

	fmt.Printf("compareAppBoxesAgainstDB succeeded with %d queries, %d boxes and %d boxBytes\n", numQueries, sumOfBoxes, sumOfBoxBytes)
}

// test runner copy/pastad/tweaked in handlers_e2e_test.go and postgres_integration_test.go
func runBoxCreateMutateDelete(t *testing.T, comparator boxTestComparator) {
	start := time.Now()

	db, shutdownFunc := setupIdb(t, test.MakeGenesisV2())
	defer shutdownFunc()

	appid := basics.AppIndex(1)

	// ---- ROUND 1: create and fund the box app  ---- //
	currentRound := basics.Round(1)

	//createTxn, err := test.MakeComplexCreateAppTxn(test.AccountA, test.BoxApprovalProgram, test.BoxClearProgram, 8)
	//require.NoError(t, err)
	//
	//payNewAppTxn := test.MakePaymentTxn(1000, 500000, 0, 0, 0, 0, test.AccountA, appid.Address(), basics.Address{},
	//	basics.Address{})
	//
	//block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &createTxn, &payNewAppTxn)
	//require.NoError(t, err)

	vb1, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDeleteR1.vb")
	require.NoError(t, err)
	blk1 := ledgercore.MakeValidatedBlock(vb1.Blk, vb1.Delta)
	err = db.AddBlock(&blk1)
	require.NoError(t, err)

	opts := idb.ApplicationQuery{ApplicationID: uint64(appid)}

	rowsCh, round := db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	row, ok := <-rowsCh
	require.True(t, ok)
	require.NoError(t, row.Error)
	require.NotNil(t, row.Application.CreatedAtRound)
	require.Equal(t, uint64(currentRound), *row.Application.CreatedAtRound)

	// ---- ROUND 2: create 8 boxes for appid == 1  ---- //
	currentRound = basics.Round(2)

	boxNames := []string{
		"a great box",
		"another great box",
		"not so great box",
		"disappointing box",
		"don't box me in this way",
		"I will be assimilated",
		"I'm destined for deletion",
		"box #8",
	}

	expectedAppBoxes := map[basics.AppIndex]map[string]string{}

	expectedAppBoxes[appid] = map[string]string{}
	newBoxValue := "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	//boxTxns := make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range boxNames {
	//	expectedAppBoxes[appid][apps.MakeBoxKey(uint64(appid), boxName)] = newBoxValue
	//
	//	args := []string{"create", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//}

	for _, boxName := range boxNames {
		expectedAppBoxes[appid][apps.MakeBoxKey(uint64(appid), boxName)] = newBoxValue
	}

	vb2, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDeleteR2.vb")
	require.NoError(t, err)
	blk2 := ledgercore.MakeValidatedBlock(vb2.Blk, vb2.Delta)
	err = db.AddBlock(&blk2)
	require.NoError(t, err)
	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	comparator(t, db, expectedAppBoxes, nil, true)

	// ---- ROUND 3: populate the boxes appropriately  ---- //
	currentRound = basics.Round(3)

	appBoxesToSet := map[string]string{
		"a great box":               "it's a wonderful box",
		"another great box":         "I'm wonderful too",
		"not so great box":          "bummer",
		"disappointing box":         "RUG PULL!!!!",
		"don't box me in this way":  "non box-conforming",
		"I will be assimilated":     "THE BORG",
		"I'm destined for deletion": "I'm still alive!!!",
		"box #8":                    "eight is beautiful",
	}

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//expectedAppBoxes[appid] = make(map[string]string)
	//for boxName, valPrefix := range appBoxesToSet {
	//	args := []string{"set", boxName, valPrefix}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(appid), boxName)
	//	expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	//}

	expectedAppBoxes[appid] = make(map[string]string)
	for boxName, valPrefix := range appBoxesToSet {
		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}

	vb3, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDeleteR3.vb")
	require.NoError(t, err)
	blk3 := ledgercore.MakeValidatedBlock(vb3.Blk, vb3.Delta)
	err = db.AddBlock(&blk3)
	require.NoError(t, err)
	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	comparator(t, db, expectedAppBoxes, nil, true)

	// ---- ROUND 4: delete the unhappy boxes  ---- //
	currentRound = basics.Round(4)

	appBoxesToDelete := []string{
		"not so great box",
		"disappointing box",
		"I'm destined for deletion",
	}

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range appBoxesToDelete {
	//	args := []string{"delete", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(appid), boxName)
	//	delete(expectedAppBoxes[appid], key)
	//}

	for _, boxName := range appBoxesToDelete {
		key := apps.MakeBoxKey(uint64(appid), boxName)
		delete(expectedAppBoxes[appid], key)
	}

	vb4, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDeleteR4.vb")
	require.NoError(t, err)
	blk4 := ledgercore.MakeValidatedBlock(vb4.Blk, vb4.Delta)
	err = db.AddBlock(&blk4)
	require.NoError(t, err)
	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	deletedBoxes := make(map[basics.AppIndex]map[string]bool)
	deletedBoxes[appid] = make(map[string]bool)
	for _, deletedBox := range appBoxesToDelete {
		deletedBoxes[appid][deletedBox] = true
	}
	comparator(t, db, expectedAppBoxes, deletedBoxes, true)

	// ---- ROUND 5: create 3 new boxes, overwriting one of the former boxes  ---- //
	currentRound = basics.Round(5)

	appBoxesToCreate := []string{
		"fantabulous",
		"disappointing box", // overwriting here
		"AVM is the new EVM",
	}
	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range appBoxesToCreate {
	//	args := []string{"create", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(appid), boxName)
	//	expectedAppBoxes[appid][key] = newBoxValue
	//}
	for _, boxName := range appBoxesToCreate {
		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid][key] = newBoxValue
	}

	vb5, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDeleteR5.vb")
	require.NoError(t, err)
	blk5 := ledgercore.MakeValidatedBlock(vb5.Blk, vb5.Delta)
	err = db.AddBlock(&blk5)
	require.NoError(t, err)
	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	comparator(t, db, expectedAppBoxes, nil, true)

	// ---- ROUND 6: populate the 3 new boxes  ---- //
	currentRound = basics.Round(6)

	appBoxesToSet = map[string]string{
		"fantabulous":        "Italian food's the best!", // max char's
		"disappointing box":  "you made it!",
		"AVM is the new EVM": "yes we can!",
	}
	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for boxName, valPrefix := range appBoxesToSet {
	//	args := []string{"set", boxName, valPrefix}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(appid), boxName)
	//	expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	//}
	for boxName, valPrefix := range appBoxesToSet {
		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}

	vb6, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/BoxCreateMutateDeleteR6.vb")
	require.NoError(t, err)
	blk6 := ledgercore.MakeValidatedBlock(vb6.Blk, vb6.Delta)
	err = db.AddBlock(&blk6)
	require.NoError(t, err)
	_, round = db.Applications(context.Background(), opts)
	require.Equal(t, uint64(currentRound), round)

	comparator(t, db, expectedAppBoxes, nil, true)

	fmt.Printf("runBoxCreateMutateDelete total time: %s\n", time.Since(start))
}

// generateRandomBoxes generates a random slice of box keys and values for an app using future consensus params for guidance.
// NOTE: no attempt is made to adhere to the constraints BytesPerBoxReference etc.
func generateRandomBoxes(t *testing.T, appIdx basics.AppIndex, maxBoxes int) map[string]string {
	future := config.Consensus[protocol.ConsensusFuture]

	numBoxes := rand.Intn(maxBoxes + 1)
	boxes := make(map[string]string)
	for i := 0; i < numBoxes; i++ {
		nameLen := rand.Intn(future.MaxAppKeyLen + 1)
		size := rand.Intn(int(future.MaxBoxSize) + 1)

		nameBytes := make([]byte, nameLen)
		_, err := rand.Read(nameBytes)
		require.NoError(t, err)
		key := apps.MakeBoxKey(uint64(appIdx), string(nameBytes))

		require.Positive(t, len(key))

		valueBytes := make([]byte, size)
		_, err = rand.Read(valueBytes)
		require.NoError(t, err)

		boxes[key] = string(valueBytes)
	}
	return boxes
}

func createRandomBoxesWithDelta(t *testing.T, numApps, maxBoxes int) (map[basics.AppIndex]map[string]string, ledgercore.StateDelta) {
	appBoxes := make(map[basics.AppIndex]map[string]string)

	delta := ledgercore.StateDelta{
		KvMods: map[string]ledgercore.KvValueDelta{},
		Accts:  ledgercore.MakeAccountDeltas(numApps),
	}

	for i := 0; i < numApps; i++ {
		appIndex := basics.AppIndex(rand.Int63())
		boxes := generateRandomBoxes(t, appIndex, maxBoxes)
		appBoxes[appIndex] = boxes

		for key, value := range boxes {
			embeddedAppIdx, _, err := apps.SplitBoxKey(key)
			require.NoError(t, err)
			require.Equal(t, uint64(appIndex), embeddedAppIdx)

			val := string([]byte(value)[:])
			delta.KvMods[key] = ledgercore.KvValueDelta{Data: []byte(val)}
		}

	}
	return appBoxes, delta
}

func randomMutateSomeBoxesWithDelta(t *testing.T, appBoxes map[basics.AppIndex]map[string]string) ledgercore.StateDelta {
	var delta ledgercore.StateDelta
	delta.KvMods = make(map[string]ledgercore.KvValueDelta)

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
			delta.KvMods[key] = ledgercore.KvValueDelta{Data: []byte(val)}
		}
	}

	return delta
}

func deleteSomeBoxesWithDelta(t *testing.T, appBoxes map[basics.AppIndex]map[string]string) (map[basics.AppIndex]map[string]bool, ledgercore.StateDelta) {
	deletedBoxes := make(map[basics.AppIndex]map[string]bool, len(appBoxes))

	var delta ledgercore.StateDelta
	delta.KvMods = make(map[string]ledgercore.KvValueDelta)

	for appIndex, boxes := range appBoxes {
		deletedBoxes[appIndex] = map[string]bool{}
		for key := range boxes {
			if rand.Intn(2) == 0 {
				continue
			}
			deletedBoxes[appIndex][key] = true
			delta.KvMods[key] = ledgercore.KvValueDelta{Data: nil}
		}
	}

	return deletedBoxes, delta
}

func addAppBoxesBlock(t *testing.T, db *IndexerDb, delta ledgercore.StateDelta) {
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
}

// Integration test for validating that box evolution is ingested as expected across rounds using database to compare
func TestBoxCreateMutateDeleteAgainstDB(t *testing.T) {
	runBoxCreateMutateDelete(t, compareAppBoxesAgainstDB)
}

// Write random apps with random box names and values, then read them from indexer DB and compare.
// NOTE: this does not populate TotalBoxes nor TotalBoxBytes deep under StateDeltas.Accts and therefore
// no query is taken to compare the summary box information in `account.account_data`
// Mutate some boxes and repeat the comparison.
// Delete some boxes and repeat the comparison.
func TestRandomWriteReadBoxes(t *testing.T) {
	start := time.Now()

	db, shutdownFunc := setupIdb(t, test.MakeGenesisV2())
	defer shutdownFunc()

	appBoxes, delta := createRandomBoxesWithDelta(t, 10, 2500)
	addAppBoxesBlock(t, db, delta)
	compareAppBoxesAgainstDB(t, db, appBoxes, nil, false)

	delta = randomMutateSomeBoxesWithDelta(t, appBoxes)
	addAppBoxesBlock(t, db, delta)
	compareAppBoxesAgainstDB(t, db, appBoxes, nil, false)

	deletedBoxes, delta := deleteSomeBoxesWithDelta(t, appBoxes)
	addAppBoxesBlock(t, db, delta)
	compareAppBoxesAgainstDB(t, db, appBoxes, deletedBoxes, false)

	fmt.Printf("TestRandomWriteReadBoxes total time: %s\n", time.Since(start))
}
