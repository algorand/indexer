package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/processor/blockprocessor"
	itest "github.com/algorand/indexer/util/test"
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
	ledgerLogger, _ := test.NewNullLogger()
	l, err := itest.MakeTestLedger(ledgerLogger)
	assert.NoError(t, err)
	defer l.Close()
	proc, err := blockprocessor.MakeProcessorWithLedger(logger, l, nil)
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

func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "indexer")
	if err != nil {
		t.Fatalf(err.Error())
	}
	return dir
}

// TestParameterConfigErrorWhenBothFileTypesArePresent test that if both file types are there then it is an error
func TestParameterConfigErrorWhenBothFileTypesArePresent(t *testing.T) {

	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	for _, configFiletype := range config.FileTypes {
		autoloadPath := filepath.Join(indexerDataDir, autoLoadParameterConfigFileName+"."+configFiletype)
		os.WriteFile(autoloadPath, []byte{}, fs.ModePerm)
	}

	daemonConfig := &daemonConfig{}
	daemonConfig.flags = pflag.NewFlagSet("indexer", 0)
	daemonConfig.indexerDataDir = indexerDataDir
	err := runDaemon(daemonConfig)
	errorStr := fmt.Errorf("config filename (%s) in data directory (%s) matched more than one filetype: %v",
		autoLoadParameterConfigFileName, indexerDataDir, config.FileTypes)
	assert.EqualError(t, err, errorStr.Error())
}

// TestIndexerConfigErrorWhenBothFileTypesArePresent test that if both file types are there then it is an error
func TestIndexerConfigErrorWhenBothFileTypesArePresent(t *testing.T) {

	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	for _, configFiletype := range config.FileTypes {
		autoloadPath := filepath.Join(indexerDataDir, autoLoadIndexerConfigFileName+"."+configFiletype)
		os.WriteFile(autoloadPath, []byte{}, fs.ModePerm)
	}

	daemonConfig := &daemonConfig{}
	daemonConfig.flags = pflag.NewFlagSet("indexer", 0)
	daemonConfig.indexerDataDir = indexerDataDir
	err := runDaemon(daemonConfig)
	errorStr := fmt.Errorf("config filename (%s) in data directory (%s) matched more than one filetype: %v",
		autoLoadIndexerConfigFileName, indexerDataDir, config.FileTypes)
	assert.EqualError(t, err, errorStr.Error())
}

// Make sure we output and return an error when both an API Config and
// enable all parameters are provided together.
func TestConfigWithEnableAllParamsExpectError(t *testing.T) {
	for _, configFiletype := range config.FileTypes {
		indexerDataDir := createTempDir(t)
		defer os.RemoveAll(indexerDataDir)
		autoloadPath := filepath.Join(indexerDataDir, autoLoadIndexerConfigFileName+"."+configFiletype)
		os.WriteFile(autoloadPath, []byte{}, fs.ModePerm)
		daemonConfig := &daemonConfig{}
		daemonConfig.flags = pflag.NewFlagSet("indexer", 0)
		daemonConfig.indexerDataDir = indexerDataDir
		daemonConfig.enableAllParameters = true
		daemonConfig.suppliedAPIConfigFile = "foobar"
		err := runDaemon(daemonConfig)
		errorStr := "not allowed to supply an api config file and enable all parameters"
		assert.EqualError(t, err, errorStr)
	}
}

func TestConfigDoesNotExistExpectError(t *testing.T) {
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	tempConfigFile := indexerDataDir + "/indexer.yml"
	daemonConfig := &daemonConfig{}
	daemonConfig.flags = pflag.NewFlagSet("indexer", 0)
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.configFile = tempConfigFile
	err := runDaemon(daemonConfig)
	// This error string is probably OS-specific
	errorStr := fmt.Sprintf("open %s: no such file or directory", tempConfigFile)
	assert.EqualError(t, err, errorStr)
}

func TestConfigInvalidExpectError(t *testing.T) {
	b := bytes.NewBufferString("")
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	tempConfigFile := indexerDataDir + "/indexer-alt.yml"
	os.WriteFile(tempConfigFile, []byte(";;;"), fs.ModePerm)
	daemonConfig := &daemonConfig{}
	daemonConfig.flags = pflag.NewFlagSet("indexer", 0)
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.configFile = tempConfigFile
	logger.SetOutput(b)
	err := runDaemon(daemonConfig)
	errorStr := "While parsing config: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `;;;` into map[string]interface {}"
	assert.EqualError(t, err, errorStr)
}

