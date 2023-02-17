package main

import (
	_ "embed"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/pipeline"
	"github.com/algorand/indexer/conduit/plugins/exporters/filewriter"
	noopExporter "github.com/algorand/indexer/conduit/plugins/exporters/noop"
	algodimporter "github.com/algorand/indexer/conduit/plugins/importers/algod"
	fileimporter "github.com/algorand/indexer/conduit/plugins/importers/filereader"
	"github.com/algorand/indexer/conduit/plugins/processors/filterprocessor"
	noopProcessor "github.com/algorand/indexer/conduit/plugins/processors/noop"
)

//go:embed conduit.test.init.default.yml
var defaultYml string

// TestInitDataDirectory tests the initialization of the data directory
func TestInitDataDirectory(t *testing.T) {
	verifyFile := func(file string, importer string, exporter string, processors []string) {
		require.FileExists(t, file)
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		var cfg pipeline.Config
		require.NoError(t, yaml.Unmarshal(data, &cfg))
		assert.Equal(t, importer, cfg.Importer.Name)
		assert.Equal(t, exporter, cfg.Exporter.Name)
		require.Equal(t, len(processors), len(cfg.Processors))
		for i := range processors {
			assert.Equal(t, processors[i], cfg.Processors[i].Name)
		}
	}

	// Defaults
	dataDirectory := t.TempDir()
	runConduitInit(dataDirectory, "", []string{}, "")
	verifyFile(fmt.Sprintf("%s/conduit.yml", dataDirectory), algodimporter.PluginName, filewriter.PluginName, nil)

	// Explicit defaults
	dataDirectory = t.TempDir()
	runConduitInit(dataDirectory, algodimporter.PluginName, []string{noopProcessor.PluginName}, filewriter.PluginName)
	verifyFile(fmt.Sprintf("%s/conduit.yml", dataDirectory), algodimporter.PluginName, filewriter.PluginName, []string{noopProcessor.PluginName})

	// Different
	dataDirectory = t.TempDir()
	runConduitInit(dataDirectory, fileimporter.PluginName, []string{noopProcessor.PluginName, filterprocessor.PluginName}, noopExporter.PluginName)
	verifyFile(fmt.Sprintf("%s/conduit.yml", dataDirectory), fileimporter.PluginName, noopExporter.PluginName, []string{noopProcessor.PluginName, filterprocessor.PluginName})
}

func TestBanner(t *testing.T) {
	test := func(t *testing.T, hideBanner bool) {
		// Capture stdout.
		stdout := os.Stdout
		defer func() {
			os.Stdout = stdout
		}()
		stdoutFilePath := path.Join(t.TempDir(), "stdout.txt")
		f, err := os.Create(stdoutFilePath)
		require.NoError(t, err)
		defer f.Close()
		os.Stdout = f

		cfg := pipeline.Config{
			ConduitArgs: &conduit.Args{ConduitDataDir: t.TempDir()},
			HideBanner:  hideBanner,
			Importer:    pipeline.NameConfigPair{Name: "test", Config: map[string]interface{}{"a": "a"}},
			Processors:  nil,
			Exporter:    pipeline.NameConfigPair{Name: "test", Config: map[string]interface{}{"a": "a"}},
		}
		data, err := yaml.Marshal(&cfg)
		require.NoError(t, err)
		configFile := path.Join(cfg.ConduitArgs.ConduitDataDir, conduit.DefaultConfigName)
		os.WriteFile(configFile, data, 0755)
		require.FileExists(t, configFile)

		err = runConduitCmdWithConfig(cfg.ConduitArgs)
		data, err = os.ReadFile(stdoutFilePath)
		require.NoError(t, err)

		if hideBanner {
			assert.NotContains(t, string(data), banner)
		} else {
			assert.Contains(t, string(data), banner)
		}
	}

	t.Run("Banner_hidden", func(t *testing.T) {
		test(t, true)
	})

	t.Run("Banner_shown", func(t *testing.T) {
		test(t, false)
	})
}

func TestLogFile(t *testing.T) {
	// returns stdout
	test := func(t *testing.T, logfile string) ([]byte, error) {
		// Capture stdout.
		stdout := os.Stdout
		defer func() {
			os.Stdout = stdout
		}()
		stdoutFilePath := path.Join(t.TempDir(), "stdout.txt")
		f, err := os.Create(stdoutFilePath)
		require.NoError(t, err)
		defer f.Close()
		os.Stdout = f

		cfg := pipeline.Config{
			LogFile:     logfile,
			ConduitArgs: &conduit.Args{ConduitDataDir: t.TempDir()},
			Importer:    pipeline.NameConfigPair{Name: "test", Config: map[string]interface{}{"a": "a"}},
			Processors:  nil,
			Exporter:    pipeline.NameConfigPair{Name: "test", Config: map[string]interface{}{"a": "a"}},
		}
		data, err := yaml.Marshal(&cfg)
		require.NoError(t, err)
		configFile := path.Join(cfg.ConduitArgs.ConduitDataDir, conduit.DefaultConfigName)
		os.WriteFile(configFile, data, 0755)
		require.FileExists(t, configFile)

		err = runConduitCmdWithConfig(cfg.ConduitArgs)
		return os.ReadFile(stdoutFilePath)
	}

	// logging to stdout
	t.Run("conduit-logging-stdout", func(t *testing.T) {
		data, err := test(t, "")
		require.NoError(t, err)
		dataStr := string(data)
		require.Contains(t, dataStr, "{")
	})

	// logging to file
	t.Run("conduit-logging-file", func(t *testing.T) {
		logfile := path.Join(t.TempDir(), "logfile.txt")
		data, err := test(t, logfile)
		require.NoError(t, err)
		dataStr := string(data)
		require.NotContains(t, dataStr, "{")
		logdata, err := os.ReadFile(logfile)
		require.NoError(t, err)
		logdataStr := string(logdata)
		require.Contains(t, logdataStr, "{")
	})
}
