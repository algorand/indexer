package filewriter

//go:generate conduit-docs ../../../../conduit-docs/

//Name: conduit_exporters_filewriter

// Config specific to the file exporter
type Config struct {
	/* <code>blocks-dir</code> is an optional path to a directory where block data should be
	stored.<br/>
	The directory is created if it doesn't exist.<br/>
	If no directory is provided the default plugin data directory is used.
	*/
	BlocksDir string `yaml:"block-dir"`
	/* <code>filename-pattern</code> is the format used to write block files. It uses go
	string formatting and should accept one number for the round.<br/>
	If the file has a '.gz' extension, blocks will be gzipped.
	Default:

		"%[1]d_block.json"
	*/
	FilenamePattern string `yaml:"filename-pattern"`
	// <code>drop-certificate</code> is used to remove the vote certificate from the block data before writing files.
	DropCertificate bool `yaml:"drop-certificate"`

	// TODO: compression level - Default, Fastest, Best compression, etc

	// TODO: How to avoid having millions of files in a directory?
	//       Write batches of blocks to a single file?
	//       Tree of directories
}
