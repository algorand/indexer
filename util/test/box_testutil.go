package test

import (
	"fmt"
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions/logic"
	"github.com/algorand/go-algorand/ledger/ledgercore"

	indxLedger "github.com/algorand/indexer/processor/eval"
	"github.com/stretchr/testify/require"
)

func getNameAndAccountPointer(t *testing.T, value *string, fullKey string, accts map[basics.Address]*ledgercore.AccountData) (basics.Address, string, *ledgercore.AccountData) {
	require.NotNil(t, value, "cannot handle a nil value for box stats modification")
	appIdx, name, err := logic.SplitBoxKey(fullKey)
	account := appIdx.Address()
	require.NoError(t, err)
	acctData, ok := accts[account]
	if !ok {
		acctData = &ledgercore.AccountData{
			AccountBaseData: ledgercore.AccountBaseData{},
		}
		accts[account] = acctData
	}
	return account, name, acctData
}

func addBoxInfoToStats(t *testing.T, fullKey string, value *string,
	accts map[basics.Address]*ledgercore.AccountData, boxTotals map[basics.Address]basics.AccountData) {
	addr, name, acctData := getNameAndAccountPointer(t, value, fullKey, accts)

	acctData.TotalBoxes++
	acctData.TotalBoxBytes += uint64(len(name) + len(*value))

	boxTotals[addr] = basics.AccountData{
		TotalBoxes:    acctData.TotalBoxes,
		TotalBoxBytes: acctData.TotalBoxBytes,
	}
}

func subtractBoxInfoToStats(t *testing.T, fullKey string, value *string,
	accts map[basics.Address]*ledgercore.AccountData, boxTotals map[basics.Address]basics.AccountData) {
	addr, name, acctData := getNameAndAccountPointer(t, value, fullKey, accts)

	prevBoxBytes := uint64(len(name) + len(*value))
	require.GreaterOrEqual(t, acctData.TotalBoxes, uint64(0))
	require.GreaterOrEqual(t, acctData.TotalBoxBytes, prevBoxBytes)

	acctData.TotalBoxes--
	acctData.TotalBoxBytes -= prevBoxBytes

	boxTotals[addr] = basics.AccountData{
		TotalBoxes:    acctData.TotalBoxes,
		TotalBoxBytes: acctData.TotalBoxBytes,
	}
}

// BuildAccountDeltasFromKvsAndMods simulates keeping track of the evolution of the box statistics
func BuildAccountDeltasFromKvsAndMods(t *testing.T, kvOriginals, kvMods map[string]*string) (
	ledgercore.StateDelta, map[string]*string, map[basics.Address]basics.AccountData) {
	kvUpdated := map[string]*string{}
	boxTotals := map[basics.Address]basics.AccountData{}
	accts := map[basics.Address]*ledgercore.AccountData{}
	/*
		1. fill the accts and kvUpdated using kvOriginals
		2. for each (fullKey, value) in kvMod:
			* (A) if the key is not present in kvOriginals just add the info as in #1
			* (B) else (fullKey present):
			    * (i)  if the value is nil
					==> remove the box info from the stats and kvUpdated with assertions
				* (ii) else (value is NOT nil):
					==> reset kvUpdated and assert that the box hasn't changed shapes
	*/

	/* 1. */
	for fullKey, value := range kvOriginals {
		addBoxInfoToStats(t, fullKey, value, accts, boxTotals)
		kvUpdated[fullKey] = value
	}

	/* 2. */
	for fullKey, value := range kvMods {
		prevValue, ok := kvOriginals[fullKey]
		if !ok {
			/* 2A. */
			addBoxInfoToStats(t, fullKey, value, accts, boxTotals)
			kvUpdated[fullKey] = value
			continue
		}
		/* 2B. */
		if value == nil {
			/* 2Bi. */
			subtractBoxInfoToStats(t, fullKey, prevValue, accts, boxTotals)
			delete(kvUpdated, fullKey)
			continue
		}
		/* 2Bii. */
		require.Equal(t, len(*prevValue), len(*value))
		require.Contains(t, kvUpdated, fullKey)
		kvUpdated[fullKey] = value
	}

	var delta ledgercore.StateDelta
	for acct, acctData := range accts {
		delta.Accts.Upsert(acct, *acctData)
	}
	return delta, kvUpdated, boxTotals
}

// CompareAppBoxesAgainstLedger uses LedgerForEvaluator to assert that provided app boxes can be retrieved as expected
func CompareAppBoxesAgainstLedger(t *testing.T, ld indxLedger.LedgerForEvaluator, round basics.Round,
	appBoxes map[basics.AppIndex]map[string]string, extras ...map[basics.AppIndex]map[string]bool) {
	require.LessOrEqual(t, len(extras), 1)
	var deletedBoxes map[basics.AppIndex]map[string]bool
	if len(extras) == 1 {
		deletedBoxes = extras[0]
	}

	caseNum := 1
	for appIdx, boxes := range appBoxes {
		for key, expectedValue := range boxes {
			msg := fmt.Sprintf("caseNum=%d, appIdx=%d, key=%#v", caseNum, appIdx, key)
			expectedAppIdx, _, err := logic.SplitBoxKey(key)
			require.NoError(t, err, msg)
			require.Equal(t, appIdx, expectedAppIdx, msg)

			boxDeleted := false
			if deletedBoxes != nil {
				if _, ok := deletedBoxes[appIdx][key]; ok {
					boxDeleted = true
				}
			}

			value, err := ld.LookupKv(round, key)
			require.NoError(t, err, msg)
			if !boxDeleted {
				require.Equal(t, expectedValue, *value, msg)
			} else {
				require.Nil(t, value, msg)
			}
		}
		caseNum++
	}
}
