package conduit

// Metadata returns fields relevant to identification and description of plugins.
type Metadata struct {
	Name         string
	Description  string
	Deprecated   bool
	SampleConfig string
}

// PluginMetadata is the common interface for providing plugin metadata.
type PluginMetadata interface {
	// Metadata associated with the plugin.
	Metadata() Metadata
}
