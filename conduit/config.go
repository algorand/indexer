package conduit

import (
	"fmt"
	"github.com/algorand/go-algorand/util"
	"github.com/spf13/pflag"
	"strings"
)

// DefaultConfigName is the default conduit configuration filename.
const DefaultConfigName = "conduit.yml"

// Config configuration for conduit running.
// This is needed to support a CONDUIT_DATA_DIR environment variable.
type Config struct {
	Flags          *pflag.FlagSet
	ConduitDataDir string `yaml:"data-dir"`
}

func (cfg *Config) String() string {
	var sb strings.Builder

	var dataDirToPrint string
	if cfg.ConduitDataDir == "" {
		dataDirToPrint = "[EMPTY]"
	} else {
		dataDirToPrint = cfg.ConduitDataDir
	}

	fmt.Fprintf(&sb, "Data Directory: %s ", dataDirToPrint)

	return sb.String()
}

// Valid validates a supplied configuration
func (cfg *Config) Valid() error {

	if cfg.ConduitDataDir == "" {
		return fmt.Errorf("supplied data directory was empty")
	}

	if !util.IsDir(cfg.ConduitDataDir) {
		return fmt.Errorf("supplied data directory (%s) was not valid", cfg.ConduitDataDir)
	}

	return nil
}
