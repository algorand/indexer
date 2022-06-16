package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/indexer/processor/blockprocessor"
	itest "github.com/algorand/indexer/util/test"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

type mockImporter struct {
}

var errMockImportBlock = errors.New("Process() invalid round blockCert.Block.Round(): 1234 nextRoundToProcess: 1")

func (imp *mockImporter) ImportBlock(vb *ledgercore.ValidatedBlock) error {
	return nil
}

func TestImportRetryAndCancel(t *testing.T) {
	// connect debug logger
	nullLogger, hook := test.NewNullLogger()
	logger = nullLogger

	// cancellable context
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)

	// create handler with mock importer and start, it should generate errors until cancelled.
	imp := &mockImporter{}
	l := itest.MakeTestLedger("ledger")
	defer l.Close()
	proc, err := blockprocessor.MakeProcessorWithLedger(l, nil)
	assert.Nil(t, err)
	proc.SetHandler(imp.ImportBlock)
	handler := blockHandler(proc, 50*time.Millisecond)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		block := rpcs.EncodedBlockCert{
			Block: bookkeeping.Block{
				BlockHeader: bookkeeping.BlockHeader{
					Round: 1234,
				},
			},
		}
		handler(cctx, &block)
	}()

	// accumulate some errors
	for len(hook.Entries) < 5 {
		time.Sleep(25 * time.Millisecond)
	}

	for _, entry := range hook.Entries {
		assert.Equal(t, entry.Message, "block 1234 import failed")
		assert.Equal(t, entry.Data["error"], errMockImportBlock)
	}

	// Wait for handler to exit.
	cancel()
	wg.Wait()
}

func TestReadGenesis(t *testing.T) {
	var reader io.Reader
	// nil reader
	_, err := readGenesis(reader)
	assert.Contains(t, err.Error(), "readGenesis() err: reader is nil")
	// no match struct field
	genesisStr := "{\"version\": 2}"
	reader = strings.NewReader(genesisStr)
	_, err = readGenesis(reader)
	assert.Contains(t, err.Error(), "json decode error")

	genesis := bookkeeping.Genesis{
		SchemaID:    "1",
		Network:     "test",
		Proto:       "test",
		RewardsPool: "AAAA",
		FeeSink:     "AAAA",
	}

	// read and decode genesis
	reader = strings.NewReader(string(json.Encode(genesis)))
	_, err = readGenesis(reader)
	assert.Nil(t, err)
	// read from empty reader
	_, err = readGenesis(reader)
	assert.Contains(t, err.Error(), "readGenesis() err: EOF")

}

func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "indexer")
	if err != nil {
		t.Fatalf(err.Error())
	}
	return dir
}

// Make sure we output and return an error when both an API Config and
// enable all parameters are provided together.
func TestConfigWithEnableAllParamsExpectError(t *testing.T) {
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	daemonConfig := newDaemonConfig()
	cmd := newDaemonCmd(daemonConfig)
	daemonConfig.flags = cmd.Flags()
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.enableAllParameters = true
	daemonConfig.suppliedAPIConfigFile = "foobar"
	err := runDaemon(daemonConfig)
	errorStr := "not allowed to supply an api config file and enable all parameters"
	if err.Error() != errorStr {
		t.Fatalf("expected error %s, but got %s", errorStr, err.Error())
	}
}

func TestConfigDoesNotExistExpectError(t *testing.T) {
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	tempConfigFile := indexerDataDir + "/indexer.yml"
	daemonConfig := newDaemonConfig()
	cmd := newDaemonCmd(daemonConfig)
	daemonConfig.flags = cmd.Flags()
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.configFile = tempConfigFile
	err := runDaemon(daemonConfig)
	// This error string is probably OS-specific
	errorStr := "no such file or directory"
	if !strings.Contains(err.Error(), errorStr) {
		t.Fatalf("expected error %s, but got %s", errorStr, err.Error())
	}
}

func TestConfigInvalidExpectError(t *testing.T) {
	b := bytes.NewBufferString("")
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	tempConfigFile := indexerDataDir + "/indexer-alt.yml"
	os.WriteFile(tempConfigFile, []byte(";;;"), fs.ModePerm)
	daemonConfig := newDaemonConfig()
	cmd := newDaemonCmd(daemonConfig)
	daemonConfig.flags = cmd.Flags()
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.configFile = tempConfigFile
	logger.SetOutput(b)
	// Should assert this is an error even if it's not one we directly control (ours are wrapped)
	_ = runDaemon(daemonConfig)
	errorStr := "invalid config file"
	logs := b.String()
	if !strings.Contains(logs, errorStr) {
		t.Fatalf("expected error to contain %s, but got %s", errorStr, logs)
	}
}

func TestConfigSpecifiedTwiceExpectError(t *testing.T) {
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	tempConfigFile := indexerDataDir + "/indexer.yml"
	os.WriteFile(tempConfigFile, []byte{}, fs.ModePerm)
	daemonConfig := newDaemonConfig()
	cmd := newDaemonCmd(daemonConfig)
	daemonConfig.flags = cmd.Flags()
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.configFile = tempConfigFile
	err := runDaemon(daemonConfig)
	expectedError := fmt.Errorf("indexer configuration was found in data directory (%s) as well as supplied via command line.  Only provide one.",
		filepath.Join(indexerDataDir, "indexer.yml"))
	if err.Error() != expectedError.Error() {
		t.Fatalf("expected error %v, but got %v", expectedError, err)
	}
}
