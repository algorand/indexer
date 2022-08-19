package networkimporter

import (
	"context"
	"os"
	"testing"

	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/plugins"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var (
	logger       *logrus.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	testImporter importers.Importer
)

func init() {
	logger = logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	ctx, cancel = context.WithCancel(context.Background())
}

func TestImporterorterMetadata(t *testing.T) {
	testImporter = New()
	metadata := testImporter.Metadata()
	assert.Equal(t, metadata.Type(), networkImporterMetadata.Type())
	assert.Equal(t, metadata.Name(), networkImporterMetadata.Name())
	assert.Equal(t, metadata.Description(), networkImporterMetadata.Description())
	assert.Equal(t, metadata.Deprecated(), networkImporterMetadata.Deprecated())
}

func TestCloseSuccess(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.PluginConfig(
		"genesis-path: /Users/ganesh/go-algorand/installer/genesis/mainnet/genesis.json"+
			"\nconfig-path: /Users/ganesh/go-algorand/installer/"), logger)
	assert.NoError(t, err)
	err = testImporter.Close()
	assert.NoError(t, err)
}

func TestInitSuccess(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.PluginConfig(
		"genesis-path: /Users/ganesh/go-algorand/installer/genesis/mainnet/genesis.json"+
			"\nconfig-path: /Users/ganesh/go-algorand/installer/"), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)
	testImporter.Close()
}

func TestInitUnmarshalFailure(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, "`", logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "connect failure in unmarshalConfig")
	testImporter.Close()
}

func TestConfigDefault(t *testing.T) {
	testImporter = New()
	expected, err := yaml.Marshal(&ImporterConfig{})
	if err != nil {
		t.Fatalf("unable to Marshal default networkImporter.ImporterConfig: %v", err)
	}
	assert.Equal(t, plugins.PluginConfig(expected), testImporter.Config())
}

func TestGetBlockSuccess(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.PluginConfig(
		"genesis-path: /Users/ganesh/go-algorand/installer/genesis/mainnet/genesis.json"+
			"\nconfig-path: /Users/ganesh/go-algorand/installer/"), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	downloadedBlk, err := testImporter.GetBlock(uint64(20000000))
	assert.NoError(t, err)
	assert.Equal(t, downloadedBlk.Round(), uint64(20000000))
	cancel()
}

func TestGetBlockNoBlockFound(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.PluginConfig(
		"genesis-path: /Users/ganesh/go-algorand/installer/genesis/mainnet/genesis.json"+
			"\nconfig-path: /Users/ganesh/go-algorand/installer/lessRetries/"), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	_, err = testImporter.GetBlock(uint64(23000000000))
	require.Contains(t, err.Error(), "FetchBlock failed after multiple blocks download attempts")
	cancel()
}

func TestGetBlockContextCancelled(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.PluginConfig(
		"genesis-path: /Users/ganesh/go-algorand/installer/genesis/mainnet/genesis.json"+
			"\nconfig-path: /Users/ganesh/go-algorand/installer/"), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	cancel()
	_, err = testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
}
