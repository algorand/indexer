package fileimporter

//go:generate conduit-docs ../../../../conduit-docs/

import "time"

//Name: conduit_importers_filereader

// Config specific to the file importer
type Config struct {
	// <code>block-dir</code> is the path to a directory where block data is stored.
	BlocksDir string `yaml:"block-dir"`
	/* <code>retry-duration</code> controls the delay between checks when the importer has caught up and is waiting for new blocks to appear.<br/>
	The input duration will be interpreted in nanoseconds.
	*/
	RetryDuration time.Duration `yaml:"retry-duration"`
	/* <code>retry-count</code> controls the number of times to check for a missing block
	before generating an error. The retry count and retry duration should
	be configured according the expected round time.
	*/
	RetryCount uint64 `yaml:"retry-count"`
	/* <code>filename-pattern</code> is the format used to find block files. It uses go string formatting and should accept one number for the round.
	The default pattern is

	"%[1]d_block.json"
	*/
	FilenamePattern string `yaml:"filename-pattern"`

	// TODO: Option to delete files after processing them
}
