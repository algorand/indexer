package filewriter

// ExporterConfig specific to the file exporter
type ExporterConfig struct {
	// full file path to a directory
	// where the block data should be stored.
	// Create if directory doesn't exist
	BlocksDir string `yaml:"block-dir"`
}
