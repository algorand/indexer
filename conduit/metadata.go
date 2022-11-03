package conduit

// Metadata returns fields relevant to identification and description of plugins.
type Metadata struct {
	Name         string
	Description  string
	Deprecated   bool
	SampleConfig string
}

type PluginMetadata interface {
	// Metadata associated with each Exporter.
	Metadata() Metadata
}
