package noop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/data/bookkeeping"

	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/data"
	testutil "github.com/algorand/indexer/util/test"
)

var nc = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &noopExporter{}
})
var ne = nc.New()

func TestExporterBuilderByName(t *testing.T) {
	exporters.Register(metadata.Name, nc)
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
	assert.NoError(t, ne.Init(context.Background(), testutil.MockedInitProvider(nil), plugins.MakePluginConfig(""), nil))
}

func TestExporterConfig(t *testing.T) {
	defaultConfig := &ExporterConfig{}
	expected, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("unable to Marshal default noop.ExporterConfig: %v", err)
	}
	assert.NoError(t, ne.Init(context.Background(), testutil.MockedInitProvider(nil), plugins.MakePluginConfig(""), nil))
	assert.Equal(t, string(expected), ne.Config())
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
