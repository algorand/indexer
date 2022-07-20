package algodimporter

import (
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestRegisterAlgodImporter(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterAlgodImporter("", "", lg)
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)
}

func TestGetBlock(t *testing.T) {
	var lg *log.Logger
	bot, err := RegisterAlgodImporter("https://node-archival-mainnet.internal.aws.algodev.network", "9XxlZqyx27XDrFvV0JU1EVuxRzXJU96Peo07bK0oqslfBeNZdBHXab53D2eui72ib", lg)
	assert.NoError(t, err)
	assert.NotEqual(t, bot, nil)

	blk, err := bot.GetBlock(uint64(10))
	assert.Equal(t, blk.Block.Round(), basics.Round(10))
	assert.NoError(t, err)
	assert.NotEqual(t, blk, nil)
}
