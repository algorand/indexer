package conduit

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// DefaultConfigBaseName is the default conduit configuration filename without the extension.
var DefaultConfigBaseName = "conduit"

// DefaultConfigName is the default conduit configuration filename.
var DefaultConfigName = fmt.Sprintf("%s.yml", DefaultConfigBaseName)

// DefaultLogLevel is the default conduit log level if none is provided.
var DefaultLogLevel = log.InfoLevel

// Args configuration for conduit running.
// This is needed to support a CONDUIT_DATA_DIR environment variable.
type Args struct {
	ConduitDataDir    string `yaml:"data-dir"`
	NextRoundOverride uint64 `yaml:"next-round-override"`
}
