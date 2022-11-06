package noop

// ExporterConfig specific to the noop exporter
type ExporterConfig struct {
	// Optionally specify the round to start on
	Round uint64 `yaml:"round"`
}
