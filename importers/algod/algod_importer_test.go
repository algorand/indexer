package algodimporter

import (
	"context"
	"testing"

	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

var (
	logger *logrus.Logger
	ctx    context.Context
	cancel context.CancelFunc
)

func init() {
	logger, _ = test.NewNullLogger()
	ctx, cancel = context.WithCancel(context.Background())
}

func TestMetadata(t *testing.T) {
	bot := New()
	metadata := bot.Metadata()
	assert.Equal(t, metadata.Type(), plugins.PluginType("importer"))
	assert.Equal(t, metadata.Name(), "algod")
	assert.Equal(t, metadata.Description(), "Importer for fetching block from algod rest endpoint.")
	assert.Equal(t, metadata.Deprecated(), false)
}

func TestClose(t *testing.T) {
	bot := New()
	s := "netaddr: ''\ntoken: ''"
	err := bot.Init(ctx, plugins.PluginConfig(s), logger)
	assert.NoError(t, err)
	err = bot.Close()
	assert.NoError(t, err)
}

func TestInit(t *testing.T) {
	bot := New()
	s := "netaddr: ''\ntoken: ''"
	err := bot.Init(ctx, plugins.PluginConfig(s), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)
	bot.Close()
}

func TestGetBlock(t *testing.T) {
	bot := New()
	s := "netaddr: ''\ntoken: ''"
	err := bot.Init(ctx, plugins.PluginConfig(s), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)

	blk, err := bot.GetBlock(uint64(10))
	assert.Error(t, err)
	assert.True(t, blk.Empty())
	cancel()
}
