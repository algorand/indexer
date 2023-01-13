package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/algorand/indexer/cmd/conduit/internal/list"
	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/pipeline"
	// We need to import these so that the package wide init() function gets called
	_ "github.com/algorand/indexer/conduit/plugins/exporters/all"
	_ "github.com/algorand/indexer/conduit/plugins/importers/all"
	_ "github.com/algorand/indexer/conduit/plugins/processors/all"
	"github.com/algorand/indexer/loggers"
	"github.com/algorand/indexer/version"
)

var (
	loggerManager        *loggers.LoggerManager
	logger               *log.Logger
	conduitCmd           = makeConduitCmd()
	initCmd              = makeInitCmd()
	defaultDataDirectory = "data"
	//go:embed banner.txt
	banner string
)

// init() function for main package
func init() {
	loggerManager = loggers.MakeLoggerManager(os.Stdout)
	conduitCmd.AddCommand(initCmd)
}

// runConduitCmdWithConfig run the main logic with a supplied conduit config
func runConduitCmdWithConfig(args *conduit.Args) error {
	defer pipeline.HandlePanic(logger)

	if args.ConduitDataDir == "" {
		args.ConduitDataDir = os.Getenv("CONDUIT_DATA_DIR")
	}

	pCfg, err := pipeline.MakePipelineConfig(args)
	if err != nil {
		return err
	}

	// Initialize logger
	level, err := log.ParseLevel(pCfg.PipelineLogLevel)
	if err != nil {
		return fmt.Errorf("runConduitCmdWithConfig(): invalid log level: %s", err)
	}

	logger, err = loggerManager.MakeRootLogger(level, pCfg.LogFile)
	if err != nil {
		return fmt.Errorf("runConduitCmdWithConfig(): failed to create logger: %w", err)
	}

	logger.Infof("Using data directory: %s", args.ConduitDataDir)
	logger.Info("Conduit configuration is valid")

	if !pCfg.HideBanner {
		fmt.Printf(banner)
	}

	if pCfg.LogFile != "" {
		logger.Infof("Conduit log file: %s", pCfg.LogFile)
	}

	ctx := context.Background()
	pipeline, err := pipeline.MakePipeline(ctx, pCfg, logger)
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

// makeConduitCmd creates the main cobra command, initializes flags
func makeConduitCmd() *cobra.Command {
	cfg := &conduit.Args{}
	var vFlag bool
	cmd := &cobra.Command{
		Use:   "conduit",
		Short: "run the conduit framework",
		Long:  "run the conduit framework",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConduitCmdWithConfig(cfg)
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if vFlag {
				fmt.Println("Conduit Pre-Release")
				fmt.Printf("%s\n", version.LongVersion())
				os.Exit(0)
			}
		},
		SilenceUsage: true,
		// Silence errors because our logger will catch and print any errors
		SilenceErrors: true,
	}
	cmd.Flags().StringVarP(&cfg.ConduitDataDir, "data-dir", "d", "", "set the data directory for the conduit binary")
	cmd.Flags().Uint64VarP(&cfg.NextRoundOverride, "next-round-override", "r", 0, "set the starting round. Overrides next-round in metadata.json")
	cmd.Flags().BoolVarP(&vFlag, "version", "v", false, "print the conduit version")

	cmd.AddCommand(list.Command)

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

	_, _ = f.WriteString(sampleConfig)
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
	// Hidden command to generate docs in a given directory
	// conduit generate-docs [path]
	if len(os.Args) == 3 && os.Args[1] == "generate-docs" {
		err := doc.GenMarkdownTree(conduitCmd, os.Args[2])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := conduitCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
