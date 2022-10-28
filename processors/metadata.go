package processors

import "github.com/algorand/indexer/plugins"

// ProcessorMetadata returns fields relevant to identification and description of plugins.
type ProcessorMetadata struct {
	name         string
	description  string
	deprecated   bool
	sampleConfig string
}

// MakeProcessorMetadata creates a processor metadata object
func MakeProcessorMetadata(name string, description string, deprecated bool, sampleConfig string) ProcessorMetadata {
	return ProcessorMetadata{name: name, description: description, deprecated: deprecated, sampleConfig: sampleConfig}
}

// Type implements the Plugin.Type interface
func (meta ProcessorMetadata) Type() plugins.PluginType {
	return plugins.Processor
}

// Name implements the Plugin.Name interface
func (meta ProcessorMetadata) Name() string {
	return meta.name
}

// Description provides a brief description of the purpose of the Importer
func (meta *ProcessorMetadata) Description() string {
	return meta.description
}

// Deprecated is used to warn users against deprecated plugins
func (meta *ProcessorMetadata) Deprecated() bool {
	return meta.deprecated
}

// SampleConfig returns the sample config of the plugin
func (meta *ProcessorMetadata) SampleConfig() string {
	return meta.sampleConfig
}
