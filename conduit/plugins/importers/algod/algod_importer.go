package algodimporter

import (
	"context"
	_ "embed" // used to embed config
	"fmt"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/importers"
	"github.com/algorand/indexer/data"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
)

const importerName = "algod"

type algodImporter struct {
	aclient *algod.Client
	logger  *logrus.Logger
	cfg     Config
	ctx     context.Context
	cancel  context.CancelFunc
}

//go:embed sample.yaml
var sampleConfig string

var algodImporterMetadata = conduit.Metadata{
	Name:         importerName,
	Description:  "Importer for fetching blocks from an algod REST API.",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

// New initializes an algod importer
func New() importers.Importer {
	return &algodImporter{}
}

func (algodImp *algodImporter) Metadata() conduit.Metadata {
	return algodImporterMetadata
}

// package-wide init function
func init() {
	importers.Register(importerName, importers.ImporterConstructorFunc(func() importers.Importer {
		return &algodImporter{}
	}))
}

func (algodImp *algodImporter) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*bookkeeping.Genesis, error) {
	algodImp.ctx, algodImp.cancel = context.WithCancel(ctx)
	algodImp.logger = logger
	var err error
	algodImp.cfg, err = unmarshalConfig(string(cfg))
	if err != nil {
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
	if algodImp.cancel != nil {
		algodImp.cancel()
	}
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
		start := time.Now()
		blockbytes, err = algodImp.aclient.BlockRaw(rnd).Do(algodImp.ctx)
		dt := time.Since(start)
		GetAlgodRawBlockTimeSeconds.Observe(dt.Seconds())
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

func (algodImp *algodImporter) ProvideMetrics() []prometheus.Collector {
	return []prometheus.Collector{
		GetAlgodRawBlockTimeSeconds,
	}
}

func unmarshalConfig(cfg string) (config Config, err error) {
	err = yaml.Unmarshal([]byte(cfg), &config)
	return
}
