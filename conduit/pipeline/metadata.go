package pipeline

// TODO: move this to plugins package after reorganizing plugin packages.

import (
	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/conduit/plugins/importers"
	"github.com/algorand/indexer/conduit/plugins/processors"
)

// AllMetadata gets a slice with metadata from all registered plugins.
func AllMetadata() (results []conduit.Metadata) {
	results = append(results, ImporterMetadata()...)
	results = append(results, ProcessorMetadata()...)
	results = append(results, ExporterMetadata()...)
	return
}

// ImporterMetadata gets a slice with metadata for all importers.Importer plugins.
func ImporterMetadata() (results []conduit.Metadata) {
	for _, constructor := range importers.Importers {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
}

// ProcessorMetadata gets a slice with metadata for all importers.Processor plugins.
func ProcessorMetadata() (results []conduit.Metadata) {
	for _, constructor := range processors.Processors {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
}

// ExporterMetadata gets a slice with metadata for all importers.Exporter plugins.
func ExporterMetadata() (results []conduit.Metadata) {
	for _, constructor := range exporters.Exporters {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
}
