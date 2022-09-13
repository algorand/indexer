package processors

// BlockProcessorConfig configuration for a block processor
type BlockProcessorConfig struct {
	// Catchpoint to initialize the local ledger to
	Catchpoint string `yaml:"catchpoint"`

	IndexerDatadir string `yaml:"indexer-data-dir"`
	AlgodDataDir   string `yaml:"algod-data-dir"`
	AlgodToken     string `yaml:"algod-token"`
	AlgodAddr      string `yaml:"algod-addr"`
}

// FilterProcessorSubConfig is the configuration needed for each additional filter
type FilterProcessorSubConfig struct {
	// The tag of the struct to analyze
	FilterTag string `yaml:"tag"`
	// The type of expression to search for "const" or "regex"
	ExpressionType string `yaml:"expression-type"`
	// The expression to search
	Expression string `yaml:"expression"`
}

// FilterProcessorConfig configuration for the filter processor
type FilterProcessorConfig struct {
	Filters []map[string][]FilterProcessorSubConfig `yaml:"filters"`
}
