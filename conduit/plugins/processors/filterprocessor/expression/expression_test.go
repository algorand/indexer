package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/data/basics"
)

func TestExpression(t *testing.T) {
	testcases := []struct {
		name         string
		input        interface{}
		filterType   FilterType
		searchString string
		kind         interface{}
		errorString  string
		match        bool
	}{
		{
			name:         "Alias 1 (2 < 4)",
			input:        basics.AppIndex(2),
			filterType:   LessThanFilter,
			searchString: "4",
			kind:         uint64(0),
			errorString:  "",
			match:        true,
		},
		{
			name:         "Alias 2 (4 < 4)",
			input:        basics.AppIndex(4),
			filterType:   LessThanFilter,
			searchString: "4",
			kind:         uint64(0),
			errorString:  "",
			match:        false,
		},
		{
			name:         "Alias 3 (4 <= 4)",
			input:        basics.AppIndex(4),
			filterType:   LessThanEqualFilter,
			searchString: "4",
			kind:         uint64(0),
			errorString:  "",
			match:        true,
		},
		{
			name:         "Alias 4 (4 == 4)",
			input:        basics.AppIndex(4),
			filterType:   EqualToFilter,
			searchString: "4",
			kind:         uint64(0),
			errorString:  "",
			match:        true,
		},
		{
			name:         "Alias 5 (4 == 5)",
			input:        basics.AppIndex(4),
			filterType:   EqualToFilter,
			searchString: "5",
			kind:         uint64(0),
			errorString:  "",
			match:        false,
		},
		{
			name:         "Alias 6 (4 != 5)",
			input:        basics.AppIndex(4),
			filterType:   NotEqualToFilter,
			searchString: "5",
			kind:         uint64(0),
			errorString:  "",
			match:        true,
		},
		{
			name:         "Alias 7 (4 > 20)",
			input:        basics.AppIndex(4),
			filterType:   GreaterThanFilter,
			searchString: "20",
			kind:         uint64(0),
			errorString:  "",
			match:        false,
		},
		{
			name:         "Alias 8 (4 > 2)",
			input:        basics.AppIndex(4),
			filterType:   GreaterThanFilter,
			searchString: "2",
			kind:         uint64(0),
			errorString:  "",
			match:        true,
		},
		{
			name:         "Alias 9 (4 >= 4)",
			input:        basics.AppIndex(4),
			filterType:   GreaterThanEqualFilter,
			searchString: "4",
			kind:         uint64(0),
			errorString:  "",
			match:        true,
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			exp, err := MakeExpression(tc.filterType, tc.searchString, tc.kind)
			require.NoError(t, err)

			m, err := (*exp).Search(tc.input)
			if tc.errorString != "" {
				assert.ErrorContains(t, err, tc.errorString)
				return
			}
			assert.Equal(t, tc.match, m)
		})
	}
}
