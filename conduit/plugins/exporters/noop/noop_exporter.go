package noop

import (
	"context"
	_ "embed" // used to embed config
	"fmt"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/data"
)

var implementationName = "noop"

// `noopExporter`s will function without ever erroring. This means they will also process out of order blocks
// which may or may not be desirable for different use cases--it can hide errors in actual exporters expecting in order
// block processing.
// The `noopExporter` will maintain `Round` state according to the round of the last block it processed.
type noopExporter struct {
	round uint64
	cfg   ExporterConfig
}

//go:embed sample.yaml
var sampleConfig string

var metadata = conduit.Metadata{
	Name:         implementationName,
	Description:  "noop exporter",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

func (exp *noopExporter) Metadata() conduit.Metadata {
	return metadata
}

func (exp *noopExporter) Init(_ context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, _ *logrus.Logger) error {
	if err := cfg.UnmarshalConfig(&exp.cfg); err != nil {
		return fmt.Errorf("init failure in unmarshalConfig: %v", err)
	}
	exp.round = exp.cfg.Round
	return nil
}

func (exp *noopExporter) Config() string {
	ret, _ := yaml.Marshal(exp.cfg)
	return string(ret)
}

func (exp *noopExporter) Close() error {
	return nil
}

func (exp *noopExporter) Receive(exportData data.BlockData) error {
	exp.round = exportData.Round() + 1
	return nil
}

func (exp *noopExporter) Round() uint64 {
	return exp.round
}

func init() {
	exporters.Register(implementationName, exporters.ExporterConstructorFunc(func() exporters.Exporter {
		return &noopExporter{}
	}))
}
