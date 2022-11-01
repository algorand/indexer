package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/processors"
)

func makeListCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists all plugins available to conduit",
		Long:  "lists all plugins available to conduit",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListCmd(os.Stdout, args)
		},
		SilenceUsage: true,
		// Silence errors because our logger will catch and print any errors
		SilenceErrors: true,
	}

	return cmd
}

func runListCmd(out io.Writer, listArgs []string) error {
	if len(listArgs) == 0 {
		output, err := outputAll()
		if err != nil {
			return err
		}

		fmt.Fprint(out, output)
		return nil
	}

	if len(listArgs) == 1 {
		return fmt.Errorf("invalid argument, specify [importers, processors, exporters] before plugin name")
	}

	pluginTypeSpecifier := listArgs[0]
	pluginName := listArgs[1]

	var sampleConfig string

	switch pluginTypeSpecifier {
	case "importers":
		{
			importerCtor, err := importers.ImporterBuilderByName(pluginName)
			if err != nil {
				return fmt.Errorf("no importer by name: %s", pluginName)
			}
			// if we found an importer...
			meta := importerCtor.New().Metadata()
			sampleConfig = meta.SampleConfig()
		}
		break
	case "processors":
		{
			processorCtor, err := processors.ProcessorBuilderByName(pluginName)
			if err != nil {
				return fmt.Errorf("no processor by name: %s", pluginName)
			}
			meta := processorCtor.New().Metadata()
			sampleConfig = meta.SampleConfig()
		}
		break
	case "exporters":
		{
			exporterCtor, err := exporters.ExporterBuilderByName(pluginName)
			if err != nil {
				return fmt.Errorf("no exporter by name: %s", pluginName)
			}
			meta := exporterCtor.New().Metadata()
			sampleConfig = meta.SampleConfig()
		}
		break
	default:
		return fmt.Errorf("invalid argument type (%s), specify [importers, processors, exporters] before plugin name", pluginTypeSpecifier)
	}

	fmt.Fprint(out, sampleConfig)
	return nil
}

func outputAll() (string, error) {

	var sb strings.Builder

	_, err := fmt.Fprint(&sb, "importers:\n")
	if err != nil {
		return "", err
	}
	importerNames := importers.ImporterNames()
	for _, importerName := range importerNames {
		ctor, err := importers.ImporterBuilderByName(importerName)
		// belt and suspenders
		if err != nil {
			return "", err
		}
		meta := ctor.New().Metadata()

		fmtString := "%s - %s\n"

		if meta.Deprecated() {
			fmtString = "[DEPRECATED] " + fmtString
		}

		_, err = fmt.Fprintf(&sb, "  "+fmtString, importerName, meta.Description())
		if err != nil {
			return "", err
		}

	}

	_, err = fmt.Fprint(&sb, "\nprocessors:\n")
	if err != nil {
		return "", err
	}
	processorNames := processors.ProcessorNames()
	for _, processorName := range processorNames {
		ctor, err := processors.ProcessorBuilderByName(processorName)
		// belt and suspenders
		if err != nil {
			return "", err
		}
		meta := ctor.New().Metadata()

		fmtString := "%s - %s\n"

		if meta.Deprecated() {
			fmtString = "[DEPRECATED] " + fmtString
		}

		_, err = fmt.Fprintf(&sb, "  "+fmtString, processorName, meta.Description())
		if err != nil {
			return "", err
		}

	}

	_, err = fmt.Fprint(&sb, "\nexporters:\n")
	if err != nil {
		return "", err
	}
	exporterNames := exporters.ExporterNames()
	for _, exporterName := range exporterNames {
		ctor, err := exporters.ExporterBuilderByName(exporterName)
		// belt and suspenders
		if err != nil {
			return "", err
		}
		meta := ctor.New().Metadata()

		fmtString := "%s - %s\n"

		if meta.Deprecated() {
			fmtString = "[DEPRECATED] " + fmtString
		}

		_, err = fmt.Fprintf(&sb, "  "+fmtString, exporterName, meta.Description())
		if err != nil {
			return "", err
		}

	}

	return sb.String(), nil

}
