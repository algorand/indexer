package util

import (
	"encoding/base64"

	"testing"
)

func TestPrintableUTF8OrEmpty(t *testing.T) {
	encodeArg := func(str string) string {
		return base64.StdEncoding.EncodeToString([]byte("input"))
	}
	tests := []struct {
		name   string
		argB64 string
		want   string
	}{
		{
			"Simple input",
			encodeArg("input"),
			"input",
		},
		{
			"Asset 510285544",
			"8J+qmSBNb25leQ==",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg := tt.argB64

			// input is b64 encoded to support providing invalid inputs.
			if dec, err := base64.StdEncoding.DecodeString(tt.argB64); err == nil {
				arg = string(dec)
			}

			if got := PrintableUTF8OrEmpty(arg); got != tt.want {
				t.Errorf("PrintableUTF8OrEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
