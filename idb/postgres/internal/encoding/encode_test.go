package encoding

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeNulls(t *testing.T) {
	const moreEmojiThanEmacsCanHandleBase64 = "8J+YgPCfporwn42M8J+lj/Cfmo7wn5ec8J+VifCfj7TigI3imKDvuI8="
	manyEmojiBytes, _ := base64.StdEncoding.DecodeString(moreEmojiThanEmacsCanHandleBase64)
	manyEmoji := string(manyEmojiBytes)
	tests := []struct {
		input    string
		expected string
	}{
		{"aoeu", "aoeu"},                 // no change
		{"ao\x00eu", "ao\\u0000eu"},      // zero byte
		{"ao\\u0000eu", "ao\\\\u0000eu"}, // \ -> \\
		{"ao\xc0 eu", "ao\\u00c0 eu"},    // invalid utf8 \xc0\x20
		{"ăѣ𝔠ծềſģȟᎥ𝒋ǩľḿꞑȯ𝘱𝑞𝗋𝘴ȶ𝞄𝜈ψ𝒙𝘆𝚣1234567890!@#$%^&*()-_=+[{]};:'\",<.>/?", "ăѣ𝔠ծềſģȟᎥ𝒋ǩľḿꞑȯ𝘱𝑞𝗋𝘴ȶ𝞄𝜈ψ𝒙𝘆𝚣1234567890!@#$%^&*()-_=+[{]};:'\",<.>/?"},
		{manyEmoji, manyEmoji},
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
