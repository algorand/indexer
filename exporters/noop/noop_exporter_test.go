package noop

import (
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"testing"
)

var nc = &Constructor{}

var ne = nc.New()

func TestExporterByName(t *testing.T) {
	logger, _ := test.NewNullLogger()
	exporters.RegisterExporter(Metadata.ExpName, nc)
	ne, err := exporters.ExporterByName(Metadata.ExpName, "", logger)
	assert.NoError(t, err)
	assert.Implements(t, (*exporters.Exporter)(nil), ne)
}

func TestExporterMetadata(t *testing.T) {
	meta := ne.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, Metadata.ExpName, meta.Name())
	assert.Equal(t, Metadata.ExpDescription, meta.Description())
	assert.Equal(t, Metadata.ExpDeprecated, meta.Deprecated())
}

func TestExporterConnect(t *testing.T) {
	assert.NoError(t, ne.Connect("", nil))
}

func TestExporterConfig(t *testing.T) {
	assert.Equal(t, plugins.PluginConfig(""), ne.Config())
}

func TestExporterDisconnect(t *testing.T) {
	assert.NoError(t, ne.Disconnect())
}

func TestExporterHandleGenesis(t *testing.T) {
	assert.NoError(t, ne.HandleGenesis(bookkeeping.Genesis{}))
}

func TestExporterRoundReceive(t *testing.T) {
	eData := data.BlockData{
		Block: &bookkeeping.Block{
			BlockHeader: bookkeeping.BlockHeader{
				Round: 5,
			},
		},
	}
	assert.Equal(t, uint64(0), ne.Round())
	assert.NoError(t, ne.Receive(eData))
	assert.Equal(t, uint64(6), ne.Round())
}
