package noop

import (
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"testing"
)

var nc = &Constructor{}

var ne = nc.New()

func TestExporterBuilderByName(t *testing.T) {
	exporters.RegisterExporter(noopExporterMetadata.ExpName, nc)
	neBuilder, err := exporters.ExporterBuilderByName(noopExporterMetadata.ExpName)
	assert.NoError(t, err)
	ne := neBuilder.New()
	assert.Implements(t, (*exporters.Exporter)(nil), ne)
}

func TestExporterMetadata(t *testing.T) {
	meta := ne.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, noopExporterMetadata.ExpName, meta.Name())
	assert.Equal(t, noopExporterMetadata.ExpDescription, meta.Description())
	assert.Equal(t, noopExporterMetadata.ExpDeprecated, meta.Deprecated())
}

func TestExporterInit(t *testing.T) {
	assert.NoError(t, ne.Init("", nil))
}

func TestExporterConfig(t *testing.T) {
	defaultConfig := &ExporterConfig{}
	expected, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("unable to Marshal default noop.ExporterConfig: %v", err)
	}
	assert.NoError(t, ne.Init("", nil))
	assert.Equal(t, plugins.PluginConfig(expected), ne.Config())
}

func TestExporterClose(t *testing.T) {
	assert.NoError(t, ne.Close())
}

func TestExporterHandleGenesis(t *testing.T) {
	assert.NoError(t, ne.HandleGenesis(bookkeeping.Genesis{}))
}

func TestExporterStartRound(t *testing.T) {
	assert.NoError(t, ne.Init("", nil))
	assert.Equal(t, uint64(0), ne.Round())
	assert.NoError(t, ne.Init("round: 55", nil))
	assert.Equal(t, uint64(55), ne.Round())

}

func TestExporterRoundReceive(t *testing.T) {
	eData := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 5,
		},
	}
	assert.NoError(t, ne.Receive(eData))
	assert.Equal(t, uint64(6), ne.Round())
}
