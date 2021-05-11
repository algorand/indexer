package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
