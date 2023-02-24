package expression

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpression(t *testing.T) {
	// used at the end to make sure we covered all filter types
	allCovered := make(map[ExpressionType]interface{})

	testcases := []struct {
		name            string
		expressionType  ExpressionType
		searchString    string
		targetInterface interface{}
		input           interface{}
		match           bool
		makeErr         string
		searchErr       string
	}{
		// wrong type errors
		{
			name:            "error_Invalid-type",
			expressionType:  GreaterThan,
			targetInterface: int8(1),
			makeErr:         "unknown expression type: int",
		},
		// wrong type errors
		{
			name:            "error_Regex-Wrong-type",
			expressionType:  GreaterThan,
			searchString:    "5",
			targetInterface: "asdf",
			makeErr:         "target type (string) does not support greater-than filters",
		},
		{
			name:            "error_Uint-bad-filter-type",
			expressionType:  Regex,
			searchString:    "not a number",
			targetInterface: uint64(0),
			makeErr:         "target type (numeric) does not support regex filters",
		},
		{
			name:            "error_Int-bad-filter-type",
			expressionType:  Regex,
			searchString:    "not a number",
			targetInterface: int64(0),
			makeErr:         "target type (numeric) does not support regex filters",
		},
		// search string errors
		{
			name:            "error_Uint-not-a-number",
			expressionType:  GreaterThan,
			searchString:    "not a number",
			targetInterface: uint64(0),
			makeErr:         "search string \"not a number\" is not a valid uint64:",
		},
		{
			name:            "error_Int-not-a-number",
			expressionType:  GreaterThanEqual,
			searchString:    "not a number",
			targetInterface: int64(0),
			makeErr:         "search string \"not a number\" is not a valid int64:",
		},
		{
			name:            "error_Uint-signed-number",
			expressionType:  GreaterThan,
			searchString:    "-1",
			targetInterface: uint64(0),
			makeErr:         "search string \"-1\" is not a valid uint64:",
		},
		{
			name:            "error_Int-overflow",
			expressionType:  GreaterThanEqual,
			searchString:    fmt.Sprintf("%d", uint64(math.MaxInt)+1),
			targetInterface: int64(0),
			makeErr:         fmt.Sprintf("search string \"%d\" is not a valid int64:", uint64(math.MaxInt)+1),
		},
		{
			name:            "error_Regex-bad-pattern",
			expressionType:  Regex,
			searchString:    "*",
			targetInterface: "asdf",
			makeErr:         "error parsing regexp:",
		},
		// input errors
		{
			name:            "error_Regex-match",
			expressionType:  Regex,
			searchString:    ".*thing",
			targetInterface: "asdf",
			input:           5,
			match:           true,
			searchErr:       "unexpected regex search input type (int)",
		},
		{
			name:            "error_Uint-bad-number",
			expressionType:  GreaterThan,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           "El Toro Loco",
			searchErr:       "unexpected numeric search input \"El Toro Loco\"",
		},
		{
			name:            "error_Int-bad-number",
			expressionType:  GreaterThan,
			searchString:    "10",
			targetInterface: int64(0),
			input:           "Bad Company",
			searchErr:       "unexpected numeric search input \"Bad Company\"",
		},
		{
			name:            "error_Uint-bad-number-2",
			expressionType:  GreaterThan,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           int64(10), // wrong sign
			searchErr:       "unexpected numeric search input \"10\"",
		},
		{
			name:            "error_Int-bad-number-2",
			expressionType:  GreaterThan,
			searchString:    "10",
			targetInterface: int64(0),
			input:           uint64(10), // wrong sign
			searchErr:       "unexpected numeric search input \"10\"",
		},
		// match / no-match - Regex
		{
			name:            "Regex-match",
			expressionType:  Regex,
			searchString:    ".*Dragonoid",
			targetInterface: "asdf",
			input:           "Bakugan Dragonoid",
			match:           true,
		},
		{
			name:            "Regex-no-match",
			expressionType:  Regex,
			searchString:    "Mohawk Warrior",
			targetInterface: "asdf",
			input:           "Grave Digger",
			match:           false,
		},
		// match / no-match - EqualTo (numeric and string)
		{
			name:            "EqualTo-string-match",
			expressionType:  EqualTo,
			searchString:    ".*odon",
			targetInterface: "asdf",
			input:           "Megalodon",
			match:           false,
		},
		{
			name:            "EqualTo-string-match-special",
			expressionType:  EqualTo,
			searchString:    ".*odon",
			targetInterface: "asdf",
			input:           ".*odon",
			match:           true,
		},
		{
			name:            "EqualTo-string-no-match",
			expressionType:  EqualTo,
			searchString:    "Monster Mutt",
			targetInterface: "asdf",
			input:           "Max-D",
			match:           false,
		},
		{
			name:            "EqualTo-int-match",
			expressionType:  EqualTo,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(10),
			match:           true,
		},
		{
			name:            "EqualTo-int-no-match",
			expressionType:  EqualTo,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(11),
			match:           false,
		},
		{
			name:            "EqualTo-uint-match",
			expressionType:  EqualTo,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(10),
			match:           true,
		},
		{
			name:            "EqualTo-uint-no-match",
			expressionType:  EqualTo,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(11),
			match:           false,
		},
		// match / no-match - GreaterThan
		{
			name:            "GreaterThan-int-match",
			expressionType:  GreaterThan,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(11),
			match:           true,
		},
		{
			name:            "GreaterThan-int-no-match",
			expressionType:  GreaterThan,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(10),
			match:           false,
		},
		{
			name:            "GreaterThan-uint-match",
			expressionType:  GreaterThan,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(11),
			match:           true,
		},
		{
			name:            "GreaterThan-uint-no-match",
			expressionType:  GreaterThan,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(10),
			match:           false,
		},
		// match / no-match - GreaterThanEqual
		{
			name:            "GreaterThanEqual-int-match",
			expressionType:  GreaterThanEqual,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(10),
			match:           true,
		},
		{
			name:            "GreaterThanEqual-int-no-match",
			expressionType:  GreaterThanEqual,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(9),
			match:           false,
		},
		{
			name:            "GreaterThanEqual-uint-match",
			expressionType:  GreaterThanEqual,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(10),
			match:           true,
		},
		{
			name:            "GreaterThanEqual-uint-no-match",
			expressionType:  GreaterThanEqual,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(9),
			match:           false,
		},
		// match / no-match - LessThan
		{
			name:            "LessThan-int-match",
			expressionType:  LessThan,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(9),
			match:           true,
		},
		{
			name:            "LessThan-int-no-match",
			expressionType:  LessThan,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(10),
			match:           false,
		},
		{
			name:            "LessThan-uint-match",
			expressionType:  LessThan,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(9),
			match:           true,
		},
		{
			name:            "LessThan-uint-no-match",
			expressionType:  LessThan,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(10),
			match:           false,
		},
		// match / no-match - LessThanEqual
		{
			name:            "LessThanEqual-int-match",
			expressionType:  LessThanEqual,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(10),
			match:           true,
		},
		{
			name:            "LessThanEqual-int-no-match",
			expressionType:  LessThanEqual,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(11),
			match:           false,
		},
		{
			name:            "LessThanEqual-uint-match",
			expressionType:  LessThanEqual,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(10),
			match:           true,
		},
		{
			name:            "LessThanEqual-uint-no-match",
			expressionType:  LessThanEqual,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(11),
			match:           false,
		},
		// match / no-match - NotEqualFilter
		{
			name:            "NotEqualTo-int-match",
			expressionType:  NotEqualTo,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(9),
			match:           true,
		},
		{
			name:            "NotEqualTo-int-no-match",
			expressionType:  NotEqualTo,
			searchString:    "10",
			targetInterface: int64(0),
			input:           int64(10),
			match:           false,
		},
		{
			name:            "NotEqualTo-uint-match",
			expressionType:  NotEqualTo,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(9),
			match:           true,
		},
		{
			name:            "NotEqualTo-uint-no-match",
			expressionType:  NotEqualTo,
			searchString:    "10",
			targetInterface: uint64(0),
			input:           uint64(10),
			match:           false,
		},
	}

	// Expression create and search tests.
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			exp, err := MakeExpression(tc.expressionType, tc.searchString, tc.targetInterface)
			if tc.makeErr != "" {
				assert.ErrorContains(t, err, tc.makeErr)
				return
			}
			require.NoError(t, err)

			match, err := exp.Match(tc.input)
			if tc.searchErr != "" {
				assert.ErrorContains(t, err, tc.searchErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.match, match)
		})
	}

	// Quick check that the inputs cover all types of expressions.
	t.Run("All Expressions Tested", func(t *testing.T) {
		for _, tc := range testcases {
			allCovered[tc.expressionType] = struct{}{}
		}
		require.EqualValues(t, TypeMap, allCovered)
	})
}

func BenchmarkSearch(b *testing.B) {
	exp, err := MakeExpression(EqualTo, "10", "")
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		exp.Match("11")
	}
}
