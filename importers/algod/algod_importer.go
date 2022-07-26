package algodimporter

import (
	"context"
	"fmt"
	"net/url"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const importerName = "algod"

type algodImporter struct {
	aclient *algod.Client
	logger  *logrus.Logger
	cfg     ImporterConfig
	ctx     context.Context
	cancel  context.CancelFunc
}

var algodImporterMetadata = importers.ImporterMetadata{
	ImpName:        importerName,
	ImpDescription: "Importer for fetching block from algod rest endpoint.",
	ImpDeprecated:  false,
}

// New initializes an algod importer
func New() importers.Importer {
	return &algodImporter{}
}

func (bot *algodImporter) Metadata() importers.ImporterMetadata {
	return algodImporterMetadata
}

func (bot *algodImporter) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) error {
	bot.ctx, bot.cancel = context.WithCancel(ctx)
	bot.logger = logger
	if err := bot.unmarhshalConfig(string(cfg)); err != nil {
		return fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}
	var client *algod.Client
	u, err := url.Parse(bot.cfg.NetAddr)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		bot.cfg.NetAddr = "http://" + bot.cfg.NetAddr
	}
	client, err = algod.MakeClient(bot.cfg.NetAddr, bot.cfg.Token)
	if err != nil {
		return err
	}
	bot.aclient = client
	return err
}

func (bot *algodImporter) Close() error {
	var err error
	bot.cancel()
	return err
}

func (bot *algodImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var blockbytes []byte
	var err error
	var blk data.BlockData
	aclient := bot.aclient
	blockbytes, err = aclient.BlockRaw(rnd).Do(bot.ctx)
	if err != nil {
		return blk, err
	}
	tmpBlk := new(rpcs.EncodedBlockCert)
	err = protocol.Decode(blockbytes, tmpBlk)

	blk = data.BlockData{BlockHeader: tmpBlk.Block.BlockHeader, Payset: tmpBlk.Block.Payset, Certificate: &tmpBlk.Certificate}
	return blk, err
}

func (bot *algodImporter) unmarhshalConfig(cfg string) error {
	return yaml.Unmarshal([]byte(cfg), &bot.cfg)
}
