package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/config"
)

var (
	suppliedAPIConfigFile string
	showAllDisabled       bool
)

var apiConfigCmd = &cobra.Command{
	Use:   "api-config",
	Short: "api configuration",
	Long:  "api configuration",
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		config.BindFlags(cmd)
		err = configureLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure logger: %v", err)
			panic(exit{1})
		}
		swag, err := generated.GetSwagger()

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get swagger: %v", err)
			panic(exit{1})
		}

		var potentialDisabledMapConfig *api.DisabledMapConfig
		if suppliedAPIConfigFile != "" {
			potentialDisabledMapConfig, err = api.MakeDisabledMapConfigFromFile(swag, suppliedAPIConfigFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to created disabled map config from file: %v", err)
				panic(exit{1})
			}
		}

		var displayDisabledMapConfig *api.DisplayDisabledMap
		// Show a limited subset
		if !showAllDisabled {
			displayDisabledMapConfig = api.MakeDisplayDisabledMapFromConfig(swag, potentialDisabledMapConfig, true)
		} else {
			displayDisabledMapConfig = api.MakeDisplayDisabledMapFromConfig(swag, potentialDisabledMapConfig, false)
		}

		output, err := displayDisabledMapConfig.String()

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to output yaml: %v", err)
			panic(exit{1})
		}

		fmt.Fprint(os.Stdout, output)
		panic(exit{0})

	},
}

func init() {
	apiConfigCmd.Flags().BoolVar(&showAllDisabled, "all", false, "show all api config parameters, enabled and disabled")
	apiConfigCmd.Flags().StringVar(&suppliedAPIConfigFile, "api-config-file", "", "supply an API config file to enable/disable parameters")
}
