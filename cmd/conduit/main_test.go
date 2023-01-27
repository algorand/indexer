package main

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/pipeline"
	"github.com/algorand/indexer/loggers"
)

// TestInitDataDirectory tests the initialization of the data directory
func TestInitDataDirectory(t *testing.T) {
	verifyFile := func(file string) {
		require.FileExists(t, file)
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		require.Equal(t, sampleConfig, string(data))
	}

	// avoid clobbering an existing data directory
	defaultDataDirectory = "override"
	require.NoDirExists(t, defaultDataDirectory)

	runConduitInit("")
	verifyFile(fmt.Sprintf("%s/conduit.yml", defaultDataDirectory))

	runConduitInit(fmt.Sprintf("%s/provided_directory", defaultDataDirectory))
	verifyFile(fmt.Sprintf("%s/provided_directory/conduit.yml", defaultDataDirectory))

	os.RemoveAll(defaultDataDirectory)
}

func TestBanner(t *testing.T) {
	test := func(t *testing.T, hideBanner bool) {
		// Install test logger.
		var logbuilder strings.Builder
		loggerManager = loggers.MakeLoggerManager(&logbuilder)
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
