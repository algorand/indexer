package main

import (
	"github.com/algorand/indexer/cmd/block-generator/generator"
	"github.com/spf13/cobra"
	"math/rand"
)

const configFileName = "block_generator_config"

func init() {
	rand.Seed(12345)

	var configFile string
	var port uint64

	var daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Start the generator daemon in standalone mode.",
		Run: func(cmd *cobra.Command, args []string) {
			_, done := generator.StartServer(configFile, int(port))
			<- done
		},
	}

	daemonCmd.Flags().StringVarP(&configFile, "config", "c", "", "Specify the block configuration yaml file.")
	daemonCmd.Flags().Uint64VarP(&port, "port", "p", 4010, "Port to start the server at.")

	daemonCmd.MarkFlagRequired("config")

	rootCmd.AddCommand(daemonCmd)
}
