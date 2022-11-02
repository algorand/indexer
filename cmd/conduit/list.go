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
			meta, err := importers.ImporterMetaDataByName(pluginName)
			if err != nil {
				return fmt.Errorf("no importer by name: %s", pluginName)
			}
			// if we found an importer...
			sampleConfig = meta.SampleConfig()
		}
		break
	case "processors":
		{
			meta, err := processors.ProcessorMetaDataByName(pluginName)
			if err != nil {
				return fmt.Errorf("no processor by name: %s", pluginName)
			}

			sampleConfig = meta.SampleConfig()

		}
		break
	case "exporters":
		{
			meta, err := exporters.ExporterMetaDataByName(pluginName)
			if err != nil {
				return fmt.Errorf("no exporter by name: %s", pluginName)
			}
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

	// function to generate the proper format
	genFunc := func(w io.Writer, deprecated bool, name string, description string) error {

		var err error
		fmtString := "%s - %s\n"

		if deprecated {
			fmtString = "[DEPRECATED] " + fmtString
		}

		_, err = fmt.Fprintf(&sb, "  "+fmtString, name, description)
		if err != nil {
			return err
		}
		return nil
	}

	_, err := fmt.Fprint(&sb, "importers:\n")
	if err != nil {
		return "", err
	}
	importerNames := importers.ImporterNames()
	for _, importerName := range importerNames {

		meta, err := importers.ImporterMetaDataByName(importerName)
		// belt and suspenders
		if err != nil {
			return "", err
		}
		err = genFunc(&sb, meta.Deprecated(), importerName, meta.Description())
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

		meta, err := processors.ProcessorMetaDataByName(processorName)
		// belt and suspenders
		if err != nil {
			return "", err
		}
		err = genFunc(&sb, meta.Deprecated(), processorName, meta.Description())
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

		meta, err := exporters.ExporterMetaDataByName(exporterName)
		// belt and suspenders
		if err != nil {
			return "", err
		}
		err = genFunc(&sb, meta.Deprecated(), exporterName, meta.Description())
		if err != nil {
			return "", err
		}

	}

	return sb.String(), nil

}
