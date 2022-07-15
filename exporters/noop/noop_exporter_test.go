package noop

import (
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/exporters"
	"github.com/stretchr/testify/assert"
	"testing"
)

var nc = &Constructor{}

var ne = nc.New()

func TestExporterByName(t *testing.T) {
	exporters.RegisterExporter(noopExporterMetadata.Name, nc)
	ne, err := exporters.ExporterByName(noopExporterMetadata.Name, "")
	assert.NoError(t, err)
	assert.Implements(t, (*exporters.Exporter)(nil), ne)
}

func TestExporterMetadata(t *testing.T) {
	meta := ne.Metadata()
	assert.Equal(t, noopExporterMetadata.Name, meta.Name)
	assert.Equal(t, noopExporterMetadata.Description, meta.Description)
	assert.Equal(t, noopExporterMetadata.Deprecated, meta.Deprecated)
}

func TestExporterConnect(t *testing.T) {
	assert.NoError(t, ne.Connect(""))
}

func TestExporterConfig(t *testing.T) {
	assert.Equal(t, exporters.ExporterConfig(""), ne.Config())
}

func TestExporterDisconnect(t *testing.T) {
	assert.NoError(t, ne.Disconnect())
}

func TestExporterHandleGenesis(t *testing.T) {
	assert.NoError(t, ne.HandleGenesis(bookkeeping.Genesis{}))
}

func TestExporterRoundReceive(t *testing.T) {
	eData := &exporters.BlockExportData{
		Block: bookkeeping.Block{
			BlockHeader: bookkeeping.BlockHeader{
				Round: 5,
			},
		},
	}
	assert.Equal(t, uint64(0), ne.Round())
	assert.NoError(t, ne.Receive(eData))
	assert.Equal(t, uint64(6), ne.Round())
}
