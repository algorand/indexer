package postgres

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/data/transactions/logic"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/protocol"

	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util/test"
)

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

func createBoxesWithDelta(t *testing.T, numApps, maxBoxes int) (map[basics.AppIndex]map[string]string, ledgercore.StateDelta) {
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

	numQueries := 0
	sumOfBoxes := 0
	sumOfBoxBytes := 0

	appBoxSQL := `SELECT app, name, value FROM app_box WHERE app = $1 AND name = $2`

	caseNum := 1
	var totalBoxes, totalBoxBytes int
	for appIdx, boxes := range appBoxes {
		totalBoxes = 0
		totalBoxBytes = 0

		// compare expected against db contents one box at a time
		for key, expectedValue := range boxes {
			msg := fmt.Sprintf("caseNum=%d, appIdx=%d, key=%#v", caseNum, appIdx, key)
			expectedAppIdx, boxName, err := logic.SplitBoxKey(key)
			require.NoError(t, err, msg)
			require.Equal(t, appIdx, expectedAppIdx, msg)

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
				require.Equal(t, expectedAppIdx, app, msg)
				require.Equal(t, boxName, string(name), msg)
				require.Equal(t, expectedValue, string(value), msg)

				totalBoxes++
				totalBoxBytes += len(boxName) + len(expectedValue)
			} else {
				require.ErrorContains(t, err, "no rows in result set", msg)
			}
		}
		sumOfBoxes += totalBoxes
		sumOfBoxBytes += totalBoxBytes
		caseNum++
	}

	fmt.Printf("CompareAppBoxesAgainstDB succeeded with %d queries, %d boxes and %d boxBytes\n", numQueries, sumOfBoxes, sumOfBoxBytes)
}

// Write random apps with random box names and values, then read them from indexer DB and compare.
// NOTE: this does not populate TotalBoxes nor TotalBoxBytes deep under StateDeltas.Accts and therefore
// no query is taken to compare the summary box information in `account.account_data`
// Mutate some boxes and repeat the comparison.
// Delete some boxes and repeat the comparison.
func TestWriteReadBoxes(t *testing.T) {
	start := time.Now()

	db, shutdownFunc, _, ld := setupIdb(t, test.MakeGenesis())
	defer shutdownFunc()
	defer ld.Close()

	appBoxes, delta := createBoxesWithDelta(t, 10, 2500)
	addAppBoxesBlock(t, db, delta)
	CompareAppBoxesAgainstDB(t, db, appBoxes)

	delta = mutateSomeBoxesWithDelta(t, appBoxes)
	addAppBoxesBlock(t, db, delta)
	CompareAppBoxesAgainstDB(t, db, appBoxes)

	deletedBoxes, delta := deleteSomeBoxesWithDelta(t, appBoxes)
	addAppBoxesBlock(t, db, delta)
	CompareAppBoxesAgainstDB(t, db, appBoxes, deletedBoxes)

	fmt.Printf("TestWriteReadBoxes total time: %s\n", time.Since(start))
}
