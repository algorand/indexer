package list

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/pipeline"
)

// Command is the list command to embed in a root cobra command.
var Command = &cobra.Command{
	Use:   "list",
	Short: "lists all plugins available to conduit",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		printAll()
		return nil
	},
	// Silence errors because our logger will catch and print any errors
	SilenceErrors: true,
}

func makeDetailsCommand(use string, data func() []conduit.Metadata) *cobra.Command {
	return &cobra.Command{
		Use:     use + "s",
		Aliases: []string{use},
		Short:   fmt.Sprintf("usage detail for %s plugins", use),
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				printMetadata(os.Stdout, data())
			} else {
				printDetails(args[0], data())
			}
		},
	}
}

func init() {
	Command.AddCommand(makeDetailsCommand("importer", pipeline.ImporterMetadata))
	Command.AddCommand(makeDetailsCommand("processor", pipeline.ProcessorMetadata))
	Command.AddCommand(makeDetailsCommand("exporter", pipeline.ExporterMetadata))
}

func printDetails(name string, plugins []conduit.Metadata) {
	for _, data := range plugins {
		if data.Name == name {
			fmt.Println(data.SampleConfig)
			return
		}
	}

	fmt.Printf("Plugin not found: %s\n", name)
}

func printMetadata(w io.Writer, plugins []conduit.Metadata) {
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})

	for _, data := range plugins {
		fmtString := "%s - %s\n"

		if data.Deprecated {
			fmtString = "[DEPRECATED] " + fmtString
		}

		fmt.Fprintf(w, "  "+fmtString, data.Name, data.Description)
	}
}

func printAll() {
	fmt.Fprint(os.Stdout, "Importers:\n")
	printMetadata(os.Stdout, pipeline.ImporterMetadata())
	fmt.Fprint(os.Stdout, "\nProcessors:\n")
	printMetadata(os.Stdout, pipeline.ProcessorMetadata())
	fmt.Fprint(os.Stdout, "\nExporters:\n")
	printMetadata(os.Stdout, pipeline.ExporterMetadata())
}
