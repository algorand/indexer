package filewriter_test

import (
	"testing"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters/filewriter"
	"github.com/algorand/indexer/plugins"
	"github.com/stretchr/testify/assert"
)

var fileCons = &filewriter.Constructor{}

var fileExp = fileCons.New()

func TestExporterMetadata(t *testing.T) {
	meta := fileExp.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, "filewriter", meta.Name())
	assert.Equal(t, "", meta.Description())
	assert.Equal(t, "", meta.Deprecated())
}

func TestExporterInit(t *testing.T) {
	assert.Panics(t, func() { fileExp.Init("", nil) })
}

func TestExporterConfig(t *testing.T) {
	assert.Panics(t, func() { fileExp.Config() })
}

func TestExporterClose(t *testing.T) {
	assert.Panics(t, func() { fileExp.Close() })
}

func TestExporterReceive(t *testing.T) {
	assert.Panics(t, func() { fileExp.Receive(data.BlockData{}) })
}

func TestExporterHandleGenesis(t *testing.T) {
	assert.Panics(t, func() { fileExp.HandleGenesis(bookkeeping.Genesis{}) })
}

func TestExporterRound(t *testing.T) {
	assert.Panics(t, func() { fileExp.Round() })
}
