package plugins

// PluginType is defined for each plugin category
type PluginType string

const (
	// Exporter PluginType
	Exporter = "exporter"

	// Processor PluginType
	Processor = "processors"

	// Importer PluginType
	Importer = "importer"
)

// PluginMetadata provides static per-plugin data
type PluginMetadata interface {
	// Type used to differentiate behaviour across plugin categories
	Type() PluginType
	// Name used to differentiate behavior across plugin implementations
	Name() string
}
