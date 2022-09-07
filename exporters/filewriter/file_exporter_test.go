package filewriter_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters/filewriter"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

var logger *logrus.Logger
var fileCons = &filewriter.Constructor{}

func init() {
	logger, _ = test.NewNullLogger()
	os.Remove("/tmp/block1.json")
	os.Remove("/tmp/block2.json")
	os.Remove("/tmp/block3.json")
}

func TestExporterMetadata(t *testing.T) {
	fileExp := fileCons.New()
	meta := fileExp.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, "filewriter", meta.Name())
	assert.Equal(t, "Exporter for writing data to a file.", meta.Description())
	assert.Equal(t, false, meta.Deprecated())
}

func TestExporterConfig(t *testing.T) {
	fileExp := fileCons.New()
	assert.Equal(t, uint64(0), fileExp.Round())
	config := "round: 10\n" +
		"path: /tmp/blocks1.json\n" +
		"configs: \"\"\n"
	// creates a new output file
	err := fileExp.Init(plugins.PluginConfig(config), logger)
	defer fileExp.Close()
	assert.NoError(t, err)
	pluginConfig := fileExp.Config()
	assert.Equal(t, config, string(pluginConfig))
	assert.Equal(t, uint64(10), fileExp.Round())
	// can open existing file
	err = fileExp.Init(plugins.PluginConfig(config), logger)
	defer fileExp.Close()
	assert.NoError(t, err)
}
func TestExporterHandleGenesis(t *testing.T) {
	fileExp := fileCons.New()
	config := "round: 10\n" +
		"path: /tmp/blocks2.json\n"
	fileExp.Init(plugins.PluginConfig(config), logger)
	defer fileExp.Close()
	genesisA := bookkeeping.Genesis{
		SchemaID:    "test",
		Network:     "test",
		Proto:       "test",
		Allocation:  nil,
		RewardsPool: "AAAAAAA",
		FeeSink:     "AAAAAAA",
		Timestamp:   1234,
		Comment:     "",
		DevMode:     true,
	}
	err := fileExp.HandleGenesis(genesisA)
	assert.NoError(t, err)
	fd, _ := os.OpenFile("/tmp/blocks2.json", os.O_RDONLY, 0755)
	stat, _ := fd.Stat()
	assert.Greater(t, int(stat.Size()), 0)

	// genesis mismatch
	fileExp.Init(plugins.PluginConfig(config), logger)
	defer fileExp.Close()
	genesisB := bookkeeping.Genesis{
		SchemaID:    "test",
		Network:     "test",
		Proto:       "test",
		Allocation:  nil,
		RewardsPool: "AAAAAAA",
		FeeSink:     "AAAAAAA",
		Timestamp:   5678,
		Comment:     "",
		DevMode:     false,
	}

	err = fileExp.HandleGenesis(genesisB)
	assert.Contains(t, err.Error(), "genesis hash in file /tmp/blocks2.json does not match expected value")
}

func TestExporterReceive(t *testing.T) {
	fileExp := fileCons.New()
	block := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 3,
		},
		Payset:      nil,
		Delta:       nil,
		Certificate: nil,
	}
	// exporter not initialized
	err := fileExp.Receive(block)
	assert.Contains(t, err.Error(), "exporter not initialized")
	// initialize
	config := "round: 2\n" +
		"path: /tmp/blocks3.json\n"
	fileExp.Init(plugins.PluginConfig(config), logger)
	defer fileExp.Close()
	// incorrect round
	err = fileExp.Receive(block)
	assert.Contains(t, err.Error(), "received round 3, expected round 2")

	// write block to file
	for i := 2; i < 8; i++ {
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
		assert.Equal(t, uint64(i+1), fileExp.Round())
	}
	// written data are valid
	fd, _ := os.OpenFile("/tmp/blocks3.json", os.O_RDONLY, 0755)
	scanner := bufio.NewScanner(fd)
	var blockData data.BlockData
	for scanner.Scan() {
		err := json.Unmarshal([]byte(scanner.Text()), &blockData)
		assert.NoError(t, err)
	}
}

func TestExporterClose(t *testing.T) {
	configsPath := "/tmp/configs.yml"
	f, err := os.OpenFile(configsPath, os.O_CREATE|os.O_WRONLY, 0755)
	assert.NoError(t, err)
	defer f.Close()
	fileExp := fileCons.New()
	config := "round: 13\n" +
		"path: /tmp/blocks3.json\n" +
		fmt.Sprintf("configs: %s", configsPath)
	fileExp.Init(plugins.PluginConfig(config), logger)
	block := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 13,
		},
		Payset:      nil,
		Delta:       nil,
		Certificate: nil,
	}
	fileExp.Receive(block)
	err = fileExp.Close()
	assert.NoError(t, err)
	// assert round is updated correctly
	configs, err := ioutil.ReadFile(configsPath)
	assert.NoError(t, err)
	var exporterConfig filewriter.ExporterConfig
	err = yaml.Unmarshal(configs, &exporterConfig)
	assert.NoError(t, err)
	assert.Equal(t, uint64(14), exporterConfig.Round)
}
