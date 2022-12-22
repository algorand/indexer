package filewriter

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"

	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/util"
	testutil "github.com/algorand/indexer/util/test"
)

var logger *logrus.Logger
var fileCons = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &fileExporter{}
})
var configTemplate = "block-dir: %s/blocks\n"
var round = basics.Round(2)

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
	assert.Equal(t, metadata.Name, meta.Name)
	assert.Equal(t, metadata.Description, meta.Description)
	assert.Equal(t, metadata.Deprecated, meta.Deprecated)
}

func TestExporterInitDefaults(t *testing.T) {
	tempdir := t.TempDir()
	override := path.Join(tempdir, "override")

	testcases := []struct {
		blockdir string
		expected string
	}{
		{
			blockdir: "",
			expected: tempdir,
		},
		{
			blockdir: "''",
			expected: tempdir,
		},
		{
			blockdir: override,
			expected: override,
		},
		{
			blockdir: fmt.Sprintf("'%s'", override),
			expected: override,
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(fmt.Sprintf("blockdir=%s", tc.blockdir), func(t *testing.T) {
			t.Parallel()
			fileExp := fileCons.New()
			defer fileExp.Close()
			pcfg := plugins.MakePluginConfig(fmt.Sprintf("block-dir: %s", tc.blockdir))
			pcfg.DataDir = tempdir
			err := fileExp.Init(context.Background(), testutil.MockedInitProvider(&round), pcfg, logger)
			require.NoError(t, err)
			pluginConfig := fileExp.Config()
			assert.Contains(t, pluginConfig, fmt.Sprintf("block-dir: %s", tc.expected))
		})
	}
}

func TestExporterInit(t *testing.T) {
	config, _ := getConfig(t)
	fileExp := fileCons.New()
	defer fileExp.Close()

	// creates a new output file
	err := fileExp.Init(context.Background(), testutil.MockedInitProvider(&round), plugins.MakePluginConfig(config), logger)
	pluginConfig := fileExp.Config()
	configWithDefault := config + "filename-pattern: '%[1]d_block.json'\n" + "drop-certificate: false\n"
	assert.Equal(t, configWithDefault, string(pluginConfig))
	fileExp.Close()

	// can open existing file
	err = fileExp.Init(context.Background(), testutil.MockedInitProvider(&round), plugins.MakePluginConfig(config), logger)
	assert.NoError(t, err)
	fileExp.Close()
}

func sendData(t *testing.T, fileExp exporters.Exporter, config string, numRounds int) {
	// Test invalid block receive
	block := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 3,
		},
		Payset:      nil,
		Delta:       nil,
		Certificate: nil,
	}

	err := fileExp.Receive(block)
	require.Contains(t, err.Error(), "exporter not initialized")

	// initialize
	rnd := basics.Round(0)
	err = fileExp.Init(context.Background(), testutil.MockedInitProvider(&rnd), plugins.MakePluginConfig(config), logger)
	require.NoError(t, err)

	// incorrect round
	err = fileExp.Receive(block)
	require.Contains(t, err.Error(), "received round 3, expected round 0")

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
		require.NoError(t, err)
	}

	require.NoError(t, fileExp.Close())
}

func TestExporterReceive(t *testing.T) {
	config, tempdir := getConfig(t)
	fileExp := fileCons.New()
	numRounds := 5
	sendData(t, fileExp, config, numRounds)

	// block data is valid
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf(FilePattern, i)
		path := fmt.Sprintf("%s/blocks/%s", tempdir, filename)
		assert.FileExists(t, path)

		var blockData data.BlockData
		err := util.DecodeFromFile(path, &blockData, true)
		require.Equal(t, basics.Round(i), blockData.BlockHeader.Round)
		require.NoError(t, err)
		require.NotNil(t, blockData.Certificate)
	}
}

func TestExporterClose(t *testing.T) {
	config, _ := getConfig(t)
	fileExp := fileCons.New()
	rnd := basics.Round(0)
	fileExp.Init(context.Background(), testutil.MockedInitProvider(&rnd), plugins.MakePluginConfig(config), logger)
	require.NoError(t, fileExp.Close())
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

		var blockData data.BlockData
		err := util.DecodeFromFile(path, &blockData, true)
		require.Equal(t, basics.Round(i), blockData.BlockHeader.Round)
		require.NoError(t, err)
		require.NotNil(t, blockData.Certificate)
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
		err := util.DecodeFromFile(path, &blockData, true)
		assert.NoError(t, err)
		assert.Nil(t, blockData.Certificate)
	}
}
