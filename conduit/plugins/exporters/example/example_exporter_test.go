package example

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/data"
)

var exCons = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &exampleExporter{}
})

var exExp = exCons.New()

func TestExporterMetadata(t *testing.T) {
	meta := exExp.Metadata()
	assert.Equal(t, metadata.Name, meta.Name)
	assert.Equal(t, metadata.Description, meta.Description)
	assert.Equal(t, metadata.Deprecated, meta.Deprecated)
}

func TestExporterInit(t *testing.T) {
	assert.Panics(t, func() { exExp.Init(context.Background(), nil, plugins.MakePluginConfig(""), nil) })
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
