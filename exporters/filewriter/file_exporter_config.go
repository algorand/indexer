package filewriter

// ExporterConfig specific to the file exporter
type ExporterConfig struct {
	// round to start at
	Round uint64 `yaml:"round"`
	// full file path to existing block file.
	// Create if file doesn't exist
	BlockFilepath string `yaml:"path"`
	// full file path to a configuration file.
	ConfigFilePath string `yaml:"configs"`
}
