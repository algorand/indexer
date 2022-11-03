package exporters

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

type mockExporter struct {
	Exporter
}

func (m *mockExporter) Metadata() conduit.Metadata {
	return conduit.Metadata{}
}

type mockExporterConstructor struct {
	me *mockExporter
}

func (c *mockExporterConstructor) New() Exporter {
	return c.me
}

func TestExporterByNameSuccess(t *testing.T) {
	me := mockExporter{}
	Register("foobar", &mockExporterConstructor{&me})

	expC, err := ExporterBuilderByName("foobar")
	assert.NoError(t, err)
	exp := expC.New()
	assert.Implements(t, (*Exporter)(nil), exp)
}

func TestExporterByNameNotFound(t *testing.T) {
	_, err := ExporterBuilderByName("barfoo")
	expectedErr := "no Exporter Constructor for barfoo"
	assert.EqualError(t, err, expectedErr)
}
