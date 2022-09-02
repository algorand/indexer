package filewriter_test

import (
	"testing"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters/filewriter"
	"github.com/algorand/indexer/plugins"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var fileCons = &filewriter.Constructor{}

var fileExp = fileCons.New()

func TestExporterMetadata(t *testing.T) {
	meta := fileExp.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, "filewriter", meta.Name())
	assert.Equal(t, "Exporter for writing data to a file.", meta.Description())
	assert.Equal(t, false, meta.Deprecated())
}

func TestExporterConfig(t *testing.T) {
	config := "round: 10\n" +
		"path: /tmp/blocks.json\n"
	err := fileExp.Init(plugins.PluginConfig(config), log.New())
	assert.NoError(t, err)
	pluginConfig := fileExp.Config()
	assert.Equal(t, config, string(pluginConfig))
	assert.Equal(t, uint64(10), fileExp.Round())
}
func TestExporterHandleGenesis(t *testing.T) {
	assert.Panics(t, func() { fileExp.HandleGenesis(bookkeeping.Genesis{}) })
}

func TestExporterClose(t *testing.T) {
	assert.Panics(t, func() { fileExp.Close() })
}

func TestExporterReceive(t *testing.T) {
	assert.Panics(t, func() { fileExp.Receive(data.BlockData{}) })
}
