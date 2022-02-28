package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/api"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/config"
)

var apiConfigCmd = &cobra.Command{
	Use:   "api-config",
	Short: "dump api configuration",
	Long:  "dump api configuration",
	//Args:
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

		var displayDisabledMapConfig *api.DisplayDisabledMap
		// Show a limited subset
		if len(args) == 0 {
			displayDisabledMapConfig = api.MakeDisplayDisabledMapFromConfig(swag, disabledMapConfig, true)

		} else {
			// This is the only acceptable option
			if args[0] != "all" {
				fmt.Fprintf(os.Stderr, "unrecognized option to api-config: %s", args[0])
				os.Exit(1)
			}

			displayDisabledMapConfig = api.MakeDisplayDisabledMapFromConfig(swag, disabledMapConfig, false)
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
