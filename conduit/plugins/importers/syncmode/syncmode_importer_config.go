package syncmode

// Config specific to the sync mode importer
type Config struct {
	// Algod netaddr string
	NetAddr string `yaml:"netaddr"`
	// Algod rest endpoint token
	Token string `yaml:"token"`
}
