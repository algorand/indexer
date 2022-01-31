package future

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var expectations = []expectation{
	{
		// we should NOT see the file third_party/go-algorand/ledger/ledgercore/accountdata.go:
		"../third_party/go-algorand/ledger/ledgercore/accountdata.go",
		missing{shouldMiss: true, msg: "\n!!!path ledger/ledgercore/accountdata.go detected ==> unlimited assets has arrived\n"},

		// AND struct AccountBaseData should not be available either:
		"AccountBaseData",
		missing{shouldMiss: true, msg: "\n!!!struct AccountBaseData is detected ==> unlimited assets has arrived\n"},

		// AND finally, AccountBaseData.TotalAssets also should not exist:
		[]string{"TotalAssets"},
		missing{shouldMiss: true, msg: "\n!!!field TotalAssets is detected ==> unlimited assets has arrived\n"},
	},
}

func TestTheFuture(t *testing.T) {
	problems := getProblematicExpectations(t, expectations)

	for i, prob := range problems {
		for _, err := range prob {
			assert.NoError(t, err, "error in expectation #%d (%s):\n%v", i+1, expectations[i].File, err)
		}
	}
}
