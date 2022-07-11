package test

import (
	"fmt"
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions/logic"

	indxLedger "github.com/algorand/indexer/processor/eval"
	"github.com/stretchr/testify/require"
)

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
