package noop

import (
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/exporters"
)

// `noopExporter`s will function without ever erroring. This means they will also process out of order blocks
// which may or may not be desirable for different use cases--it can hide errors in actual exporters expecting in order
// block processing.
// The `noopExporter` will maintain `Round` state according to the round of the last block it processed.
type noopExporter struct {
	round uint64
	cfg   exporters.ExporterConfig
}

var NoopExporterMetadata exporters.ExporterMetadata = exporters.ExporterMetadata{
	Name:        "noop",
	Description: "noop exporter",
	Deprecated:  false,
}

// NoopConstructor is the ExporterConstructor implementation for the "noop" exporter
type NoopConstructor struct{}

func (c *NoopConstructor) New() exporters.Exporter {
	return &noopExporter{
		round: 0,
		cfg:   "",
	}
}

func (exp *noopExporter) Metadata() exporters.ExporterMetadata {
	return NoopExporterMetadata
}

func (exp *noopExporter) Connect(_ exporters.ExporterConfig) error {
	return nil
}

func (exp *noopExporter) Config() exporters.ExporterConfig {
	return exp.cfg
}

func (exp *noopExporter) Disconnect() error {
	return nil
}

func (exp *noopExporter) Receive(exportData exporters.ExportData) error {
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
	exporters.RegisterExporter(NoopExporterMetadata.Name, &NoopConstructor{})
}
