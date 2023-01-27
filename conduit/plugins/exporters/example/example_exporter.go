package example

import (
	"context"
	_ "embed" // used to embed config

	"github.com/sirupsen/logrus"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/data"
)

// This is our exporter object. It should store all the in memory data required to run the Exporter.
type exampleExporter struct{}

//go:embed sample.yaml
var sampleConfig string

// Each Exporter should implement its own Metadata object. These fields shouldn't change at runtime so there is
// no reason to construct more than a single metadata object.
var metadata = conduit.Metadata{
	Name:         "example",
	Description:  "example exporter",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

// Metadata returns the Exporter's Metadata object
func (exp *exampleExporter) Metadata() conduit.Metadata {
	return metadata
}

// Init provides the opportunity for your Exporter to initialize connections, store config variables, etc.
func (exp *exampleExporter) Init(_ context.Context, _ data.InitProvider, _ plugins.PluginConfig, _ *logrus.Logger) error {
	panic("not implemented")
}

// Config returns the unmarshaled config object
func (exp *exampleExporter) Config() string {
	panic("not implemented")
}

// Close provides the opportunity to close connections, flush buffers, etc. when the process is terminating
func (exp *exampleExporter) Close() error {
	panic("not implemented")
}

// Receive is the main handler function for blocks
func (exp *exampleExporter) Receive(exportData data.BlockData) error {
	panic("not implemented")
}

// Round should return the round number of the next expected round that should be provided to the Exporter
func (exp *exampleExporter) Round() uint64 {
	panic("not implemented")
}

func init() {
	// In order to provide a Constructor to the exporter_factory, we register our Exporter in the init block.
	// To load this Exporter into the factory, simply import the package.
	exporters.Register(metadata.Name, exporters.ExporterConstructorFunc(func() exporters.Exporter {
		return &exampleExporter{}
	}))
}
