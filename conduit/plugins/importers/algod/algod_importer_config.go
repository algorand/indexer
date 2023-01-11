package algodimporter

//go:generate conduit-docs ../../../../conduit-docs/

//Name: conduit_importers_algod

// Config specific to the algod importer
type Config struct {
	// <code>netaddr</code> is the Algod network address. It must be either an <code>http</code> or <code>https</code> URL.
	NetAddr string `yaml:"netaddr"`
	// <code>token</code> is the Algod API endpoint token.
	Token string `yaml:"token"`
}
