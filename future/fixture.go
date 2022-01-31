package future

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type msg struct {
	Subject string
	Body    []byte
}

func TestMsg(t *testing.T) {
	m := msg{"hello", nil}
	assert.Equal(t, "hello", m.Subject)
}