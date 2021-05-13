package generator

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeightedSelectionInternal(t *testing.T) {
	weights := []float32{0.10, 0.30, 0.60}
	options := []interface{}{"10", "30", "60"}

	testcases := []struct{
		selectionNum float32
		expected interface{}
	} {
		{
			selectionNum: 0.0,
			expected: options[0],
		},
		{
			selectionNum: 0.10,
			expected: options[0],
		},
		{
			selectionNum: 0.101,
			expected: options[1],
		},
		{
			selectionNum: 0.4,
			expected: options[1],
		},
		{
			selectionNum: 0.401,
			expected: options[2],
		},
		{
			selectionNum: 1.0,
			expected: options[2],
		},
	}

	for _, test := range testcases {
		name := fmt.Sprintf("selectionNum %f - expected %v", test.selectionNum, test.expected)
		t.Run(name, func(t *testing.T) {
			actual, err := weightedSelectionInternal(test.selectionNum, weights, options)
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestWeightedSelection(t *testing.T) {
	weights := []float32{0.10, 0.30, 0.60}
	options := []interface{}{"10", "30", "60"}
	selections := make(map[interface{}]int)

	for i:=0; i < 100; i++ {
		selected, err := weightedSelection(weights, options)
		require.NoError(t, err)
		selections[selected] += 1
	}

	assert.Less(t, selections[options[0]], selections[options[1]])
	assert.Less(t, selections[options[1]], selections[options[2]])
}

func TestWeightedSelectionOutOfRange(t *testing.T) {
	weights := []float32{0.1}
	options := []interface{}{"1"}

	for i:=0; i < 10000; i++ {
		_, err := weightedSelection(weights, options)
		if err != nil {
			require.Errorf(t, err, outOfRangeError.Error())
			return
		}
	}
	assert.Fail(t, "Expected an out of range error by this point.")
}
