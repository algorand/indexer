package algodimporter

import (
	"context"
	"strings"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/exporters"
	importerplugin "github.com/algorand/indexer/importer_plugin"
	log "github.com/sirupsen/logrus"
)

type algodImporter struct {
	aclient *algod.Client
	log     *log.Logger
}

// Getblock takes the round number as an input and downloads that specific block, updates the 'lastRound' local variable and returns an exporters.BlockExportData struct
func (bot *algodImporter) GetBlock(rnd uint64) (blk *exporters.BlockExportData, err error) {
	var blockbytes []byte
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	aclient := bot.aclient
	blockbytes, err = aclient.BlockRaw(rnd).Do(ctx)
	if err != nil {
		return nil, err
	}
	tmpBlk := new(rpcs.EncodedBlockCert)
	err = protocol.Decode(blockbytes, tmpBlk)

	blk = &exporters.BlockExportData{Block: tmpBlk.Block, Certificate: tmpBlk.Certificate}
	return
}

// RegisterAlgodImporter will be called during initialization by the user/processor_plugin, to initialize necessary connections used for initializing algod client
func RegisterAlgodImporter(netaddr, token string, log *log.Logger) (bot importerplugin.Importer, err error) {
	var client *algod.Client
	if !strings.HasPrefix(netaddr, "http") {
		netaddr = "http://" + netaddr
	}
	client, err = algod.MakeClient(netaddr, token)
	if err != nil {
		return
	}
	bot = &algodImporter{aclient: client, log: log}
	return
}
