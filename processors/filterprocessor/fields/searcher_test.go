package fields

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/indexer/processors/filterprocessor/expression"
)

// TestInternalSearch tests the internal search functionality
func TestInternalSearch(t *testing.T) {

	defer func() {
		// Since this function should only be called after validation is performed,
		// this recovery function lets us recover is the schema changes in anyway in the future
		if r := recover(); r != nil {
			assert.True(t, false)
		}
	}()

	address1 := basics.Address{1}
	address2 := basics.Address{2}

	var expressionType expression.FilterType = "exact"
	tag := "SignedTxnWithAD.SignedTxn.AuthAddr"
	exp, err := expression.MakeExpression(expressionType, address1.String())
	assert.NoError(t, err)
	searcher, err := MakeFieldSearcher(exp, expressionType, tag)

	result := searcher.search(
		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					AuthAddr: address1,
				},
			},
		},
	)

	assert.True(t, result)

	result = searcher.search(
		transactions.SignedTxnInBlock{
			SignedTxnWithAD: transactions.SignedTxnWithAD{
				SignedTxn: transactions.SignedTxn{
					AuthAddr: address2,
				},
			},
		},
	)

	assert.False(t, result)
}

// TestMakeFieldSearcher tests making a field searcher is valid
func TestMakeFieldSearcher(t *testing.T) {
	var expressionType expression.FilterType = "exact"
	tag := "SignedTxnWithAD.SignedTxn.AuthAddr"
	sampleExpressionStr := "sample"
	exp, err := expression.MakeExpression(expressionType, sampleExpressionStr)
	assert.NoError(t, err)
	searcher, err := MakeFieldSearcher(exp, expressionType, tag)
	assert.NoError(t, err)
	assert.NotNil(t, searcher)
	assert.Equal(t, searcher.Tag, tag)
	assert.Equal(t, searcher.MethodToCall, expression.TypeToFunctionMap[expressionType])

	searcher, err = MakeFieldSearcher(exp, "made-up-expression-type", sampleExpressionStr)
	assert.Error(t, err)
	assert.Nil(t, searcher)

}

// TestCheckTagExistsAndHasCorrectFunction tests that the check tag exists and function relation works
func TestCheckTagExistsAndHasCorrectFunction(t *testing.T) {
	// check that something that doesn't exist throws an error
	err := checkTagExistsAndHasCorrectFunction("exact", "SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.LoreumIpsum.SDF")
	assert.ErrorContains(t, err, "does not exist in transactions")

	err = checkTagExistsAndHasCorrectFunction("exact", "LoreumIpsum")
	assert.ErrorContains(t, err, "does not exist in transactions")

	// Fee does not have a "String" Function so we cant use exact with it.
	err = checkTagExistsAndHasCorrectFunction("exact", "SignedTxnWithAD.SignedTxn.Txn.Header.Fee")
	assert.ErrorContains(t, err, "does not contain the needed method")

	// a made up expression type should throw an error
	err = checkTagExistsAndHasCorrectFunction("made-up-expression-type", "SignedTxnWithAD.SignedTxn.AuthAddr")
	assert.ErrorContains(t, err, "is not supported")

	err = checkTagExistsAndHasCorrectFunction("exact", "SignedTxnWithAD.SignedTxn.AuthAddr")
	assert.NoError(t, err)
}
