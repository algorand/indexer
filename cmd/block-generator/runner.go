package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	//rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", configFileName, fmt.Sprintf("Override default config file from '%s'.", configFileName))
	//rootCmd.PersistentFlags().UintVarP(&port, "port", "p", 4010, "Port to start the server at.")

	rootCmd.AddCommand(runnerCmd)

	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
}

var runnerCmd = &cobra.Command{
	Use:   "runner",
	Short: "Run test suite and collect results.",
	//Args:
	Run: func(cmd *cobra.Command, args []string) {

	},
}
