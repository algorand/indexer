package blockprocessor

//go:generate conduit-docs ../../../../conduit-docs/

// Config configuration for a block processor
type Config struct {
	// Catchpoint to initialize the local ledger to
	Catchpoint string `yaml:"catchpoint"`

	// Ledger directory
	LedgerDir string `yaml:"ledger-dir"`
	// Algod data directory
	AlgodDataDir string `yaml:"algod-data-dir"`
	// Algod token
	AlgodToken string `yaml:"algod-token"`
	// Algod address
	AlgodAddr string `yaml:"algod-addr"`
}
