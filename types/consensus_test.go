package types

import (
	"testing"
)

func TestProtocol(t *testing.T) {
	_, err := Protocol("future")
	if err != nil {
		t.Fatalf("future %v", err)
	}
}
