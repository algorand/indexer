package postgresql

import (
	"context"
	_ "embed" // used to embed config
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/data"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/conduit/plugins/exporters/postgresql/util"
	"github.com/algorand/indexer/idb"
	// Necessary to ensure the postgres implementation has been registered in the idb factory
	_ "github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/types"
	iutil "github.com/algorand/indexer/util"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// PluginName to use when configuring.
const PluginName = "postgresql"

type postgresqlExporter struct {
	round  uint64
	cfg    ExporterConfig
	db     idb.IndexerDb
	logger *logrus.Logger
	wg     sync.WaitGroup
	ctx    context.Context
	cf     context.CancelFunc
	dm     util.DataManager
}

//go:embed sample.yaml
var sampleConfig string

var metadata = conduit.Metadata{
	Name:         PluginName,
	Description:  "Exporter for writing data to a postgresql instance.",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

func (exp *postgresqlExporter) Metadata() conduit.Metadata {
	return metadata
}

func (exp *postgresqlExporter) Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error {
	exp.ctx, exp.cf = context.WithCancel(ctx)
	dbName := "postgres"
	exp.logger = logger
	if err := cfg.UnmarshalConfig(&exp.cfg); err != nil {
		return fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}
	// Inject a dummy db for unit testing
	if exp.cfg.Test {
		dbName = "dummy"
	}
	var opts idb.IndexerDbOptions
	opts.MaxConn = exp.cfg.MaxConn
	opts.ReadOnly = false

	// for some reason when ConnectionString is empty, it's automatically
	// connecting to a local instance that's running.
	// this behavior can be reproduced in TestConnectDbFailure.
	if !exp.cfg.Test && exp.cfg.ConnectionString == "" {
		return fmt.Errorf("connection string is empty for %s", dbName)
	}
	db, ready, err := idb.IndexerDbByName(dbName, exp.cfg.ConnectionString, opts, exp.logger)
	if err != nil {
		return fmt.Errorf("connect failure constructing db, %s: %v", dbName, err)
	}
	exp.db = db
	<-ready
	_, err = iutil.EnsureInitialImport(exp.db, *initProvider.GetGenesis())
	if err != nil {
		return fmt.Errorf("error importing genesis: %v", err)
	}
	dbRound, err := db.GetNextRoundToAccount()
	if err != nil {
		return fmt.Errorf("error getting next db round : %v", err)
	}
	if uint64(initProvider.NextDBRound()) != dbRound {
		return fmt.Errorf("initializing block round %d but next round to account is %d", initProvider.NextDBRound(), dbRound)
	}
	exp.round = uint64(initProvider.NextDBRound())

	// if data pruning is enabled
	if !exp.cfg.Test && exp.cfg.Delete.Rounds > 0 {
		exp.dm = util.MakeDataManager(exp.ctx, &exp.cfg.Delete, exp.db, logger)
		exp.wg.Add(1)
		go exp.dm.DeleteLoop(&exp.wg, &exp.round)
	}
	return nil
}

func (exp *postgresqlExporter) Config() string {
	ret, _ := yaml.Marshal(exp.cfg)
	return string(ret)
}

func (exp *postgresqlExporter) Close() error {
	if exp.db != nil {
		exp.db.Close()
	}

	exp.cf()
	exp.wg.Wait()
	return nil
}

func (exp *postgresqlExporter) Receive(exportData data.BlockData) error {
	if exportData.Delta == nil {
		if exportData.Round() == 0 {
			exportData.Delta = &sdk.LedgerStateDelta{}
		} else {
			return fmt.Errorf("receive got an invalid block: %#v", exportData)
		}
	}
	// Do we need to test for consensus protocol here?
	/*
		_, ok := config.Consensus[block.CurrentProtocol]
			if !ok {
				return fmt.Errorf("protocol %s not found", block.CurrentProtocol)
		}
	*/
	vb := types.ValidatedBlock{
		Block: sdk.Block{BlockHeader: exportData.BlockHeader, Payset: exportData.Payset},
		Delta: *exportData.Delta,
	}
	if err := exp.db.AddBlock(&vb); err != nil {
		return err
	}
	atomic.StoreUint64(&exp.round, exportData.Round()+1)
	return nil
}

func (exp *postgresqlExporter) unmarhshalConfig(cfg string) error {
	return yaml.Unmarshal([]byte(cfg), &exp.cfg)
}

func init() {
	exporters.Register(PluginName, exporters.ExporterConstructorFunc(func() exporters.Exporter {
		return &postgresqlExporter{}
	}))
}
