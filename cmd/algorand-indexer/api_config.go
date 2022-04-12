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
	showRecommended       bool
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
			os.Exit(1)
		}
		swag, err := generated.GetSwagger()

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get swagger: %v", err)
			os.Exit(1)
		}

		// Can't show recommended and a file at the same time
		if suppliedAPIConfigFile != "" && showRecommended {
			fmt.Fprintf(os.Stderr, "do not supply --api-config-file and --recommended at the same time")
			os.Exit(1)
		}

		if showRecommended {
			recommendedConfig := api.GetDefaultDisabledMapConfigForPostgres()
			displayMap := api.MakeDisplayDisabledMapFromConfig(swag, recommendedConfig, true)

			output, err := displayMap.String()

			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to output yaml: %v", err)
				os.Exit(1)
			}

			fmt.Fprint(os.Stdout, output)
			os.Exit(0)

		}

		options := makeOptions()
		if suppliedAPIConfigFile != "" {
			potentialDisabledMapConfig, err := api.MakeDisabledMapConfigFromFile(swag, suppliedAPIConfigFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to created disabled map config from file: %v", err)
				os.Exit(1)
			}
			options.DisabledMapConfig = potentialDisabledMapConfig
		}

		var displayDisabledMapConfig *api.DisplayDisabledMap
		// Show a limited subset
		if !showAllDisabled {
			displayDisabledMapConfig = api.MakeDisplayDisabledMapFromConfig(swag, options.DisabledMapConfig, true)
		} else {
			displayDisabledMapConfig = api.MakeDisplayDisabledMapFromConfig(swag, options.DisabledMapConfig, false)
		}

		output, err := displayDisabledMapConfig.String()

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to output yaml: %v", err)
			os.Exit(1)
		}

		fmt.Fprint(os.Stdout, output)
		os.Exit(0)

	},
}

func init() {
	apiConfigCmd.Flags().BoolVar(&showAllDisabled, "all", false, "show all api config parameters, enabled and disabled")
	apiConfigCmd.Flags().StringVar(&suppliedAPIConfigFile, "api-config-file", "", "supply an API config file to enable/disable parameters")
	apiConfigCmd.Flags().BoolVar(&showRecommended, "recommended", false, "show the recommended configuration")
}
