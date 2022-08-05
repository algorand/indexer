package conduit

import (
	"fmt"
	"github.com/algorand/indexer/config"
	"github.com/spf13/pflag"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	logger *log.Logger
)

// init() function for main package
func init() {

	// Setup logger
	logger = log.New()
	logger.SetFormatter(&log.JSONFormatter{
		DisableHTMLEscape: true,
	})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(log.InfoLevel)
}

type conduitConfig struct {
	flags          *pflag.FlagSet
	conduitDataDir string
}

func (cfg *conduitConfig) String() string {
	if cfg == nil {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Data Directory: %s ", cfg.conduitDataDir)

	return sb.String()
}

// validateConfig validates a supplied configuration
func validateConfig(cfg *conduitConfig) error {
	// belt and suspenders
	if cfg == nil {
		return fmt.Errorf("validation failure.  configuration was nil")
	}

	return nil
}

// conduitCmd creates the main cobra command, initializes flags, and viper aliases
func conduitCmd() *cobra.Command {
	cfg := &conduitConfig{}
	conduitCmd := &cobra.Command{
		Use:   "conduit",
		Short: "run the conduit framework",
		Long:  "run the conduit framework",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			// Tie cobra variables into local go-lang variables
			config.BindFlagSet(cfg.flags)
			logger.Info(cfg)

			if err := validateConfig(cfg); err != nil {
				return err
			}

			return runConduit(cfg)
		},
	}

	cfg.flags = conduitCmd.Flags()
	cfg.flags.StringVarP(&cfg.conduitDataDir, "data-dir", "d", "", "set the data directory for the conduit binary")

	return conduitCmd
}

// runConduit runs the main conduit command, the cfg is assumed to be initialized and
// valid at this point
func runConduit(cfg *conduitConfig) error {
	// belt and suspenders...
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}
	return nil
}

func main() {
	if err := conduitCmd().Execute(); err != nil {
		// TODO Log here
		os.Exit(1)
	}

	os.Exit(0)
}
