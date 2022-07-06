package exporters

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
)

type mockExporter struct {
	mock.Mock
	Exporter
}

func (m *mockExporter) Connect(config ExporterConfig) error {
	args := m.Called(config)
	return args.Error(0)
}

type mockExporterConstructor struct {
	me *mockExporter
}

func (c *mockExporterConstructor) New() Exporter {
	return c.me
}

func TestExporterByNameSuccess(t *testing.T) {
	me := mockExporter{}
	me.On("Connect", mock.Anything).Return(nil)
	RegisterExporter("foobar", &mockExporterConstructor{&me})

	exp, err := ExporterByName("foobar", "")
	assert.NoError(t, err)
	assert.Implements(t, (*Exporter)(nil), exp)
}

func TestExporterByNameNotFound(t *testing.T) {
	_, err := ExporterByName("barfoo", "")
	expectedErr := "no Exporter Constructor for barfoo"
	assert.EqualError(t, err, expectedErr)
}

func TestExporterByNameConnectFailure(t *testing.T) {
	me := mockExporter{}
	expectedErr := fmt.Errorf("connect failure")
	me.On("Connect", mock.Anything).Return(expectedErr)
	RegisterExporter("baz", &mockExporterConstructor{&me})
	_, err := ExporterByName("baz", "")
	assert.EqualError(t, err, expectedErr.Error())
}
