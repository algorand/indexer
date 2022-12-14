package algodimporter

//go:generate conduit-docs ../../../../conduit-docs/

// Config specific to the algod importer
type Config struct {
	// Algod netaddr string
	NetAddr string `yaml:"netaddr"`
	// Algod rest endpoint token
	Token string `yaml:"token"`
}
