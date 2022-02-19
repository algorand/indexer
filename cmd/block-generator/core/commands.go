package core

import (
	"github.com/spf13/cobra"

	"github.com/algorand/indexer/cmd/block-generator/generator"
	"github.com/algorand/indexer/cmd/block-generator/runner"
)

// BlockGenerator related cobra commands, ready to be executed or included as subcommands.
var BlockGenerator *cobra.Command

func init() {
	BlockGenerator = &cobra.Command{
		Use:   `block-generator`,
		Short: `Block generator testing tools.`,
	}
	BlockGenerator.AddCommand(runner.RunnerCmd)
	BlockGenerator.AddCommand(generator.DaemonCmd)
}
