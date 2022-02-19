package core

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/cmd/block-generator/generator"
	"github.com/algorand/indexer/cmd/block-generator/runner"
)

// Block-generator related cobra commands, ready to be executed or included as subcommands.
var (
	RunnerCmd      *cobra.Command
	DaemonCmd      *cobra.Command
	BlockGenerator *cobra.Command
)

func init() {
	// Runner
	rand.Seed(12345)
	var runnerArgs runner.Args

	RunnerCmd = &cobra.Command{
		Use:   "runner",
		Short: "Run test suite and collect results.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runner.Run(runnerArgs); err != nil {
				fmt.Println(err)
			}
		},
	}

	RunnerCmd.Flags().StringVarP(&runnerArgs.Path, "scenario", "s", "", "Directory containing scenarios, or specific scenario file.")
	RunnerCmd.Flags().StringVarP(&runnerArgs.IndexerBinary, "indexer-binary", "i", "", "Path to indexer binary.")
	RunnerCmd.Flags().Uint64VarP(&runnerArgs.IndexerPort, "indexer-port", "p", 4010, "Port to start the server at. This is useful if you have a prometheus server for collecting additional data.")
	RunnerCmd.Flags().StringVarP(&runnerArgs.PostgresConnectionString, "postgres-connection-string", "c", "", "Postgres connection string.")
	RunnerCmd.Flags().DurationVarP(&runnerArgs.RunDuration, "test-duration", "d", 5*time.Minute, "Duration to use for each scenario.")
	RunnerCmd.Flags().StringVarP(&runnerArgs.ReportDirectory, "report-directory", "r", "", "Location to place test reports.")
	RunnerCmd.Flags().StringVarP(&runnerArgs.LogLevel, "log-level", "l", "error", "LogLevel to use when starting Indexer. [error, warn, info, debug, trace]")
	RunnerCmd.Flags().StringVarP(&runnerArgs.CPUProfilePath, "cpuprofile", "", "", "Path where Indexer writes its CPU profile.")
	RunnerCmd.Flags().BoolVarP(&runnerArgs.ResetReportDir, "reset", "", false, "If set any existing report directory will be deleted before running tests.")
	RunnerCmd.Flags().BoolVarP(&runnerArgs.RunValidation, "validate", "", false, "If set the validator will run after test-duration has elapsed to verify data is correct. An extra line in each report indicates validator success or failure.")

	RunnerCmd.MarkFlagRequired("scenario")
	RunnerCmd.MarkFlagRequired("indexer-binary")
	RunnerCmd.MarkFlagRequired("postgres-connection-string")
	RunnerCmd.MarkFlagRequired("report-directory")

	// Daemon
	var configFile string
	var port uint64

	DaemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Start the generator daemon in standalone mode.",
		Run: func(cmd *cobra.Command, args []string) {
			addr := fmt.Sprintf(":%d", port)
			srv, _ := generator.MakeServer(configFile, addr)
			srv.ListenAndServe()
		},
	}

	DaemonCmd.Flags().StringVarP(&configFile, "config", "c", "", "Specify the block configuration yaml file.")
	DaemonCmd.Flags().Uint64VarP(&port, "port", "p", 4010, "Port to start the server at.")

	DaemonCmd.MarkFlagRequired("config")

	// block-generator
	BlockGenerator = &cobra.Command{
		Use:   `block-generator`,
		Short: `Block generator testing tools.`,
	}
	BlockGenerator.AddCommand(RunnerCmd)
	BlockGenerator.AddCommand(DaemonCmd)

}
