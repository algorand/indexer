package noop

import (
	"context"
	"testing"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	testutil "github.com/algorand/indexer/util/test"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

var nc = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &noopExporter{}
})
var ne = nc.New()

func TestExporterBuilderByName(t *testing.T) {
	exporters.RegisterExporter(metadata.Name, nc)
	neBuilder, err := exporters.ExporterBuilderByName(metadata.Name)
	assert.NoError(t, err)
	ne := neBuilder.New()
	assert.Implements(t, (*exporters.Exporter)(nil), ne)
}

func TestExporterMetadata(t *testing.T) {
	meta := ne.Metadata()
	assert.Equal(t, metadata.Name, meta.Name)
	assert.Equal(t, metadata.Description, meta.Description)
	assert.Equal(t, metadata.Deprecated, meta.Deprecated)
}

func TestExporterInit(t *testing.T) {
	assert.NoError(t, ne.Init(context.Background(), testutil.MockedInitProvider(nil), "", nil))
}

func TestExporterConfig(t *testing.T) {
	defaultConfig := &ExporterConfig{}
	expected, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("unable to Marshal default noop.ExporterConfig: %v", err)
	}
	assert.NoError(t, ne.Init(context.Background(), testutil.MockedInitProvider(nil), "", nil))
	assert.Equal(t, plugins.PluginConfig(expected), ne.Config())
}

func TestExporterClose(t *testing.T) {
	assert.NoError(t, ne.Close())
}

func TestExporterRoundReceive(t *testing.T) {
	eData := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 5,
		},
	}
	assert.NoError(t, ne.Receive(eData))
}
