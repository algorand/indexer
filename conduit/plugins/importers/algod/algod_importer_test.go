package algodimporter

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/util/test"
)

var (
	logger *logrus.Logger
	ctx    context.Context
	cancel context.CancelFunc
)

func init() {
	logger = logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	ctx, cancel = context.WithCancel(context.Background())
}

func TestImporterMetadata(t *testing.T) {
	testImporter := New()
	metadata := testImporter.Metadata()
	assert.Equal(t, metadata.Name, algodImporterMetadata.Name)
	assert.Equal(t, metadata.Description, algodImporterMetadata.Description)
	assert.Equal(t, metadata.Deprecated, algodImporterMetadata.Deprecated)
}

func TestCloseSuccess(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"archival"},
		{"follower"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ts := test.NewAlgodServer(test.GenesisResponder)
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ts.URL)
			_, err := testImporter.Init(ctx, plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			err = testImporter.Close()
			assert.NoError(t, err)
		})
	}
}

func TestInitSuccess(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"archival"},
		{"follower"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ts := test.NewAlgodServer(test.GenesisResponder)
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ts.URL)
			_, err := testImporter.Init(ctx, plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)
			testImporter.Close()
		})
	}
}

func TestInitGenesisFailure(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"archival"},
		{"follower"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ts := test.NewAlgodServer(test.MakeGenesisResponder(sdk.Genesis{}))
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ts.URL)
			_, err := testImporter.Init(ctx, plugins.MakePluginConfig(cfgStr), logger)
			assert.Error(t, err)
			assert.ErrorContains(t, err, "unable to fetch genesis file")
			testImporter.Close()
		})
	}
}

func TestInitUnmarshalFailure(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"archival"},
		{"follower"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			testImporter := New()
			_, err := testImporter.Init(ctx, plugins.MakePluginConfig("`"), logger)
			assert.Error(t, err)
			assert.ErrorContains(t, err, "connect failure in unmarshalConfig")
			testImporter.Close()
		})
	}
}

func TestConfigDefault(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"archival"},
		{"follower"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			testImporter := New()
			expected, err := yaml.Marshal(&Config{})
			if err != nil {
				t.Fatalf("unable to Marshal default algodimporter.Config: %v", err)
			}
			assert.Equal(t, string(expected), testImporter.Config())
		})
	}
}

func TestWaitForBlockBlockFailure(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"archival"},
		{"follower"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ts := test.NewAlgodServer(test.GenesisResponder)
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ts.URL)
			_, err := testImporter.Init(ctx, plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)

			blk, err := testImporter.GetBlock(uint64(10))
			assert.Error(t, err)
			assert.True(t, blk.Empty())
		})
	}
}

func TestGetBlockSuccess(t *testing.T) {
	tests := []struct {
		name        string
		algodServer *httptest.Server
	}{
		{"archival", test.NewAlgodServer(test.GenesisResponder,
			test.BlockResponder,
			test.BlockAfterResponder)},
		{"follower", test.NewAlgodServer(test.GenesisResponder,
			test.BlockResponder,
			test.BlockAfterResponder, test.LedgerStateDeltaResponder)},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ctx, cancel = context.WithCancel(context.Background())
			testImporter := New()

			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ttest.algodServer.URL)
			_, err := testImporter.Init(ctx, plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)

			downloadedBlk, err := testImporter.GetBlock(uint64(10))
			assert.NoError(t, err)
			assert.Equal(t, downloadedBlk.Round(), uint64(10))
			assert.True(t, downloadedBlk.Empty())
			cancel()
		})
	}
}

func TestGetBlockContextCancelled(t *testing.T) {
	tests := []struct {
		name        string
		algodServer *httptest.Server
	}{
		{"archival", test.NewAlgodServer(test.GenesisResponder,
			test.BlockResponder,
			test.BlockAfterResponder)},
		{"follower", test.NewAlgodServer(test.GenesisResponder,
			test.BlockResponder,
			test.BlockAfterResponder, test.LedgerStateDeltaResponder)},
	}

	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ctx, cancel = context.WithCancel(context.Background())
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ttest.algodServer.URL)
			_, err := testImporter.Init(ctx, plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)

			cancel()
			_, err = testImporter.GetBlock(uint64(10))
			assert.Error(t, err)
		})
	}
}

func TestGetBlockFailure(t *testing.T) {
	tests := []struct {
		name        string
		algodServer *httptest.Server
	}{
		{"archival", test.NewAlgodServer(test.GenesisResponder,
			test.BlockAfterResponder)},
		{"follower", test.NewAlgodServer(test.GenesisResponder,
			test.BlockAfterResponder, test.LedgerStateDeltaResponder)},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ctx, cancel = context.WithCancel(context.Background())
			testImporter := New()

			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ttest.algodServer.URL)
			_, err := testImporter.Init(ctx, plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)

			_, err = testImporter.GetBlock(uint64(10))
			assert.Error(t, err)
			cancel()
		})
	}
}

func TestAlgodImporter_ProvideMetrics(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"archival"},
		{"follower"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			testImporter := &algodImporter{}
			assert.Len(t, testImporter.ProvideMetrics("blah"), 1)
		})
	}
}
