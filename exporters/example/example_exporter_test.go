package example

import (
	"context"
	"testing"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	"github.com/stretchr/testify/assert"
)

var exCons exporters.ExporterConstructorFunc

var exExp = exCons.New()

func TestExporterMetadata(t *testing.T) {
	meta := exExp.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, exampleExporterMetadata.ExpName, meta.Name())
	assert.Equal(t, exampleExporterMetadata.ExpDescription, meta.Description())
	assert.Equal(t, exampleExporterMetadata.ExpDeprecated, meta.Deprecated())
}

func TestExporterInit(t *testing.T) {
	assert.Panics(t, func() { exExp.Init(context.Background(), nil, "", nil) })
}

func TestExporterConfig(t *testing.T) {
	assert.Panics(t, func() { exExp.Config() })
}

func TestExporterClose(t *testing.T) {
	assert.Panics(t, func() { exExp.Close() })
}

func TestExporterReceive(t *testing.T) {
	assert.Panics(t, func() { exExp.Receive(data.BlockData{}) })
}
