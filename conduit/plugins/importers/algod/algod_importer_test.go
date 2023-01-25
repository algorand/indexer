package algodimporter

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/importers"
	"github.com/algorand/indexer/util/test"
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

func TestImporterMetadata(t *testing.T) {
	testImporter = New()
	metadata := testImporter.Metadata()
	assert.Equal(t, metadata.Name, algodImporterMetadata.Name)
	assert.Equal(t, metadata.Description, algodImporterMetadata.Description)
	assert.Equal(t, metadata.Deprecated, algodImporterMetadata.Deprecated)
}

func TestCloseSuccess(t *testing.T) {
	ts := test.NewAlgodServer(test.GenesisResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	err = testImporter.Close()
	assert.NoError(t, err)
}

func TestInitSuccess(t *testing.T) {
	ts := test.NewAlgodServer(test.GenesisResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)
	testImporter.Close()
}

func TestInitGenesisFailure(t *testing.T) {
	ts := test.NewAlgodServer(test.MakeGenesisResponder(sdk.Genesis{}))
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "unable to fetch genesis file")
	testImporter.Close()
}

func TestInitUnmarshalFailure(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("`"), logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "connect failure in unmarshalConfig")
	testImporter.Close()
}

func TestConfigDefault(t *testing.T) {
	testImporter = New()
	expected, err := yaml.Marshal(&Config{})
	if err != nil {
		t.Fatalf("unable to Marshal default algodimporter.Config: %v", err)
	}
	assert.Equal(t, string(expected), testImporter.Config())
}

func TestWaitForBlockBlockFailure(t *testing.T) {
	ts := test.NewAlgodServer(test.GenesisResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	blk, err := testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
	assert.True(t, blk.Empty())
}

func TestGetBlockSuccess(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	ts := test.NewAlgodServer(
		test.GenesisResponder,
		test.BlockResponder,
		test.BlockAfterResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	downloadedBlk, err := testImporter.GetBlock(uint64(10))
	assert.NoError(t, err)
	assert.Equal(t, downloadedBlk.Round(), uint64(10))
	assert.True(t, downloadedBlk.Empty())
	cancel()
}

func TestGetBlockContextCancelled(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	ts := test.NewAlgodServer(
		test.GenesisResponder,
		test.BlockResponder,
		test.BlockAfterResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	cancel()
	_, err = testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
}

func TestGetBlockFailure(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	ts := test.NewAlgodServer(
		test.GenesisResponder,
		test.BlockAfterResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	_, err = testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
	cancel()
}