func TestConfigSpecifiedTwiceExpectError(t *testing.T) {
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)
	tempConfigFile := indexerDataDir + "/indexer.yml"
	os.WriteFile(tempConfigFile, []byte{}, fs.ModePerm)
	daemonConfig := &daemonConfig{}
	daemonConfig.flags = pflag.NewFlagSet("indexer", 0)
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.configFile = tempConfigFile
	err := runDaemon(daemonConfig)
	errorStr := fmt.Sprintf("indexer configuration was found in data directory (%s) as well as supplied via command line.  Only provide one",
		filepath.Join(indexerDataDir, "indexer.yml"))
	assert.EqualError(t, err, errorStr)
}

func TestLoadAPIConfigGivenAutoLoadAndUserSuppliedExpectError(t *testing.T) {

	for _, configFiletype := range config.FileTypes {
		indexerDataDir := createTempDir(t)
		defer os.RemoveAll(indexerDataDir)

		autoloadPath := filepath.Join(indexerDataDir, autoLoadParameterConfigFileName+"."+configFiletype)
		userSuppliedPath := filepath.Join(indexerDataDir, "foobar.yml")
		os.WriteFile(autoloadPath, []byte{}, fs.ModePerm)
		cfg := &daemonConfig{}
		cfg.indexerDataDir = indexerDataDir
		cfg.suppliedAPIConfigFile = userSuppliedPath

		err := loadIndexerParamConfig(cfg)
		errorStr := fmt.Sprintf("api parameter configuration was found in data directory (%s) as well as supplied via command line.  Only provide one",
			autoloadPath)
		assert.EqualError(t, err, errorStr)
	}
}

func TestLoadAPIConfigGivenUserSuppliedExpectSuccess(t *testing.T) {
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)

	userSuppliedPath := filepath.Join(indexerDataDir, "foobar.yml")
	cfg := &daemonConfig{}
	cfg.indexerDataDir = indexerDataDir
	cfg.suppliedAPIConfigFile = userSuppliedPath

	err := loadIndexerParamConfig(cfg)
	assert.NoError(t, err)
}

func TestLoadAPIConfigGivenAutoLoadExpectSuccess(t *testing.T) {
	for _, configFiletype := range config.FileTypes {
		indexerDataDir := createTempDir(t)
		defer os.RemoveAll(indexerDataDir)

		autoloadPath := filepath.Join(indexerDataDir, autoLoadParameterConfigFileName+"."+configFiletype)
		os.WriteFile(autoloadPath, []byte{}, fs.ModePerm)
		cfg := &daemonConfig{}
		cfg.indexerDataDir = indexerDataDir

		err := loadIndexerParamConfig(cfg)
		assert.NoError(t, err)
		assert.Equal(t, autoloadPath, cfg.suppliedAPIConfigFile)
	}
}

func TestIndexerDataDirNotProvidedExpectError(t *testing.T) {
	errorStr := "indexer data directory was not provided"

	assert.EqualError(t, configureIndexerDataDir(""), errorStr)
}

func TestIndexerDataDirCreateFailExpectError(t *testing.T) {
	invalidDir := filepath.Join("foo", "bar")

	assert.Error(t, configureIndexerDataDir(invalidDir))
}

func TestIndexerPidFileExpectSuccess(t *testing.T) {
	indexerDataDir := createTempDir(t)
	defer os.RemoveAll(indexerDataDir)

	pidFilePath := path.Join(indexerDataDir, "pidFile")
	assert.NoError(t, createIndexerPidFile(pidFilePath))
}

func TestIndexerPidFileCreateFailExpectError(t *testing.T) {
	for _, configFiletype := range config.FileTypes {
		indexerDataDir := createTempDir(t)
		defer os.RemoveAll(indexerDataDir)
		autoloadPath := filepath.Join(indexerDataDir, autoLoadIndexerConfigFileName+"."+configFiletype)
		os.WriteFile(autoloadPath, []byte{}, fs.ModePerm)

		invalidDir := filepath.Join(indexerDataDir, "foo", "bar")
		cfg := &daemonConfig{}
		cfg.pidFilePath = invalidDir

		cfg.flags = pflag.NewFlagSet("indexer", 0)
		cfg.indexerDataDir = indexerDataDir

		assert.ErrorContains(t, runDaemon(cfg), "pid file")
		assert.Error(t, createIndexerPidFile(cfg.pidFilePath))
	}
}
