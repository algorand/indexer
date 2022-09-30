package filewriter

// Config specific to the file exporter
type Config struct {
	// BlocksDir is the path to a directory where block data should be stored.
	// The directory is created if it doesn't exist.
	BlocksDir string `yaml:"block-dir"`
	// FilenamePattern is the format used to write block files. It uses go string formatting and should accept one number for the round.
	FilenamePattern string `yaml:"filename-pattern"`
	// DropCertificate is used to remove the vote certificate from the block data before writing files.
	DropCertificate bool `yaml:"drop-certificate"`
}
