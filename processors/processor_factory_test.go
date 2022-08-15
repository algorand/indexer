package processors

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/plugins"
)

var logger *logrus.Logger

func init() {
	logger, _ = test.NewNullLogger()
}

type mockProvider struct {
}

func (m mockProvider) AdvanceDBRound() {
}

func (m mockProvider) Genesis() *bookkeeping.Genesis {
	return nil
}

func (m mockProvider) NextDBRound() basics.Round {
	return basics.Round(0)
}

type mockProcessor struct {
	mock.Mock
	Processor
}

func (m *mockProcessor) Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error {
	args := m.Called(ctx, initProvider, cfg, logger)
	return args.Error(0)
}

func (m *mockProcessor) Metadata() ProcessorMetadata {
	return MakeProcessorMetadata("foobar", "", false)
}

type mockProcessorConstructor struct {
	me *mockProcessor
}

func (c *mockProcessorConstructor) New() Processor {
	return c.me
}

func TestProcessorByNameSuccess(t *testing.T) {
	me := mockProcessor{}
	me.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	RegisterProcessor("foobar", &mockProcessorConstructor{&me})

	exp, err := ProcessorByName(context.Background(), "foobar", "", mockProvider{}, logger)
	assert.NoError(t, err)
	assert.Implements(t, (*Processor)(nil), exp)
}

func TestProcessorByNameNotFound(t *testing.T) {
	_, err := ProcessorByName(context.Background(), "barfoo", "", mockProvider{}, logger)
	expectedErr := "no Processor Constructor for barfoo"
	assert.EqualError(t, err, expectedErr)
}

func TestProcessorByNameConnectFailure(t *testing.T) {
	me := mockProcessor{}
	expectedErr := fmt.Errorf("connect failure")
	me.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(expectedErr)
	RegisterProcessor("baz", &mockProcessorConstructor{&me})
	_, err := ProcessorByName(context.Background(), "baz", "", mockProvider{}, logger)
	assert.EqualError(t, err, expectedErr.Error())
}
