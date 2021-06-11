package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use: `block-generator`,
	Short: `Block generator testing tools.`,
}

func main() {
	rootCmd.Execute()
}