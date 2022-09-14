package blockprocessor

// Config configuration for a block processor
type Config struct {
	// Catchpoint to initialize the local ledger to
	Catchpoint string `yaml:"catchpoint"`

	IndexerDatadir string `yaml:"indexer-data-dir"`
	AlgodDataDir   string `yaml:"algod-data-dir"`
	AlgodToken     string `yaml:"algod-token"`
	AlgodAddr      string `yaml:"algod-addr"`
}
