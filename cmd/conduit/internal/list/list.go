package list

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/processors"
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
	Command.AddCommand(makeDetailsCommand("importer", importerMetadata))
	Command.AddCommand(makeDetailsCommand("processor", processorMetadata))
	Command.AddCommand(makeDetailsCommand("exporter", exporterMetadata))
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

func importerMetadata() (results []conduit.Metadata) {
	for _, constructor := range importers.Importers {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
}

func processorMetadata() (results []conduit.Metadata) {
	for _, constructor := range processors.Processors {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
}

func exporterMetadata() (results []conduit.Metadata) {
	for _, constructor := range exporters.Exporters {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
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
	fmt.Fprint(os.Stdout, "importers:\n")
	printMetadata(os.Stdout, importerMetadata())
	fmt.Fprint(os.Stdout, "processors:\n")
	printMetadata(os.Stdout, processorMetadata())
	fmt.Fprint(os.Stdout, "exporters:\n")
	printMetadata(os.Stdout, exporterMetadata())
}
