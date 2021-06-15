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

	var scenarioDir string
	var indexerBinary string
	var indexerPort uint64
	var postgresConnectionString string
	var runDuration time.Duration
	var reportDirectory string

	var runnerCmd = &cobra.Command{
		Use:   "runner",
		Short: "Run test suite and collect results.",
		Run: func(cmd *cobra.Command, args []string) {
			runnerArgs := runner.RunnerArgs{
				Path:                     scenarioDir,
				IndexerBinary:            indexerBinary,
				IndexerPort:              indexerPort,
				PostgresConnectionString: postgresConnectionString,
				RunDuration:              runDuration,
				ReportDirectory:          reportDirectory,
			}
			if err := runner.Run(runnerArgs); err != nil {
				fmt.Println(err)
			}
		},
	}

	runnerCmd.Flags().StringVarP(&scenarioDir, "scenario", "s", "", "Directory containing scenarios, or specific scenario file.")
	runnerCmd.Flags().StringVarP(&indexerBinary, "indexer-binary", "i", "", "Path to indexer binary.")
	runnerCmd.Flags().Uint64VarP(&indexerPort, "indexer-port", "p", 4010, "Port to start the server at. This is useful if you have a prometheus server for collecting additional data.")
	runnerCmd.Flags().StringVarP(&postgresConnectionString, "postgres-connection-string", "c", "", "Postgres connection string.")
	runnerCmd.Flags().DurationVarP(&runDuration, "test-duration", "d", 5 * time.Minute, "Duration to use for each scenario.")
	runnerCmd.Flags().StringVarP(&reportDirectory, "report-directory", "r", "", "Location to place test reports.")

	runnerCmd.MarkFlagRequired("scenario")
	runnerCmd.MarkFlagRequired("indexer-binary")
	runnerCmd.MarkFlagRequired("postgres-connection-string")
	runnerCmd.MarkFlagRequired("report-directory")

	rootCmd.AddCommand(runnerCmd)
}
