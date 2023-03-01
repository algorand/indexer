package fields

import (
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/conduit/plugins/processors/filterprocessor/expression"
)

// TestInternalSearch tests the internal search functionality
func TestInternalSearch(t *testing.T) {
	address1 := sdk.Address{1}
	address2 := sdk.Address{2}

	var expressionType expression.Type = expression.EqualTo
	tag := "sgnr"
	exp, err := expression.MakeExpression(expressionType, address1.String(), "")
	require.NoError(t, err)
	searcher, err := MakeFieldSearcher(exp, expressionType, tag, false)
	require.NoError(t, err)

	result, err := searcher.search(
		&sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				AuthAddr: address1,
			},
		},
	)

	require.NoError(t, err)
	assert.True(t, result)

	result, err = searcher.search(
		&sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				AuthAddr: address2,
			},
		},
	)

	require.NoError(t, err)
	assert.False(t, result)
}

// TestMakeFieldSearcher tests making a field searcher is valid
func TestMakeFieldSearcher(t *testing.T) {
	expressionType := expression.EqualTo
	tag := "sgnr"
	sampleExpressionStr := "sample"
	exp, err := expression.MakeExpression(expressionType, sampleExpressionStr, "")
	require.NoError(t, err)
	searcher, err := MakeFieldSearcher(exp, expressionType, tag, false)
	require.NoError(t, err)
	require.NotNil(t, searcher)
	assert.Equal(t, searcher.Tag, tag)

	searcher, err = MakeFieldSearcher(exp, "made-up-expression-type", sampleExpressionStr, false)
	require.Error(t, err)
}

// TestCheckTagExistsAndHasCorrectFunction tests that the check tag exists and function relation works
func TestCheckTagExistsAndHasCorrectFunction(t *testing.T) {
	// check that something that doesn't exist throws an error
	err := checkTagAndExpressionExist(expression.EqualTo, "SignedTxnWithAD.SignedTxn.Txn.PaymentTxnFields.LoreumIpsum.SDF")
	assert.ErrorContains(t, err, "does not exist in transactions")

	err = checkTagAndExpressionExist(expression.EqualTo, "LoreumIpsum")
	assert.ErrorContains(t, err, "does not exist in transactions")

	// a made up expression type should throw an error
	err = checkTagAndExpressionExist("made-up-expression-type", "sgnr")
	assert.ErrorContains(t, err, "is not supported")

	err = checkTagAndExpressionExist(expression.EqualTo, "sgnr")
	assert.NoError(t, err)
}

func TestInnerTxnSearch(t *testing.T) {
	var addr sdk.Address
	addr[0] = 0x1
	exp, err := expression.MakeExpression(expression.EqualTo, addr.String(), "")
	require.NoError(t, err)

	txnWithInnerMatch := sdk.SignedTxnWithAD{
		ApplyData: sdk.ApplyData{
			EvalDelta: sdk.EvalDelta{
				InnerTxns: []sdk.SignedTxnWithAD{
					{
						SignedTxn: sdk.SignedTxn{
							Txn: sdk.Transaction{
								Header: sdk.Header{
									Sender: addr,
								},
							},
						},
					},
				},
			},
		},
	}

	{
		// searchInner: false
		searcher, err := MakeFieldSearcher(exp, expression.EqualTo, "txn.snd", false)
		require.NoError(t, err)

		// Provide the matching inner transaction.
		// It matches with searchInner: false.
		matches, err := searcher.search(&txnWithInnerMatch.EvalDelta.InnerTxns[0])

		require.NoError(t, err)
		assert.Equal(t, matches, true)

		// Provide the root txn, no match at the root.
		// It should have no match with searchInner: false.
		matches, err = searcher.search(&txnWithInnerMatch)

		// No match on inner txn
		require.NoError(t, err)
		assert.Equal(t, matches, false)
	}

	{
		// searchInner: true
		searcher, err := MakeFieldSearcher(exp, expression.EqualTo, "txn.snd", true)
		require.NoError(t, err)

		// Provide the matching inner transaction.
		// It matches with searchInner: false.
		matches, err := searcher.search(&txnWithInnerMatch.EvalDelta.InnerTxns[0])

		require.NoError(t, err)
		assert.Equal(t, matches, true)

		// Provide the root txn, no match at the root.
		// It should have no match with searchInner: false.
		matches, err = searcher.search(&txnWithInnerMatch)

		// Match on inner txn
		require.NoError(t, err)
		assert.Equal(t, matches, true)
	}
}
