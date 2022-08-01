package algodimporter

// ImporterConfig specific to the algod importer
type ImporterConfig struct {
	// Algod netaddr string
	NetAddr string `yaml:"netaddr"`
	// Algod rest endpoint token
	Token string `yaml:"token"`
}
