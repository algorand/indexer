package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/cmd/block-generator/runner"
)

func init() {
	rand.Seed(12345)
	var runnerArgs runner.Args

	var runnerCmd = &cobra.Command{
		Use:   "runner",
		Short: "Run test suite and collect results.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runner.Run(runnerArgs); err != nil {
				fmt.Println(err)
			}
		},
	}

	runnerCmd.Flags().StringVarP(&runnerArgs.Path, "scenario", "s", "", "Directory containing scenarios, or specific scenario file.")
	runnerCmd.Flags().StringVarP(&runnerArgs.IndexerBinary, "indexer-binary", "i", "", "Path to indexer binary.")
	runnerCmd.Flags().Uint64VarP(&runnerArgs.IndexerPort, "indexer-port", "p", 4010, "Port to start the server at. This is useful if you have a prometheus server for collecting additional data.")
	runnerCmd.Flags().StringVarP(&runnerArgs.PostgresConnectionString, "postgres-connection-string", "c", "", "Postgres connection string.")
	runnerCmd.Flags().DurationVarP(&runnerArgs.RunDuration, "test-duration", "d", 5*time.Minute, "Duration to use for each scenario.")
	runnerCmd.Flags().StringVarP(&runnerArgs.ReportDirectory, "report-directory", "r", "", "Location to place test reports.")
	runnerCmd.Flags().StringVarP(&runnerArgs.LogLevel, "log-level", "l", "error", "LogLevel to use when starting Indexer. [error, warn, info, debug, trace]")
	runnerCmd.Flags().StringVarP(&runnerArgs.CPUProfilePath, "cpuprofile", "", "", "Path where Indexer writes its CPU profile.")
	runnerCmd.Flags().BoolVarP(&runnerArgs.ResetReportDir, "reset", "", false, "If set any existing report directory will be deleted before running tests.")

	runnerCmd.MarkFlagRequired("scenario")
	runnerCmd.MarkFlagRequired("indexer-binary")
	runnerCmd.MarkFlagRequired("postgres-connection-string")
	runnerCmd.MarkFlagRequired("report-directory")

	rootCmd.AddCommand(runnerCmd)
}
