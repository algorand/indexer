package fileimporter

import "time"

// Config specific to the file importer
type Config struct {
	// BlocksDir is the path to a directory where block data should be stored.
	// The directory is created if it doesn't exist.
	BlocksDir string `yaml:"block-dir"`
	// RetryDuration controls the delay between checks when the importer has
	// caught up and is waiting for new blocks to appear.
	RetryDuration time.Duration `yaml:"retry-duration"`
	// RetryCount controls the number of times to check for a missing block
	// before generating an error. The retry count and retry duration should
	// be configured according the expected round time.
	RetryCount uint64 `yaml:"retry-count"`
	// FilenamePattern is the format used to find block files. It uses go string formatting and should accept one number for the round.
	FilenamePattern string `yaml:"filename-pattern"`

	// TODO: Option to delete files after processing them
}
