package filewriter

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/util"
)

var logger *logrus.Logger
var fileCons = &Constructor{}
var configTemplate = "block-dir: %s/blocks\n"

func init() {
	logger, _ = test.NewNullLogger()
}

func getConfig(t *testing.T) (config, tempdir string) {
	tempdir = t.TempDir()
	config = fmt.Sprintf(configTemplate, tempdir)
	return
}

func TestExporterMetadata(t *testing.T) {
	fileExp := fileCons.New()
	meta := fileExp.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, "file_writer", meta.Name())
	assert.Equal(t, "Exporter for writing data to a file.", meta.Description())
	assert.Equal(t, false, meta.Deprecated())
}

func TestExporterInit(t *testing.T) {
	config, tempdir := getConfig(t)
	ctx := context.Background()
	fileExp := fileCons.New()
	assert.Equal(t, uint64(0), fileExp.Round())
	// creates a new output file
	err := fileExp.Init(ctx, plugins.PluginConfig(config), logger)
	assert.NoError(t, err)
	pluginConfig := fileExp.Config()
	configWithDefault := config + "filename-pattern: '%[1]d_block.json'\n" + "drop-certificate: false\n"
	assert.Equal(t, configWithDefault, string(pluginConfig))
	assert.Equal(t, uint64(0), fileExp.Round())
	fileExp.Close()
	// can open existing file
	err = fileExp.Init(ctx, plugins.PluginConfig(config), logger)
	assert.NoError(t, err)
	fileExp.Close()
	// re-initializes empty file
	path := fmt.Sprintf("%s/blocks/metadata.json", tempdir)
	assert.NoError(t, os.Remove(path))
	f, err := os.Create(path)
	f.Close()
	assert.NoError(t, err)
	err = fileExp.Init(ctx, plugins.PluginConfig(config), logger)
	assert.NoError(t, err)
	fileExp.Close()
}

func TestExporterHandleGenesis(t *testing.T) {
	config, tempdir := getConfig(t)
	ctx := context.Background()
	fileExp := fileCons.New()
	fileExp.Init(ctx, plugins.PluginConfig(config), logger)
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

	{
		// Check that metadata.json is written
		metadataFile := fmt.Sprintf("%s/blocks/metadata.json", tempdir)
		require.FileExists(t, metadataFile)
		configs, err := ioutil.ReadFile(metadataFile)
		assert.NoError(t, err)
		var blockMetaData BlockMetaData
		err = json.Unmarshal(configs, &blockMetaData)
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), blockMetaData.NextRound)
		assert.Equal(t, string(genesisA.Network), blockMetaData.Network)
		assert.Equal(t, crypto.HashObj(genesisA).String(), blockMetaData.GenesisHash)
	}

	{
		// Check that genesis.json is written.
		genesisFile := fmt.Sprintf("%s/blocks/genesis.json", tempdir)
		require.FileExists(t, genesisFile)
		var genesis bookkeeping.Genesis
		err := util.DecodeFromFile(genesisFile, &genesis)
		assert.NoError(t, err)
		assert.Equal(t, genesisA, genesis)
	}

	// genesis mismatch
	fileExp.Init(ctx, plugins.PluginConfig(config), logger)
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

func sendData(t *testing.T, fileExp exporters.Exporter, config string, numRounds int) {
	ctx := context.Background()
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
	err = fileExp.Init(ctx, plugins.PluginConfig(config), logger)
	require.NoError(t, err)

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
	for i := 0; i < numRounds; i++ {
		block = data.BlockData{
			BlockHeader: bookkeeping.BlockHeader{
				Round: basics.Round(i),
			},
			Payset: nil,
			Delta: &ledgercore.StateDelta{
				PrevTimestamp: 1234,
			},
			Certificate: &agreement.Certificate{
				Round:  basics.Round(i),
				Period: 2,
				Step:   2,
			},
		}
		err = fileExp.Receive(block)
		assert.NoError(t, err)
		assert.Equal(t, uint64(i+1), fileExp.Round())
	}

	assert.NoError(t, fileExp.Close())
}

func TestExporterReceive(t *testing.T) {
	config, tempdir := getConfig(t)
	ctx := context.Background()
	fileExp := fileCons.New()
	numRounds := 5
	sendData(t, fileExp, config, numRounds)

	// block data is valid
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf(FilePattern, i)
		path := fmt.Sprintf("%s/blocks/%s", tempdir, filename)
		assert.FileExists(t, path)
		b, _ := os.ReadFile(path)
		var blockData data.BlockData
		err := json.Unmarshal(b, &blockData)
		assert.NoError(t, err)
		assert.NotNil(t, blockData.Certificate)
	}

	//	should continue from round 6 after restart
	fileExp.Init(ctx, plugins.PluginConfig(config), logger)
	assert.Equal(t, uint64(5), fileExp.Round())
	fileExp.Close()
}

func TestExporterClose(t *testing.T) {
	config, tempdir := getConfig(t)
	ctx := context.Background()
	fileExp := fileCons.New()
	fileExp.Init(ctx, plugins.PluginConfig(config), logger)
	block := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 0,
		},
		Payset:      nil,
		Delta:       nil,
		Certificate: nil,
	}
	err := fileExp.Receive(block)
	require.NoError(t, err)
	err = fileExp.Close()
	assert.NoError(t, err)
	metadataFile := fmt.Sprintf("%s/blocks/metadata.json", tempdir)
	require.FileExists(t, metadataFile)
	// assert round is updated correctly
	configs, err := ioutil.ReadFile(metadataFile)
	assert.NoError(t, err)
	var blockMetaData BlockMetaData
	err = json.Unmarshal(configs, &blockMetaData)
	assert.Equal(t, uint64(1), blockMetaData.NextRound)
}

func TestPatternOverride(t *testing.T) {
	config, tempdir := getConfig(t)
	fileExp := fileCons.New()

	patternOverride := "PREFIX_%[1]d_block.json"
	config = fmt.Sprintf("%sfilename-pattern: '%s'\n", config, patternOverride)

	numRounds := 5
	sendData(t, fileExp, config, numRounds)

	// block data is valid
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf(patternOverride, i)
		path := fmt.Sprintf("%s/blocks/%s", tempdir, filename)
		assert.FileExists(t, path)
		b, _ := os.ReadFile(path)
		var blockData data.BlockData
		err := json.Unmarshal(b, &blockData)
		assert.NoError(t, err)
	}
}

func TestDropCertificate(t *testing.T) {
	tempdir := t.TempDir()
	cfg := Config{
		BlocksDir:       tempdir,
		DropCertificate: true,
	}
	config, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	numRounds := 10
	exporter := fileCons.New()
	sendData(t, exporter, string(config), numRounds)

	// block data is valid
	for i := 0; i < numRounds; i++ {
		filename := fmt.Sprintf(FilePattern, i)
		path := fmt.Sprintf("%s/%s", tempdir, filename)
		assert.FileExists(t, path)
		var blockData data.BlockData
		err := util.DecodeFromFile(path, &blockData)
		assert.NoError(t, err)
		assert.Nil(t, blockData.Certificate)
	}
}
