package filewriter

// ExporterConfig specific to the file exporter
type ExporterConfig struct {
	// BlocksDir is the path to a directory where block data should be stored.
	// The directory is created if it doesn't exist.
	BlocksDir string `yaml:"block-dir"`
	// ExcludeStateDelta indicates that the state delta file for each round should not be written.
	ExcludeStateDelta bool `yaml:"exclude-state-delta"`
}
