package main

import (
	"fmt"
	"math/rand"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/cmd/block-generator/generator"
)

func init() {
	rand.Seed(12345)

	var configFile string
	var port uint64

	var daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Start the generator daemon in standalone mode.",
		Run: func(cmd *cobra.Command, args []string) {
			addr := fmt.Sprintf(":%d", port)
			srv, _ := generator.MakeServer(configFile, addr)
			srv.ListenAndServe()
		},
	}

	daemonCmd.Flags().StringVarP(&configFile, "config", "c", "", "Specify the block configuration yaml file.")
	daemonCmd.Flags().Uint64VarP(&port, "port", "p", 4010, "Port to start the server at.")

	daemonCmd.MarkFlagRequired("config")

	rootCmd.AddCommand(daemonCmd)
}
