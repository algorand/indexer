package noop

import (
	"fmt"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// `noopExporter`s will function without ever erroring. This means they will also process out of order blocks
// which may or may not be desirable for different use cases--it can hide errors in actual exporters expecting in order
// block processing.
// The `noopExporter` will maintain `Round` state according to the round of the last block it processed.
type noopExporter struct {
	round uint64
	cfg   ExporterConfig
}

var noopExporterMetadata exporters.ExporterMetadata = exporters.ExporterMetadata{
	ExpName:        "noop",
	ExpDescription: "noop exporter",
	ExpDeprecated:  false,
}

// Constructor is the ExporterConstructor implementation for the "noop" exporter
type Constructor struct{}

// New initializes a noopExporter
func (c *Constructor) New() exporters.Exporter {
	return &noopExporter{
		round: 0,
	}
}

func (exp *noopExporter) Metadata() exporters.ExporterMetadata {
	return noopExporterMetadata
}

func (exp *noopExporter) Init(cfg plugins.PluginConfig, _ *logrus.Logger) error {
	if err := yaml.Unmarshal([]byte(cfg), &exp.cfg); err != nil {
		return fmt.Errorf("init failure in unmarshalConfig: %v", err)
	}
	exp.round = exp.cfg.Round
	return nil
}

func (exp *noopExporter) Config() plugins.PluginConfig {
	ret, _ := yaml.Marshal(exp.cfg)
	return plugins.PluginConfig(ret)
}

func (exp *noopExporter) Close() error {
	return nil
}

func (exp *noopExporter) Receive(exportData data.BlockData) error {
	exp.round = exportData.Round() + 1
	return nil
}

func (exp *noopExporter) HandleGenesis(_ bookkeeping.Genesis) error {
	return nil
}

func (exp *noopExporter) Round() uint64 {
	return exp.round
}

func init() {
	exporters.RegisterExporter(noopExporterMetadata.ExpName, &Constructor{})
}
