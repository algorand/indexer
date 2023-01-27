package blockprocessor

import (
	"context"
	"fmt"
	"path"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/processors"
	testutil "github.com/algorand/indexer/util/test"
)

func TestConfigDeserialize(t *testing.T) {

	configStr := `---
                  catchpoint: "acatch"
                  ledger-dir: "idx_data_dir"
                  algod-data-dir: "algod_data_dir"
                  algod-token: "algod_token"
                  algod-addr: "algod_addr"
 `

	var processorConfig Config
	err := yaml.Unmarshal([]byte(configStr), &processorConfig)
	require.Nil(t, err)
	require.Equal(t, processorConfig.Catchpoint, "acatch")
	require.Equal(t, processorConfig.LedgerDir, "idx_data_dir")
	require.Equal(t, processorConfig.AlgodDataDir, "algod_data_dir")
	require.Equal(t, processorConfig.AlgodToken, "algod_token")
	require.Equal(t, processorConfig.AlgodAddr, "algod_addr")
}

var cons = processors.ProcessorConstructorFunc(func() processors.Processor {
	return &blockProcessor{}
})

func TestInitDefaults(t *testing.T) {
	tempdir := t.TempDir()
	override := path.Join(tempdir, "override")
	var round = basics.Round(0)
	ip := testutil.MockedInitProvider(&round)
	var addr basics.Address
	ip.Genesis = &sdk.Genesis{
		SchemaID:    "test",
		Network:     "test",
		Proto:       "future",
		Allocation:  nil,
		RewardsPool: addr.String(),
		FeeSink:     addr.String(),
		Timestamp:   1234,
	}
	logger, _ := test.NewNullLogger()

	testcases := []struct {
		ledgerdir string
		expected  string
	}{
		{
			ledgerdir: "",
			expected:  tempdir,
		},
		{
			ledgerdir: "''",
			expected:  tempdir,
		},
		{
			ledgerdir: override,
			expected:  override,
		},
		{
			ledgerdir: fmt.Sprintf("'%s'", override),
			expected:  override,
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(fmt.Sprintf("ledgerdir: %s", tc.ledgerdir), func(t *testing.T) {
			t.Parallel()
			proc := cons.New()
			defer proc.Close()
			pcfg := plugins.MakePluginConfig(fmt.Sprintf("ledger-dir: %s", tc.ledgerdir))
			pcfg.DataDir = tempdir
			err := proc.Init(context.Background(), ip, pcfg, logger)
			require.NoError(t, err)
			pluginConfig := proc.Config()
			assert.Contains(t, pluginConfig, fmt.Sprintf("ledger-dir: %s", tc.expected))
		})
	}
}
