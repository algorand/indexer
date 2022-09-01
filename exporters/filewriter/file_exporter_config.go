package filewriter

// ExporterConfig specific to the file exporter
type ExporterConfig struct {
	Round         uint64 `yaml:"round"`
	BlockFilepath string
}
