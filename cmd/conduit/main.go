package main

import (
	"context"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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

	// From docs:
	// BindEnv takes one or more parameters. The first parameter is the key name, the rest are the name of the
	// environment variables to bind to this key. If more than one are provided, they will take precedence in
	// the specified order. The name of the environment variable is case sensitive. If the ENV variable name is not
	// provided, then Viper will automatically assume that the ENV variable matches the following format:
	// prefix + "_" + the key name in ALL CAPS. When you explicitly provide the ENV variable name (the second
	// parameter), it does not automatically add the prefix. For example if the second parameter is "id",
	// Viper will look for the ENV variable "ID".
	//
	// One important thing to recognize when working with ENV variables is that the value will be read each time
	// it is accessed. Viper does not fix the value when the BindEnv is called.
	err := viper.BindEnv("data-dir", "CONDUIT_DATA_DIR")
	if err != nil {
		return err
	}

	// Changed returns true if the key was set during parse and IsSet determines if it is in the internal
	// viper hashmap
	if !cfg.Flags.Changed("data-dir") && viper.IsSet("data-dir") {
		cfg.ConduitDataDir = viper.Get("data-dir").(string)
	}

	logger.Info(cfg)

	if err := cfg.Valid(); err != nil {
		return err
	}

	pCfg, err := conduit.MakePipelineConfig(logger, cfg)

	if err != nil {
		return err
	}

	logger.Info("Conduit configuration is valid")

	ctx := context.Background()

	pipeline, err := conduit.MakePipeline(ctx, pCfg, logger)
	if err != nil {
		return fmt.Errorf("pipeline creation error: %w", err)
	}

	// TODO decide if blocking or not
	err = pipeline.Start()
	if err != nil {
		logger.Errorf("Pipeline start failure: %v", err)
		return err
	}

	// Make sure to call this so we can shutdown if there is an error
	defer pipeline.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
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
		SilenceUsage: true,
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
