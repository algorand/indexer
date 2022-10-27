package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/algorand/indexer/util/metrics"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/loggers"

	// We need to import these so that the package wide init() function gets called
	_ "github.com/algorand/indexer/exporters/all"
	_ "github.com/algorand/indexer/importers/all"
	_ "github.com/algorand/indexer/processors/all"
)

var (
	loggerManager        *loggers.LoggerManager
	logger               *log.Logger
	conduitCmd           = makeConduitCmd()
	initCmd              = makeInitCmd()
	defaultDataDirectory = "data"
)

// init() function for main package
func init() {

	loggerManager = loggers.MakeLoggerManager(os.Stdout)
	// Setup logger
	logger = loggerManager.MakeLogger()

	formatter := conduit.PluginLogFormatter{
		Formatter: &log.JSONFormatter{
			DisableHTMLEscape: true,
		},
		Type: "Conduit",
		Name: "main",
	}

	logger.SetFormatter(&formatter)

	conduitCmd.AddCommand(initCmd)
	metrics.RegisterPrometheusMetrics()
}

// runConduitCmdWithConfig run the main logic with a supplied conduit config
func runConduitCmdWithConfig(cfg *conduit.Config) error {
	defer conduit.HandlePanic(logger)

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

	pCfg, err := conduit.MakePipelineConfig(logger, cfg)

	if err != nil {
		return err
	}

	logger.Info("Conduit configuration is valid")

	if pCfg.LogFile != "" {
		logger.Infof("Conduit log file: %s", pCfg.LogFile)
	}

	ctx := context.Background()

	pipeline, err := conduit.MakePipeline(ctx, pCfg, logger)
	if err != nil {
		return fmt.Errorf("pipeline creation error: %w", err)
	}

	err = pipeline.Init()
	if err != nil {
		return fmt.Errorf("pipeline init error: %w", err)
	}
	pipeline.Start()
	defer pipeline.Stop()
	pipeline.Wait()
	return pipeline.Error()
}

// makeConduitCmd creates the main cobra command, initializes flags, and viper aliases
func makeConduitCmd() *cobra.Command {
	cfg := &conduit.Config{}
	cmd := &cobra.Command{
		Use:   "conduit",
		Short: "run the conduit framework",
		Long:  "run the conduit framework",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConduitCmdWithConfig(cfg)
		},
		SilenceUsage: true,
		// Silence errors because our logger will catch and print any errors
		SilenceErrors: true,
	}

	cfg.Flags = cmd.Flags()
	cfg.Flags.StringVarP(&cfg.ConduitDataDir, "data-dir", "d", "", "set the data directory for the conduit binary")

	return cmd
}

//go:embed conduit.yml.example
var sampleConfig string

func runConduitInit(path string) error {
	var location string
	if path == "" {
		path = defaultDataDirectory
		location = "in the current working directory"
	} else {
		location = fmt.Sprintf("at '%s'", path)
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	configFilePath := filepath.Join(path, conduit.DefaultConfigName)
	f, err := os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("runConduitInit(): failed to create %s", configFilePath)
	}
	defer f.Close()

	f.WriteString(sampleConfig)
	if err != nil {
		return fmt.Errorf("runConduitInit(): failed to write sample config: %w", err)
	}

	fmt.Printf("A data directory has been created %s.\n", location)
	fmt.Printf("\nBefore it can be used, the config file needs to be updated\n")
	fmt.Printf("by setting the algod address/token and the block-dir path where\n")
	fmt.Printf("Conduit should write the block files.\n")
	fmt.Printf("\nOnce the config file is updated, start Conduit with:\n")
	fmt.Printf("  ./conduit -d %s\n", path)
	return nil
}

// makeInitCmd creates a sample data directory.
func makeInitCmd() *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "initializes a sample data directory",
		Long:  "initializes a Conduit data directory and conduit.yml file configured with the file_writer plugin. The config file needs to be modified slightly to include an algod address and token. Once ready, launch conduit with './conduit -d /path/to/data'.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConduitInit(data)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVarP(&data, "data", "d", "", "Full path to new data directory. If not set, a directory named 'data' will be created in the current directory.")

	return cmd
}

func main() {
	if err := conduitCmd.Execute(); err != nil {
		logger.Errorf("%v", err)
		os.Exit(1)
	}

	os.Exit(0)
}
