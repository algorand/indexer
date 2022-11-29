package processors

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/indexer/conduit"
)

var logger *logrus.Logger

func init() {
	logger, _ = test.NewNullLogger()
}

type mockProcessor struct {
	Processor
}

func (m *mockProcessor) Metadata() conduit.Metadata {
	return conduit.Metadata{
		Name:         "foobar",
		Description:  "",
		Deprecated:   false,
		SampleConfig: "",
	}
}

type mockProcessorConstructor struct {
	me *mockProcessor
}

func (c *mockProcessorConstructor) New() Processor {
	return c.me
}

func TestProcessorBuilderByNameSuccess(t *testing.T) {
	me := mockProcessor{}
	Register("foobar", &mockProcessorConstructor{&me})

	expBuilder, err := ProcessorBuilderByName("foobar")
	assert.NoError(t, err)
	exp := expBuilder.New()
	assert.Implements(t, (*Processor)(nil), exp)
}

func TestProcessorBuilderByNameNotFound(t *testing.T) {
	_, err := ProcessorBuilderByName("barfoo")
	expectedErr := "no Processor Constructor for barfoo"
	assert.EqualError(t, err, expectedErr)
}
