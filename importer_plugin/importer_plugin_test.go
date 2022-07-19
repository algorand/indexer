package importerplugin

import (
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestRound(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterImporter("", "", lg, "algod", uint64(10))
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)

	rnd := bot.Round()
	assert.Equal(t, rnd, uint64(10))
}

func TestAlgod(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterImporter("", "", lg, "algod", uint64(0))
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)

	aclient := bot.Algod()
	assert.NotEqual(t, aclient, nil)
}

func TestRegisterImporter(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterImporter("", "", lg, "algod", uint64(0))
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)
}

func TestInvalidImporterMethod(t *testing.T) {
	var lg *log.Logger
	_, err := RegisterImporter("", "", lg, "invalid", uint64(0))
	assert.NotEqual(t, err, nil)
	assert.EqualError(t, err, "invalid importer method")
}

func TestGetBlock(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterImporter("https://node-archival-mainnet.internal.aws.algodev.network", "9XxlZqyx27XDrFvV0JU1EVuxRzXJU96Peo07bK0oqslfBeNZdBHXab53D2eui72ib", lg, "algod", uint64(0))
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)

	blk, err := bot.GetBlock(uint64(10))
	assert.Equal(t, blk.Block.Round(), basics.Round(10))
	assert.Equal(t, bot.Round(), uint64(10))
	assert.NoError(t, err)
	assert.NotEqual(t, blk, nil)
}
