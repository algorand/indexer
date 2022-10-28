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
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListCmd(os.Stdout, args)
		},
		SilenceUsage: true,
		// Silence errors because our logger will catch and print any errors
		SilenceErrors: true,
	}

	return cmd
}

func runListCmd(out io.Writer, pluginNames []string) error {
	if len(pluginNames) == 0 {
		output, err := outputAll()
		if err != nil {
			return err
		}

		fmt.Fprint(out, output)
	}

	var potentialNames []string
	var sampleConfig string

	for _, pluginName := range pluginNames {

		if strings.HasPrefix(pluginName, "importer.") {
			// search importers...
			strippedName := strings.Replace(pluginName, "importer.", "", 1)
			ctor, err := importers.ImporterBuilderByName(strippedName)
			if err != nil {
				return err
			}

			meta := ctor.New().Metadata()
			fmt.Fprint(out, meta.SampleConfig())
			return nil
		}

		if strings.HasPrefix(pluginName, "processor.") {
			strippedName := strings.Replace(pluginName, "processor.", "", 1)
			// search processors
			ctor, err := processors.ProcessorBuilderByName(strippedName)
			if err != nil {
				return err
			}

			meta := ctor.New().Metadata()
			fmt.Fprint(out, meta.SampleConfig())
			return nil
		}

		if strings.HasPrefix(pluginName, "exporter.") {
			strippedName := strings.Replace(pluginName, "exporter.", "", 1)
			// search exporters
			ctor, err := exporters.ExporterBuilderByName(strippedName)
			if err != nil {
				return err
			}

			meta := ctor.New().Metadata()
			fmt.Fprint(out, meta.SampleConfig())
			return nil
		}

		// search through importers first
		importerCtor, err := importers.ImporterBuilderByName(pluginName)
		if err == nil {
			// if we found an importer...
			potentialNames = append(potentialNames, "importer."+pluginName)
			meta := importerCtor.New().Metadata()
			sampleConfig = meta.SampleConfig()
		}

		// search through processors next
		processorCtor, err := processors.ProcessorBuilderByName(pluginName)
		if err == nil {
			// if we found an exporter...
			potentialNames = append(potentialNames, "processor."+pluginName)
			meta := processorCtor.New().Metadata()
			sampleConfig = meta.SampleConfig()
		}

		// search through exporters last
		exporterCtor, err := exporters.ExporterBuilderByName(pluginName)
		if err == nil {
			// if we found an exporter....
			potentialNames = append(potentialNames, "exporter."+pluginName)
			meta := exporterCtor.New().Metadata()
			sampleConfig = meta.SampleConfig()
		}

		if len(potentialNames) == 0 {
			return fmt.Errorf("no plugins were found with the name: %s", pluginName)
		}

		if len(potentialNames) > 1 {
			return fmt.Errorf("multiple plugins were defined with the same name.  run command again with one of the following: %v", potentialNames)
		}

		fmt.Fprint(out, sampleConfig)

	}

	return nil
}

func outputAll() (string, error) {

	var sb strings.Builder

	_, err := fmt.Fprint(&sb, "Importers:\n")
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

	_, err = fmt.Fprint(&sb, "\nProcessors:\n")
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

	_, err = fmt.Fprint(&sb, "\nExporters:\n")
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
