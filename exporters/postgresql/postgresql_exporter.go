package postgresql

import (
	"context"
	"fmt"

	"github.com/algorand/indexer/importer"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/idb"
	// Necessary to ensure the postgres implementation has been registered in the idb factory
	_ "github.com/algorand/indexer/idb/postgres"
	"github.com/algorand/indexer/plugins"
)

const exporterName = "postgresql"

type postgresqlExporter struct {
	round  uint64
	cfg    ExporterConfig
	db     idb.IndexerDb
	logger *logrus.Logger
}

var postgresqlExporterMetadata = exporters.ExporterMetadata{
	ExpName:        exporterName,
	ExpDescription: "Exporter for writing data to a postgresql instance.",
	ExpDeprecated:  false,
}

// Constructor is the ExporterConstructor implementation for the "postgresql" exporter
type Constructor struct{}

// New initializes a postgresqlExporter
func (c *Constructor) New() exporters.Exporter {
	return &postgresqlExporter{}
}

func (exp *postgresqlExporter) Metadata() exporters.ExporterMetadata {
	return postgresqlExporterMetadata
}

func (exp *postgresqlExporter) Init(_ context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error {
	dbName := "postgres"
	exp.logger = logger
	if err := exp.unmarhshalConfig(string(cfg)); err != nil {
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
	_, err = importer.EnsureInitialImport(exp.db, *initProvider.GetGenesis())
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
	return err
}

func (exp *postgresqlExporter) Config() plugins.PluginConfig {
	ret, _ := yaml.Marshal(exp.cfg)
	return plugins.PluginConfig(ret)
}

func (exp *postgresqlExporter) Close() error {
	if exp.db != nil {
		exp.db.Close()
	}
	return nil
}

func (exp *postgresqlExporter) Receive(exportData data.BlockData) error {
	if exportData.Delta == nil {
		return fmt.Errorf("receive got an invalid block: %#v", exportData)
	}
	// Do we need to test for consensus protocol here?
	/*
		_, ok := config.Consensus[block.CurrentProtocol]
			if !ok {
				return fmt.Errorf("protocol %s not found", block.CurrentProtocol)
		}
	*/
	var delta ledgercore.StateDelta
	if exportData.Delta != nil {
		delta = *exportData.Delta
	}
	vb := ledgercore.MakeValidatedBlock(
		bookkeeping.Block{
			BlockHeader: exportData.BlockHeader,
			Payset:      exportData.Payset,
		},
		delta)
	if err := exp.db.AddBlock(&vb); err != nil {
		return err
	}
	exp.round = exportData.Round() + 1
	return nil
}

func (exp *postgresqlExporter) unmarhshalConfig(cfg string) error {
	return yaml.Unmarshal([]byte(cfg), &exp.cfg)
}

func init() {
	exporters.RegisterExporter(exporterName, &Constructor{})
}
