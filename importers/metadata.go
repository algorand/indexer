package importers

import "github.com/algorand/indexer/plugins"

// ImporterMetadata returns fields relevant to identification and description of plugins.
type ImporterMetadata struct {
	ImpName        string
	ImpDescription string
	ImpDeprecated  bool
}

// Type implements the Plugin.Type interface
func (meta ImporterMetadata) Type() plugins.PluginType {
	return plugins.Importer
}

// Name implements the Plugin.Name interface
func (meta ImporterMetadata) Name() string {
	return meta.ImpName
}

// Description provides a brief description of the purpose of the Importer
func (meta *ImporterMetadata) Description() string {
	return meta.ImpDescription
}

// Deprecated is used to warn users against deprecated plugins
func (meta *ImporterMetadata) Deprecated() bool {
	return meta.ImpDeprecated
}
