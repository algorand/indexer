package exporters

import (
	"github.com/algorand/indexer/plugins"
)

// ExporterMetadata returns fields relevant to identification and description of plugins.
type ExporterMetadata struct {
	ExpName        string
	ExpDescription string
	ExpDeprecated  bool
}

// Type implements the Plugin.Type interface
func (meta ExporterMetadata) Type() plugins.PluginType {
	return plugins.Exporter
}

// Name implements the Plugin.Name interface
func (meta ExporterMetadata) Name() string {
	return meta.ExpName
}

// Description provides a brief description of the purpose of the Exporter
func (meta *ExporterMetadata) Description() string {
	return meta.ExpDescription
}

// Deprecated is used to warn users against deprecated plugins
func (meta *ExporterMetadata) Deprecated() bool {
	return meta.ExpDeprecated
}
