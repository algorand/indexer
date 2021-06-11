package main

import (
	"fmt"
	"math/rand"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/algorand/indexer/cmd/block-generator/generator"
)

var configFile string
var port uint64

const configFileName = "block_generator_config"

func init() {
	rand.Seed(12345)

	daemonCmd.Flags().StringVarP(&configFile, "config", "c", configFileName, fmt.Sprintf("Override default config file from '%s'.", configFileName))
	daemonCmd.Flags().Uint64VarP(&port, "port", "p", 4010, "Port to start the server at.")

	rootCmd.AddCommand(daemonCmd)

	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the generator daemon in standalone mode.",
	Run: func(cmd *cobra.Command, args []string) {
		generator.StartServer(configFile, port)
	},
}
