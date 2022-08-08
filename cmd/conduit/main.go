package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/data"
)

import (
	_ "github.com/algorand/indexer/exporters/all"
	_ "github.com/algorand/indexer/importers/all"
	_ "github.com/algorand/indexer/processors/all"
)

var (
	logger *log.Logger
	defaultLogLevel string
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
	defaultLogLevel = "info"
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

			// Tie cobra variables into local go-lang variables
			config.BindFlagSet(cfg.Flags)
			logger.Info(cfg)

			if err := cfg.Valid(); err != nil {
				return err
			}

			pCfg, err := conduit.MakePipelineConfig(logger, cfg)

			if err != nil {
				return err
			}

			logLevel, _ := log.ParseLevel(pCfg.PipelineLogLevel)
			logger.Infof("Log level set to: %s", pCfg.PipelineLogLevel)

			logger.SetLevel(logLevel)

			var initProvider data.InitProvider = &conduit.AlgodInitProvider{}

			logger.Info("Conduit configuration is valid")

			pipeline, err := conduit.MakePipeline(pCfg, logger, &initProvider)
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
