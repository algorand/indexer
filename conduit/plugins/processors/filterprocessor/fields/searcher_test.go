package fields

import (
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/indexer/conduit/plugins/processors/filterprocessor/expression"
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

	address1 := sdk.Address{1}
	address2 := sdk.Address{2}

	var expressionType expression.FilterType = expression.EqualToFilter
	tag := "sgnr"
	exp, err := expression.MakeExpression(expressionType, address1.String(), "")
	assert.NoError(t, err)
	searcher, err := MakeFieldSearcher(exp, expressionType, tag)
	assert.NoError(t, err)

	result, err := searcher.search(
		&sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				AuthAddr: address1,
			},
		},
	)

	assert.NoError(t, err)
	assert.True(t, result)

	result, err = searcher.search(
		&sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				AuthAddr: address2,
			},
		},
	)

	assert.NoError(t, err)
	assert.False(t, result)
}

// TestMakeFieldSearcher tests making a field searcher is valid
func TestMakeFieldSearcher(t *testing.T) {
	var expressionType expression.FilterType = expression.EqualToFilter
	tag := "sgnr"
	sampleExpressionStr := "sample"
	exp, err := expression.MakeExpression(expressionType, sampleExpressionStr, "")
	assert.NoError(t, err)
	searcher, err := MakeFieldSearcher(exp, expressionType, tag)
	assert.NoError(t, err)
	assert.NotNil(t, searcher)
	assert.Equal(t, searcher.Tag, tag)

	searcher, err = MakeFieldSearcher(exp, "made-up-expression-type", sampleExpressionStr)
	assert.Error(t, err)
	assert.Nil(t, searcher)

}

// TestCheckTagExistsAndHasCorrectFunction tests that the check tag exists and function relation works
func TestCheckTagExistsAndHasCorrectFunction(t *testing.T) {
	// check that something that doesn't exist throws an error
	err := checkTagAndExpressionExist(expression.EqualToFilter, "SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.LoreumIpsum.SDF")
	assert.ErrorContains(t, err, "does not exist in transactions")

	err = checkTagAndExpressionExist(expression.EqualToFilter, "LoreumIpsum")
	assert.ErrorContains(t, err, "does not exist in transactions")

	// a made up expression type should throw an error
	err = checkTagAndExpressionExist("made-up-expression-type", "sgnr")
	assert.ErrorContains(t, err, "is not supported")

	err = checkTagAndExpressionExist(expression.EqualToFilter, "sgnr")
	assert.NoError(t, err)
}
