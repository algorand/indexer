package algodfollower

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

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

// TestImporterMetadata tests that metadata is correctly set
func TestImporterMetadata(t *testing.T) {
	testImporter = New()
	metadata := testImporter.Metadata()
	assert.Equal(t, metadata.Name, algodFollowerImporterMetadata.Name)
	assert.Equal(t, metadata.Description, algodFollowerImporterMetadata.Description)
	assert.Equal(t, metadata.Deprecated, algodFollowerImporterMetadata.Deprecated)
}

// TestCloseSuccess tests that closing results in no error
func TestCloseSuccess(t *testing.T) {
	ts := test.NewAlgodServer(test.GenesisResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	err = testImporter.Close()
	assert.NoError(t, err)
}

// TestInitSuccess tests that initializing results in no error
func TestInitSuccess(t *testing.T) {
	ts := test.NewAlgodServer(test.GenesisResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)
	testImporter.Close()
}

// TestInitUnmarshalFailure tests config marshaling failures
func TestInitUnmarshalFailure(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("`"), logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "connect failure in unmarshalConfig")
	testImporter.Close()
}

// TestConfigDefault tests that configuration is correct by default
func TestConfigDefault(t *testing.T) {
	testImporter = New()
	expected, err := yaml.Marshal(&Config{})
	if err != nil {
		t.Fatalf("unable to Marshal default algodimporter.Config: %v", err)
	}
	assert.Equal(t, string(expected), testImporter.Config())
}

// TestWaitForBlockBlockFailure tests that GetBlock results in a failure
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

// TestGetBlockSuccess tests that GetBlock results in success
func TestGetBlockSuccess(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	ts := test.NewAlgodServer(
		test.GenesisResponder,
		test.BlockResponder,
		test.BlockAfterResponder, test.LedgerStateDeltaResponder)
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

// TestGetBlockContextCancelled results in an error if the context is cancelled
func TestGetBlockContextCancelled(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	ts := test.NewAlgodServer(
		test.GenesisResponder,
		test.BlockResponder,
		test.BlockAfterResponder, test.LedgerStateDeltaResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	cancel()
	_, err = testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
}

// TestGetBlockFailureBlockResponder tests that GetBlock results in an error due to a lack of block responsiveness
func TestGetBlockFailureBlockResponder(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	ts := test.NewAlgodServer(
		test.GenesisResponder,
		test.BlockAfterResponder, test.LedgerStateDeltaResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	_, err = testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
	cancel()
}

// TestGetBlockFailureLedgerStateDeltaResponder tests that GetBlock results in an error due to a lack of ledger state delta
func TestGetBlockFailureLedgerStateDeltaResponder(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	ts := test.NewAlgodServer(
		test.GenesisResponder,
		test.BlockResponder,
		test.BlockAfterResponder)
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.MakePluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	_, err = testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
	cancel()
}
