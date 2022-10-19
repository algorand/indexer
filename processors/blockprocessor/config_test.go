package blockprocessor

import (
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestConfigDeserialize(t *testing.T) {

	configStr := `---
                  catchpoint: "acatch"
                  data-dir: "idx_data_dir"
                  algod-data-dir: "algod_data_dir"
                  algod-token: "algod_token"
                  algod-addr: "algod_addr"
 `

	var processorConfig Config
	err := yaml.Unmarshal([]byte(configStr), &processorConfig)
	require.Nil(t, err)
	require.Equal(t, processorConfig.Catchpoint, "acatch")
	require.Equal(t, processorConfig.IndexerDatadir, "idx_data_dir")
	require.Equal(t, processorConfig.AlgodDataDir, "algod_data_dir")
	require.Equal(t, processorConfig.AlgodToken, "algod_token")
	require.Equal(t, processorConfig.AlgodAddr, "algod_addr")

}
