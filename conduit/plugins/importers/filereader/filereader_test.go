package fileimporter

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"

	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters/filewriter"
	"github.com/algorand/indexer/conduit/plugins/importers"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/util"
)

var (
	logger       *logrus.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	testImporter importers.Importer
)

func init() {
	logger = logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
}

func TestImporterorterMetadata(t *testing.T) {
	testImporter = New()
	m := testImporter.Metadata()
	assert.Equal(t, metadata.Name, m.Name)
	assert.Equal(t, metadata.Description, m.Description)
	assert.Equal(t, metadata.Deprecated, m.Deprecated)
}

// initializeTestData fills a data directory with some dummy data for the importer to read.
func initializeTestData(t *testing.T, dir string, numRounds int) sdk.Genesis {
	genesisA := sdk.Genesis{
		SchemaID:    "test",
		Network:     "test",
		Proto:       "test",
		Allocation:  nil,
		RewardsPool: "AAAAAAA",
		FeeSink:     "AAAAAAA",
		Timestamp:   1234,
	}

	err := util.EncodeToFile(path.Join(dir, "genesis.json"), genesisA, true)
	require.NoError(t, err)

	for i := 0; i < numRounds; i++ {
		block := data.BlockData{
			BlockHeader: bookkeeping.BlockHeader{
				Round: basics.Round(i),
			},
			Payset:      make([]transactions.SignedTxnInBlock, 0),
			Delta:       &ledgercore.StateDelta{},
			Certificate: nil,
		}
		blockFile := path.Join(dir, fmt.Sprintf(filewriter.FilePattern, i))
		err = util.EncodeToFile(blockFile, block, true)
		require.NoError(t, err)
	}

	return genesisA
}

func initializeImporter(t *testing.T, numRounds int) (importer importers.Importer, tempdir string, genesis *sdk.Genesis, err error) {
	tempdir = t.TempDir()
	genesisExpected := initializeTestData(t, tempdir, numRounds)
	importer = New()
	defer importer.Config()
	cfg := Config{
		BlocksDir:     tempdir,
		RetryDuration: 0,
	}
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	genesis, err = importer.Init(context.Background(), plugins.MakePluginConfig(string(data)), logger)
	assert.NoError(t, err)
	require.NotNil(t, genesis)
	require.Equal(t, genesisExpected, *genesis)
	return
}

func TestInitSuccess(t *testing.T) {
	_, _, _, err := initializeImporter(t, 1)
	require.NoError(t, err)
}

func TestInitUnmarshalFailure(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(context.Background(), plugins.MakePluginConfig("`"), logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "invalid configuration")
	testImporter.Close()
}

func TestConfigDefault(t *testing.T) {
	testImporter = New()
	expected, err := yaml.Marshal(&Config{})
	require.NoError(t, err)
	assert.Equal(t, string(expected), testImporter.Config())
}

func TestGetBlockSuccess(t *testing.T) {
	numRounds := 10
	importer, tempdir, genesis, err := initializeImporter(t, 10)
	require.NoError(t, err)
	require.NotEqual(t, "", tempdir)
	require.NotNil(t, genesis)

	for i := 0; i < numRounds; i++ {
		block, err := importer.GetBlock(uint64(i))
		require.NoError(t, err)
		require.Equal(t, basics.Round(i), block.BlockHeader.Round)
	}
}

func TestRetryAndDuration(t *testing.T) {
	tempdir := t.TempDir()
	initializeTestData(t, tempdir, 0)
	importer := New()
	defer importer.Config()
	cfg := Config{
		BlocksDir:     tempdir,
		RetryDuration: 10 * time.Millisecond,
		RetryCount:    3,
	}
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	_, err = importer.Init(context.Background(), plugins.MakePluginConfig(string(data)), logger)
	assert.NoError(t, err)

	start := time.Now()
	_, err = importer.GetBlock(0)
	assert.ErrorContains(t, err, "GetBlock(): block not found after (3) attempts")

	expectedDuration := cfg.RetryDuration*time.Duration(cfg.RetryCount) + 10*time.Millisecond
	assert.WithinDuration(t, start, time.Now(), expectedDuration, "Error should generate after retry count * retry duration")
}

func TestRetryWithCancel(t *testing.T) {
	tempdir := t.TempDir()
	initializeTestData(t, tempdir, 0)
	importer := New()
	defer importer.Config()
	cfg := Config{
		BlocksDir:     tempdir,
		RetryDuration: 1 * time.Hour,
		RetryCount:    3,
	}
	data, err := yaml.Marshal(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, err)
	_, err = importer.Init(ctx, plugins.MakePluginConfig(string(data)), logger)
	assert.NoError(t, err)

	// Cancel after delay
	delay := time.Millisecond
	go func() {
		time.Sleep(delay)
		cancel()
	}()
	start := time.Now()
	_, err = importer.GetBlock(0)
	assert.ErrorContains(t, err, "GetBlock() context finished: context canceled")

	// within 1ms of the expected time (but much less than the 3hr configuration.
	assert.WithinDuration(t, start, time.Now(), delay+time.Millisecond)
}
