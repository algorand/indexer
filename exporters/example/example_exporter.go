package example

import (
	"context"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
)

// This is our exporter object. It should store all the in memory data required to run the Exporter.
type exampleExporter struct{}

// Each Exporter should implement its own Metadata object. These fields shouldn't change at runtime so there is
// no reason to construct more than a single metadata object.
var exampleExporterMetadata exporters.ExporterMetadata = exporters.ExporterMetadata{
	ExpName:        "example",
	ExpDescription: "example exporter",
	ExpDeprecated:  false,
}

// Constructor is the ExporterConstructor implementation for an Exporter
type Constructor struct{}

// New initializes an Exporter
func (c *Constructor) New() exporters.Exporter {
	return &exampleExporter{}
}

// Metadata returns the Exporter's Metadata object
func (exp *exampleExporter) Metadata() exporters.ExporterMetadata {
	return exampleExporterMetadata
}

// Init provides the opportunity for your Exporter to initialize connections, store config variables, etc.
func (exp *exampleExporter) Init(_ context.Context, _ data.InitProvider, _ plugins.PluginConfig, _ *logrus.Logger) error {
	panic("not implemented")
}

// Config returns the unmarshaled config object
func (exp *exampleExporter) Config() plugins.PluginConfig {
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

// HandleGenesis provides the opportunity to store initial chain state
func (exp *exampleExporter) HandleGenesis(_ bookkeeping.Genesis) error {
	panic("not implemented")
}

// Round should return the round number of the next expected round that should be provided to the Exporter
func (exp *exampleExporter) Round() uint64 {
	panic("not implemented")
}

func init() {
	// In order to provide a Constructor to the exporter_factory, we register our Exporter in the init block.
	// To load this Exporter into the factory, simply import the package.
	exporters.RegisterExporter(exampleExporterMetadata.ExpName, &Constructor{})
}
