package filewriter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/plugins"
	testutil "github.com/algorand/indexer/util/test"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

var logger *logrus.Logger
var fileCons = &Constructor{}
var round = basics.Round(2)

func init() {
	logger, _ = test.NewNullLogger()
	os.RemoveAll("/tmp/blocks")
}

func TestExporterMetadata(t *testing.T) {
	fileExp := fileCons.New()
	meta := fileExp.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, "filewriter", meta.Name())
	assert.Equal(t, "Exporter for writing data to a file.", meta.Description())
	assert.Equal(t, false, meta.Deprecated())
}

func TestExporterInit(t *testing.T) {
	config := fmt.Sprintf("block-dir: %s/blocks\n", t.TempDir())
	fileExp := fileExporter{}
	defer fileExp.Close()
	err := fileExp.Init(context.Background(), testutil.MockedInitProvider(&round), plugins.PluginConfig(config), logger)
	assert.NoError(t, err)
	pluginConfig := fileExp.Config()
	assert.Equal(t, config, string(pluginConfig))
	assert.Equal(t, uint64(round), fileExp.round)
}

func TestExporterReceive(t *testing.T) {
	config := fmt.Sprintf("block-dir: %s/blocks\n", t.TempDir())
	fileExp := fileExporter{}
	defer fileExp.Close()
	block := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 3,
		},
		Payset:      nil,
		Delta:       nil,
		Certificate: nil,
	}

	err := fileExp.Receive(block)
	assert.Contains(t, err.Error(), "exporter not initialized")

	err = fileExp.Init(context.Background(), testutil.MockedInitProvider(&round), plugins.PluginConfig(config), logger)
	assert.Nil(t, err)
	err = fileExp.Receive(block)
	assert.Contains(t, err.Error(), "received round 3, expected round 2")

	// write block to file; pipeline starts at round 2, set in MockedInitProvider
	for i := 2; i < 7; i++ {
		block = data.BlockData{
			BlockHeader: bookkeeping.BlockHeader{
				Round: basics.Round(i),
			},
			Payset:      nil,
			Delta:       nil,
			Certificate: nil,
		}
		err = fileExp.Receive(block)
		assert.NoError(t, err)
		assert.Equal(t, uint64(i+1), fileExp.round)

	}

	// written data are valid
	for i := 2; i < 7; i++ {
		b, _ := os.ReadFile(filepath.Join(fileExp.cfg.BlocksDir, fmt.Sprintf("block_%d.json", i)))
		var blockData data.BlockData
		err = json.Unmarshal(b, &blockData)
		assert.NoError(t, err)
	}
}
