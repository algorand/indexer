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
	showAllDisabled bool
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

		options := makeOptions()

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
	apiConfigCmd.Flags().BoolVarP(&showAllDisabled, "all", "", false, "show all api config parameters, enabled and disabled")
	apiConfigCmd.Flags().Lookup("all").NoOptDefVal = "true"
}
