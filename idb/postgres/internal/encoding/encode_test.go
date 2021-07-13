package encoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeNulls(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"aoeu", "aoeu"},
		{"ao\x00eu", "ao\\u0000eu"},
		{"ao\\u0000eu", "ao\\\\u0000eu"},
		{"ao\xc0 eu", "ao\\u00c0 eu"}, // invalid utf8 \xc0\x20
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			actual := EscapeNulls(tc.input)
			assert.Equal(t, tc.expected, actual, "forward")
			restore := UnescapeNulls(tc.expected)
			assert.Equal(t, tc.input, restore, "reverse")
		})
	}
}
