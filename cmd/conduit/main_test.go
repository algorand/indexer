package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
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
