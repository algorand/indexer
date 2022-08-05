package conduit

import (
	"fmt"
	"github.com/algorand/go-algorand/util"
	"github.com/spf13/pflag"
	"strings"
)

// Config configuration for conduit running
type Config struct {
	Flags          *pflag.FlagSet
	ConduitDataDir string
}

func (cfg *Config) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Data Directory: %s ", cfg.ConduitDataDir)

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
