package noop

import (
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	testutil "github.com/algorand/indexer/util/test"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

var nc = &Constructor{}

var ne = nc.New()

var round = basics.Round(0)
var mockedInitProvider = &testutil.MockInitProvider{
	CurrentRound: &round,
	Genesis:      &bookkeeping.Genesis{},
}

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
	assert.NoError(t, ne.Init(mockedInitProvider, "", nil))
}

func TestExporterConfig(t *testing.T) {
	defaultConfig := &ExporterConfig{}
	expected, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("unable to Marshal default noop.ExporterConfig: %v", err)
	}
	assert.NoError(t, ne.Init(mockedInitProvider, "", nil))
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
