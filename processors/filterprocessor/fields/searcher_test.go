package fields

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCheckTagExistsAndHasCorrectFunction tests that the check tag exists and function relation works
func TestCheckTagExistsAndHasCorrectFunction(t *testing.T) {
	// check that something that doesn't exist throws an error
	err := checkTagExistsAndHasCorrectFunction("const", "SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.LoreumIpsum.SDF")
	assert.ErrorContains(t, err, "does not exist in transactions")

	err = checkTagExistsAndHasCorrectFunction("const", "LoreumIpsum")
	assert.ErrorContains(t, err, "does not exist in transactions")

	// Fee does not have a "String" Function so we cant use const with it.
	err = checkTagExistsAndHasCorrectFunction("const", "SignedTxnWithAD.SignedTxn.Txn.Header.Fee")
	assert.ErrorContains(t, err, "does not contain the needed method")
}
