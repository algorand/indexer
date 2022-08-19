package networkimporter

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/algorand/go-algorand/catchup"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/network"

	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const importerName = "network"

type networkImporter struct {
	logger         *logrus.Logger
	cfg            ImporterConfig
	ctx            context.Context
	cancel         context.CancelFunc
	networkFetcher *catchup.NetworkFetcher
}

var networkImporterMetadata = importers.ImporterMetadata{
	ImpName:        importerName,
	ImpDescription: "Importer for fetching blocks directly from the network.",
	ImpDeprecated:  false,
}

// New initializes an network importer
func New() importers.Importer {
	return &networkImporter{}
}

// Constructor is the Constructor implementation for the "network" importer
type Constructor struct{}

// New initializes a blockProcessorConstructor
func (c *Constructor) New() importers.Importer {
	return &networkImporter{}
}

func (networkImp *networkImporter) Metadata() importers.ImporterMetadata {
	return networkImporterMetadata
}

// package-wide init function
func init() {
	importers.RegisterImporter(importerName, &Constructor{})
}

func (networkImp *networkImporter) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*bookkeeping.Genesis, error) {
	networkImp.ctx, networkImp.cancel = context.WithCancel(ctx)
	networkImp.logger = logger
	if err := networkImp.unmarhshalConfig(string(cfg)); err != nil {
		return nil, fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}

	genesisText, err := ioutil.ReadFile(networkImp.cfg.GenesisPath)
	if err != nil {
		return nil, err
	}
	var genesis bookkeeping.Genesis
	err = protocol.DecodeJSON(genesisText, &genesis)
	if err != nil {
		return nil, err
	}

	localCfg, err := config.LoadConfigFromDisk(networkImp.cfg.ConfigPath)
	if err != nil {
		return nil, err
	}
	// disabling cert authentication
	localCfg.CatchupBlockValidateMode = 1

	// start a node
	net, err := network.NewWebsocketNetwork(logging.NewWrappedLogger(logger), localCfg, nil, genesis.ID(), genesis.Network, nil)
	if err != nil {
		return nil, err
	}
	net.Start()
	localCfg.NetAddress, _ = net.Address()
	net.RequestConnectOutgoing(false, ctx.Done())

	// create fetcher to fetch blocks from network
	networkImp.networkFetcher = catchup.MakeNetworkFetcher(logging.NewWrappedLogger(logger), net, localCfg, nil, false)

	return &genesis, err
}

func (networkImp *networkImporter) Config() plugins.PluginConfig {
	s, _ := yaml.Marshal(networkImp.cfg)
	return plugins.PluginConfig(s)
}

func (networkImp *networkImporter) Close() error {
	networkImp.cancel()
	return nil
}

func (networkImp *networkImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var blk data.BlockData
	block, cert, _, err := networkImp.networkFetcher.FetchBlock(networkImp.ctx, basics.Round(rnd))
	if err != nil {
		return blk, err
	}
	blk.BlockHeader = block.BlockHeader
	blk.Payset = block.Payset
	blk.Certificate = cert
	return blk, nil
}

func (networkImp *networkImporter) unmarhshalConfig(cfg string) error {
	return yaml.Unmarshal([]byte(cfg), &networkImp.cfg)
}
