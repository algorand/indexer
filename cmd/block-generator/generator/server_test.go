package generator

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRound(t *testing.T) {
	var testcases = []struct {
		name          string
		url           string
		expectedRound uint64
	}{
		{
			name:          "invalid prefix",
			url:           fmt.Sprintf("/v2/wrong/prefix/1"),
			expectedRound: 0,
		},
		{
			name:          "normal one digit",
			url:           fmt.Sprintf("%s1", blockQueryPrefix),
			expectedRound: 1,
		},
		{
			name:          "normal long number",
			url:           fmt.Sprintf("%s12345678", blockQueryPrefix),
			expectedRound: 12345678,
		},
		{
			name:          "with query parameters",
			url:           fmt.Sprintf("%s1234?pretty", blockQueryPrefix),
			expectedRound: 1234,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			round, err := parseRound(testcase.url)
			if err != nil && testcase.expectedRound != 0 {
				assert.Fail(t, fmt.Sprintf("Unexpected error parsing '%s', expected round '%d' received error: %v", testcase.url, testcase.expectedRound, err))
			}
			assert.Equal(t, testcase.expectedRound, round)
		})
	}
}
