package core

import "github.com/spf13/cobra"

// ImportValidatorCmd is the real-time import validator command.
var ImportValidatorCmd *cobra.Command

func init() {
	var args ImportValidatorArgs

	ImportValidatorCmd = &cobra.Command{
		Use:   "import-validator",
		Short: "Import validator",
		Long:  "Run with the special import validator mode. A sqlite ledger will be maintained along with the standard Indexer database. Each will increment the round in lock-step, and compare the account state of each modified account before moving on to the next round. If a data discrepency is detected, an error will be printed to stderr and the program will terminate.",
		Run: func(cmd *cobra.Command, _ []string) {
			Run(args)
		},
	}

	ImportValidatorCmd.Flags().StringVar(&args.AlgodAddr, "algod-net", "", "host:port of algod")
	ImportValidatorCmd.MarkFlagRequired("algod-net")

	ImportValidatorCmd.Flags().StringVar(
		&args.AlgodToken, "algod-token", "", "api access token for algod")
	ImportValidatorCmd.MarkFlagRequired("algod-token")

	ImportValidatorCmd.Flags().StringVar(
		&args.AlgodLedger, "algod-ledger", "", "path to algod ledger directory")
	ImportValidatorCmd.MarkFlagRequired("algod-ledger")

	ImportValidatorCmd.Flags().StringVar(
		&args.PostgresConnStr, "postgres", "", "connection string for postgres database")
	ImportValidatorCmd.MarkFlagRequired("postgres")
}
