package slack

import (
	"github.com/algorand/indexer/plugins"
	"github.com/stretchr/testify/assert"
	"testing"
)

var exCons = &Constructor{}

var exExp = exCons.New()

func TestExporterMetadata(t *testing.T) {
	meta := exExp.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, slackExporterMetadata.ExpName, meta.Name())
	assert.Equal(t, slackExporterMetadata.ExpDescription, meta.Description())
	assert.Equal(t, slackExporterMetadata.ExpDeprecated, meta.Deprecated())
}

func TestExporterInit(t *testing.T) {
	// assert.Panics(t, func() { exExp.Init("", nil) })
}

func TestExporterConfig(t *testing.T) {
	// assert.Panics(t, func() { exExp.Config() })
}

func TestExporterClose(t *testing.T) {
	// assert.Panics(t, func() { exExp.Close() })
}

func TestExporterReceive(t *testing.T) {
	// assert.Panics(t, func() { exExp.Receive(data.BlockData{}) })
}

func TestExporterHandleGenesis(t *testing.T) {
	// assert.Panics(t, func() { exExp.HandleGenesis(bookkeeping.Genesis{}) })
}

func TestExporterRound(t *testing.T) {
	// assert.Panics(t, func() { exExp.Round() })
}
