package main

import (
	"fmt"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/conduit"
)

import (
	// We need to import these so that the package wide init() function gets called
	_ "github.com/algorand/indexer/exporters/all"
	_ "github.com/algorand/indexer/importers/all"
	_ "github.com/algorand/indexer/processors/all"
)

var (
	logger *log.Logger
)

// init() function for main package
func init() {

	// Setup logger
	logger = log.New()

	formatter := conduit.PluginLogFormatter{
		Formatter: &log.JSONFormatter{
			DisableHTMLEscape: true,
		},
		Type: "Conduit",
		Name: "main",
	}

	logger.SetFormatter(&formatter)
	logger.SetOutput(os.Stdout)
}

// runConduitCmdWithConfig run the main logic with a supplied conduit config
func runConduitCmdWithConfig(cfg *conduit.Config) error {

	// Tie cobra variables into local go-lang variables
	cfg.Flags.VisitAll(func(f *pflag.Flag) {
		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores
		// e.g. prefix=STING and --favorite-color is set to STING_FAVORITE_COLOR
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			viper.BindEnv(f.Name, fmt.Sprintf("%s_%s", "CONDUIT", envVarSuffix))
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && viper.IsSet(f.Name) {
			val := viper.Get(f.Name)
			cfg.Flags.Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
	logger.Info(cfg)

	if err := cfg.Valid(); err != nil {
		return err
	}

	pCfg, err := conduit.MakePipelineConfig(logger, cfg)

	if err != nil {
		return err
	}

	logger.SetLevel(pCfg.PipelineLogLevel)
	logger.Infof("Log level set to: %s", pCfg.PipelineLogLevel)

	logger.Info("Conduit configuration is valid")

	pipeline, err := conduit.MakePipeline(pCfg, logger)
	if err != nil {
		return fmt.Errorf("pipeline creation error: %w", err)
	}

	// Make sure to call this so we can shutdown if there is an error
	defer func(pipeline conduit.Pipeline) {
		err := pipeline.Stop()
		if err != nil {
			logger.Errorf("Pipeline stoppage failure: %v", err)
		}
	}(pipeline)

	// TODO decide if blocking or not
	err = pipeline.Start()
	if err != nil {
		logger.Errorf("Pipeline start failure: %v", err)
		return err
	}

	return nil
}

// conduitCmd creates the main cobra command, initializes flags, and viper aliases
func conduitCmd() *cobra.Command {
	cfg := &conduit.Config{}
	conduitCmd := &cobra.Command{
		Use:   "conduit",
		Short: "run the conduit framework",
		Long:  "run the conduit framework",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConduitCmdWithConfig(cfg)
		},
	}

	cfg.Flags = conduitCmd.PersistentFlags()
	cfg.Flags.StringVarP(&cfg.ConduitDataDir, "data-dir", "d", "", "set the data directory for the conduit binary")

	return conduitCmd
}

func main() {
	if err := conduitCmd().Execute(); err != nil {
		log.Errorf("%v", err)
		os.Exit(1)
	}

	os.Exit(0)
}
