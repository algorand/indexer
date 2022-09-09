package filewriter_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters/filewriter"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

var logger *logrus.Logger
var fileCons = &filewriter.Constructor{}
var config = "block-dir: /tmp/blocks\n"

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
	fileExp := fileCons.New()
	assert.Equal(t, uint64(0), fileExp.Round())
	// creates a new output file
	err := fileExp.Init(plugins.PluginConfig(config), logger)
	assert.NoError(t, err)
	pluginConfig := fileExp.Config()
	assert.Equal(t, config, string(pluginConfig))
	assert.Equal(t, uint64(0), fileExp.Round())
	fileExp.Close()
	// can open existing file
	err = fileExp.Init(plugins.PluginConfig(config), logger)
	assert.NoError(t, err)
	fileExp.Close()

}
func TestExporterHandleGenesis(t *testing.T) {
	fileExp := fileCons.New()
	fileExp.Init(plugins.PluginConfig(config), logger)
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
	fileExp.Close()
	assert.NoError(t, err)
	configs, err := ioutil.ReadFile("/tmp/blocks/metadata.json")
	assert.NoError(t, err)
	var blockMetaData filewriter.BlockMetaData
	err = json.Unmarshal(configs, &blockMetaData)
	assert.Equal(t, uint64(0), blockMetaData.NextRound)
	assert.Equal(t, string(genesisA.Network), blockMetaData.Network)
	assert.Equal(t, crypto.HashObj(genesisA).String(), blockMetaData.GenesisHash)

	// genesis mismatch
	fileExp.Init(plugins.PluginConfig(config), logger)
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
	assert.Contains(t, err.Error(), "genesis hash in metadata does not match expected value")
	fileExp.Close()

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
	fileExp.Init(plugins.PluginConfig(config), logger)

	// incorrect round
	err = fileExp.Receive(block)
	assert.Contains(t, err.Error(), "received round 3, expected round 0")

	// genesis
	genesis := bookkeeping.Genesis{
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
	err = fileExp.HandleGenesis(genesis)
	assert.NoError(t, err)

	// write block to file
	for i := 0; i < 5; i++ {
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
	fileExp.Close()

	// written data are valid
	for i := 0; i < 5; i++ {
		b, _ := os.ReadFile(fmt.Sprintf("/tmp/blocks/block_%d.json", i))
		var blockData data.BlockData
		err = json.Unmarshal(b, &blockData)
		assert.NoError(t, err)
	}

	//	should continue from round 6 after restart
	fileExp.Init(plugins.PluginConfig(config), logger)
	assert.Equal(t, uint64(5), fileExp.Round())
	fileExp.Close()
}

func TestExporterClose(t *testing.T) {
	fileExp := fileCons.New()
	fileExp.Init(plugins.PluginConfig(config), logger)
	block := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 5,
		},
		Payset:      nil,
		Delta:       nil,
		Certificate: nil,
	}
	fileExp.Receive(block)
	err := fileExp.Close()
	assert.NoError(t, err)
	// assert round is updated correctly
	configs, err := ioutil.ReadFile("/tmp/blocks/metadata.json")
	assert.NoError(t, err)
	var blockMetaData filewriter.BlockMetaData
	err = json.Unmarshal(configs, &blockMetaData)
	assert.Equal(t, uint64(6), blockMetaData.NextRound)
}
