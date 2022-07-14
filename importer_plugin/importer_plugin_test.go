package importerPlugin

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestRegisterImporter(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterImporter("", "", lg, true, uint64(0))
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)
}

func TestGetBlock(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterImporter("", "", lg, true, uint64(0))
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)

	blk, err := bot.GetBlock(uint64(10))
	assert.Equal(t, bot.Round(), uint64(10))
	assert.NoError(t, err)
	assert.NotEqual(t, blk, nil)
}

func TestRound(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterImporter("", "", lg, true, uint64(10))
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)

	rnd := bot.Round()
	assert.Equal(t, rnd, uint64(10))
}

func TestAlgod(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterImporter("", "", lg, true, uint64(0))
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)

	aclient := bot.Algod()
	assert.NotEqual(t, aclient, nil)
}
