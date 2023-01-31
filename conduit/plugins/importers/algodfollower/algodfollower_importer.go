package algodfollower

import (
	"context"
	_ "embed"
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

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
)

const importerName = "algod_follower"

type algodFollowerImporter struct {
	aclient *algod.Client
	logger  *logrus.Logger
	cfg     Config
	ctx     context.Context
	cancel  context.CancelFunc
}

func (af *algodFollowerImporter) OnComplete(input data.BlockData) error {
	_, err := af.aclient.SetSyncRound(input.Round() + 1).Do(af.ctx)
	return err
}

//go:embed sample.yaml
var sampleConfig string

var algodFollowerImporterMetadata = conduit.Metadata{
	Name:         importerName,
	Description:  "Importer for fetching blocks from an algod REST API using sync round and ledger delta algod calls.",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

// New initializes an algod importer
func New() importers.Importer {
	return &algodFollowerImporter{}
}

func (af *algodFollowerImporter) Metadata() conduit.Metadata {
	return algodFollowerImporterMetadata
}

// package-wide init function
func init() {
	importers.Register(importerName, importers.ImporterConstructorFunc(func() importers.Importer {
		return &algodFollowerImporter{}
	}))
}

func (af *algodFollowerImporter) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*sdk.Genesis, error) {
	af.ctx, af.cancel = context.WithCancel(ctx)
	af.logger = logger
	err := cfg.UnmarshalConfig(&af.cfg)
	if err != nil {
		return nil, fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}
	var client *algod.Client
	u, err := url.Parse(af.cfg.NetAddr)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		af.cfg.NetAddr = "http://" + af.cfg.NetAddr
		af.logger.Infof("Algod Importer added http prefix to NetAddr: %s", af.cfg.NetAddr)
	}
	client, err = algod.MakeClient(af.cfg.NetAddr, af.cfg.Token)
	if err != nil {
		return nil, err
	}
	af.aclient = client

	genesisResponse, err := client.GetGenesis().Do(ctx)
	if err != nil {
		return nil, err
	}

	genesis := sdk.Genesis{}

	err = json.Decode([]byte(genesisResponse), &genesis)
	if err != nil {
		return nil, err
	}

	return &genesis, err
}

func (af *algodFollowerImporter) Config() string {
	s, _ := yaml.Marshal(af.cfg)
	return string(s)
}

func (af *algodFollowerImporter) Close() error {
	if af.cancel != nil {
		af.cancel()
	}
	return nil
}

func (af *algodFollowerImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var blockbytes []byte
	var err error
	var blk data.BlockData

	for retries := 0; retries < 3; retries++ {
		// nextRound - 1 because the endpoint waits until the subsequent block is committed to return
		_, err = af.aclient.StatusAfterBlock(rnd - 1).Do(af.ctx)
		if err != nil {
			// If context has expired.
			if af.ctx.Err() != nil {
				return blk, fmt.Errorf("GetBlock ctx error: %w", err)
			}
			af.logger.Errorf(
				"r=%d error getting status %d", retries, rnd)
			continue
		}
		start := time.Now()
		blockbytes, err = af.aclient.BlockRaw(rnd).Do(af.ctx)
		dt := time.Since(start)
		GetAlgodRawBlockTimeSeconds.Observe(dt.Seconds())
		if err != nil {
			return blk, err
		}
		tmpBlk := new(rpcs.EncodedBlockCert)
		err = protocol.Decode(blockbytes, tmpBlk)
		if err != nil {
			return blk, err
		}

		// We aren't going to do anything with the new delta until we get everything
		// else converted over
		// Round 0 has no delta associated with it
		if rnd != 0 {
			_, err = af.aclient.GetLedgerStateDelta(rnd).Do(af.ctx)
			if err != nil {
				return blk, err
			}
		}

		blk = data.BlockData{
			BlockHeader: tmpBlk.Block.BlockHeader,
			Payset:      tmpBlk.Block.Payset,
			Certificate: &tmpBlk.Certificate,
		}
		return blk, err
	}
	af.logger.Error("GetBlock finished retries without fetching a block.  Check that the indexer is set to start at a round that the current algod node can handle")
	return blk, fmt.Errorf("finished retries without fetching a block.  Check that the indexer is set to start at a round that the current algod node can handle")
}

func (af *algodFollowerImporter) ProvideMetrics() []prometheus.Collector {
	return []prometheus.Collector{
		GetAlgodRawBlockTimeSeconds,
	}
}
