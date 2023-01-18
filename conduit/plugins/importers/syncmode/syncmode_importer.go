package syncmode

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
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
)

const importerName = "sync-algod"

type syncModeImporter struct {
	aclient *algod.Client
	logger  *logrus.Logger
	cfg     Config
	ctx     context.Context
	cancel  context.CancelFunc
}

func (sm *syncModeImporter) OnComplete(input data.BlockData) error {
	_, err := sm.aclient.SetSyncRound(input.Round() + 1).Do(sm.ctx)
	return err
}

//go:embed sample.yaml
var sampleConfig string

var syncModeImporterMetadata = conduit.Metadata{
	Name:         importerName,
	Description:  "Importer for fetching blocks from an algod REST API.",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

// New initializes an algod importer
func New() importers.Importer {
	return &syncModeImporter{}
}

func (sm *syncModeImporter) Metadata() conduit.Metadata {
	return syncModeImporterMetadata
}

// package-wide init function
func init() {
	importers.Register(importerName, importers.ImporterConstructorFunc(func() importers.Importer {
		return &syncModeImporter{}
	}))
}

func (sm *syncModeImporter) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*sdk.Genesis, error) {
	sm.ctx, sm.cancel = context.WithCancel(ctx)
	sm.logger = logger
	err := cfg.UnmarshalConfig(&sm.cfg)
	if err != nil {
		return nil, fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}
	var client *algod.Client
	u, err := url.Parse(sm.cfg.NetAddr)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		sm.cfg.NetAddr = "http://" + sm.cfg.NetAddr
		sm.logger.Infof("Algod Importer added http prefix to NetAddr: %s", sm.cfg.NetAddr)
	}
	client, err = algod.MakeClient(sm.cfg.NetAddr, sm.cfg.Token)
	if err != nil {
		return nil, err
	}
	sm.aclient = client

	genesisResponse, err := client.GetGenesis().Do(ctx)
	if err != nil {
		return nil, err
	}

	genesis := sdk.Genesis{}

	err = protocol.DecodeJSON([]byte(genesisResponse), &genesis)
	if err != nil {
		return nil, err
	}

	return &genesis, err
}

func (sm *syncModeImporter) Config() string {
	s, _ := yaml.Marshal(sm.cfg)
	return string(s)
}

func (sm *syncModeImporter) Close() error {
	if sm.cancel != nil {
		sm.cancel()
	}
	return nil
}

func (sm *syncModeImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var blockbytes []byte
	var err error
	var blk data.BlockData

	for retries := 0; retries < 3; retries++ {
		// nextRound - 1 because the endpoint waits until the subsequent block is committed to return
		_, err = sm.aclient.StatusAfterBlock(rnd - 1).Do(sm.ctx)
		if err != nil {
			// If context has expired.
			if sm.ctx.Err() != nil {
				return blk, fmt.Errorf("GetBlock ctx error: %w", err)
			}
			sm.logger.Errorf(
				"r=%d error getting status %d", retries, rnd)
			continue
		}
		start := time.Now()
		blockbytes, err = sm.aclient.BlockRaw(rnd).Do(sm.ctx)
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
		_, err = sm.aclient.GetLedgerStateDelta(rnd).Do(sm.ctx)
		if err != nil {
			return blk, err
		}

		blk = data.BlockData{
			BlockHeader: tmpBlk.Block.BlockHeader,
			Payset:      tmpBlk.Block.Payset,
			Certificate: &tmpBlk.Certificate,
		}
		return blk, err
	}
	sm.logger.Error("GetBlock finished retries without fetching a block.")
	return blk, fmt.Errorf("finished retries without fetching a block")
}

func (sm *syncModeImporter) ProvideMetrics() []prometheus.Collector {
	return []prometheus.Collector{
		GetAlgodRawBlockTimeSeconds,
	}
}
