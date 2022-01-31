package future

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var expectations = []expectation{
	// we should NOT see the file third_party/go-algorand/ledger/ledgercore/accountdata.go
	// and CERTAINLY NOT struct AccountBaseData
	// and ABSLUTELY NOT TotalAssets
	{
		"../third_party/go-algorand/ledger/ledgercore/accountdata.go",
		missing{shouldMiss: true, msg: "\n!!!path ledger/ledgercore/accountdata.go detected ==> unlimited assets has arrived\n"},
		"AccountBaseData",
		missing{shouldMiss: true, msg: "\n!!!struct AccountBaseData is detected ==> unlimited assets has arrived\n"},
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
