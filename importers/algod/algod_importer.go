package algodimporter

import (
	"context"
	"fmt"
	"github.com/algorand/go-algorand/data/bookkeeping"
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

// Constructor is the Constructor implementation for the "algod" importer
type Constructor struct{}

// New initializes a blockProcessorConstructor
func (c *Constructor) New() importers.Importer {
	return &algodImporter{}
}

func (algodImp *algodImporter) Metadata() importers.ImporterMetadata {
	return algodImporterMetadata
}

// package-wide init function
func init() {
	importers.RegisterImporter(importerName, &Constructor{})
}

func (algodImp *algodImporter) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*bookkeeping.Genesis, error) {
	algodImp.ctx, algodImp.cancel = context.WithCancel(ctx)
	algodImp.logger = logger
	if err := algodImp.unmarhshalConfig(string(cfg)); err != nil {
		return nil, fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}
	var client *algod.Client
	u, err := url.Parse(algodImp.cfg.NetAddr)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		algodImp.cfg.NetAddr = "http://" + algodImp.cfg.NetAddr
		algodImp.logger.Infof("Algod Importer added http prefix to NetAddr: %s", algodImp.cfg.NetAddr)
	}
	client, err = algod.MakeClient(algodImp.cfg.NetAddr, algodImp.cfg.Token)
	if err != nil {
		return nil, err
	}
	algodImp.aclient = client

	genesisResponse, err := client.GetGenesis().Do(ctx)
	if err != nil {
		return nil, err
	}

	genesis := bookkeeping.Genesis{}

	err = protocol.DecodeJSON([]byte(genesisResponse), &genesis)
	if err != nil {
		return nil, err
	}

	return &genesis, err
}

func (algodImp *algodImporter) Config() plugins.PluginConfig {
	s, _ := yaml.Marshal(algodImp.cfg)
	return plugins.PluginConfig(s)
}

func (algodImp *algodImporter) Close() error {
	algodImp.cancel()
	return nil
}

func (algodImp *algodImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var blockbytes []byte
	var err error
	var blk data.BlockData

	for retries := 0; retries < 3; retries++ {
		// nextRound - 1 because the endpoint waits until the subsequent block is committed to return
		_, err = algodImp.aclient.StatusAfterBlock(rnd - 1).Do(algodImp.ctx)
		if err != nil {
			// If context has expired.
			if algodImp.ctx.Err() != nil {
				return blk, fmt.Errorf("GetBlock ctx error: %w", err)
			}
			algodImp.logger.Errorf(
				"r=%d error getting status %d", retries, rnd)
			continue
		}
		blockbytes, err = algodImp.aclient.BlockRaw(rnd).Do(algodImp.ctx)
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
	algodImp.logger.Error("GetBlock finished retries without fetching a block.")
	return blk, fmt.Errorf("finished retries without fetching a block")
}

func (algodImp *algodImporter) unmarhshalConfig(cfg string) error {
	return yaml.Unmarshal([]byte(cfg), &algodImp.cfg)
}
