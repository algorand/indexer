package processors

import "github.com/algorand/indexer/plugins"

// ProcessorMetadata returns fields relevant to identification and description of plugins.
type ProcessorMetadata struct {
	ImplementationName        string
	ImplementationDescription string
	ImplementationDeprecated  bool
}

// Type implements the Plugin.Type interface
func (meta ProcessorMetadata) Type() plugins.PluginType {
	return plugins.Processor
}

// Name implements the Plugin.Name interface
func (meta ProcessorMetadata) Name() string {
	return meta.ImplementationName
}

// Description provides a brief description of the purpose of the Importer
func (meta *ProcessorMetadata) Description() string {
	return meta.ImplementationDescription
}

// Deprecated is used to warn users against deprecated plugins
func (meta *ProcessorMetadata) Deprecated() bool {
	return meta.ImplementationDeprecated
}
