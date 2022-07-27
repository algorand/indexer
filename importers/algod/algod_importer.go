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

func (algodImp *algodImporter) Metadata() importers.ImporterMetadata {
	return algodImporterMetadata
}

func (algodImp *algodImporter) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) error {
	algodImp.ctx, algodImp.cancel = context.WithCancel(ctx)
	algodImp.logger = logger
	if err := algodImp.unmarhshalConfig(string(cfg)); err != nil {
		return fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}
	var client *algod.Client
	u, err := url.Parse(algodImp.cfg.NetAddr)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		algodImp.cfg.NetAddr = "http://" + algodImp.cfg.NetAddr
	}
	client, err = algod.MakeClient(algodImp.cfg.NetAddr, algodImp.cfg.Token)
	if err != nil {
		return err
	}
	algodImp.aclient = client
	return err
}

func (algodImp *algodImporter) Config() plugins.PluginConfig {
	s, _ := yaml.Marshal(algodImp.cfg)
	return plugins.PluginConfig(s)
}

func (algodImp *algodImporter) Close() error {
	var err error
	algodImp.cancel()
	return err
}

func (algodImp *algodImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var blockbytes []byte
	var err error
	var blk data.BlockData
	aclient := algodImp.aclient
	blockbytes, err = aclient.BlockRaw(rnd).Do(algodImp.ctx)
	if err != nil {
		return blk, err
	}
	tmpBlk := new(rpcs.EncodedBlockCert)
	err = protocol.Decode(blockbytes, tmpBlk)

	blk = data.BlockData{
		BlockHeader: tmpBlk.Block.BlockHeader,
		Payset:      tmpBlk.Block.Payset,
		Certificate: &tmpBlk.Certificate,
	}
	return blk, err
}

func (algodImp *algodImporter) unmarhshalConfig(cfg string) error {
	return yaml.Unmarshal([]byte(cfg), &algodImp.cfg)
}
