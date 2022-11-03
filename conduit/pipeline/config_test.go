package pipeline

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigString test basic configuration stringification
func TestConfigString(t *testing.T) {
	dataDir := "/tmp/dd"
	cfg := Config{ConduitDataDir: dataDir}

	outputStr := fmt.Sprintf("Data Directory: %s ", dataDir)
	require.Equal(t, cfg.String(), outputStr)
}

// TestConfigValid tests validity scenarios for config
func TestConfigValid(t *testing.T) {
	tests := []struct {
		name    string
		dataDir string
		err     error
	}{
		{"valid", t.TempDir(), nil},
		{"nil data dir", "", fmt.Errorf("supplied data directory was empty")},
		{"invalid data dir", "/tmp/this_directory_should_not_exist", fmt.Errorf("supplied data directory (/tmp/this_directory_should_not_exist) was not valid")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{ConduitDataDir: test.dataDir}
			assert.Equal(t, test.err, cfg.Valid())
		})
	}
}
