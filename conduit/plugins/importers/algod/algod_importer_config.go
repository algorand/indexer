package algodimporter

//go:generate go run ../../../../cmd/conduit-docs/main.go ../../../../conduit-docs/

//Name: conduit_importers_algod

// Config specific to the algod importer
type Config struct {
	// <code>mode</code> is the mode of operation of the algod importer.  It must be either <code>archival</code> or <code>follower</code>.
	Mode string `yaml:"mode"`
	// <code>netaddr</code> is the Algod network address. It must be either an <code>http</code> or <code>https</code> URL.
	NetAddr string `yaml:"netaddr"`
	// <code>token</code> is the Algod API endpoint token.
	Token string `yaml:"token"`
}
