package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/indexer/config"
	"github.com/algorand/indexer/util"
)

// TestParameterConfigErrorWhenBothFileTypesArePresent test that if both file types are there then it is an error
func TestParameterConfigErrorWhenBothFileTypesArePresent(t *testing.T) {

	indexerDataDir := t.TempDir()
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

	indexerDataDir := t.TempDir()
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
		indexerDataDir := t.TempDir()
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
	indexerDataDir := t.TempDir()
	tempConfigFile := indexerDataDir + "/indexer.yml"
	daemonConfig := &daemonConfig{}
	daemonConfig.flags = pflag.NewFlagSet("indexer", 0)
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.configFile = tempConfigFile
	err := runDaemon(daemonConfig)
	// This error string is probably OS-specific
	errorStr := fmt.Sprintf("config file does not exist: open %s: no such file or directory", tempConfigFile)
	assert.EqualError(t, err, errorStr)
}

func TestConfigInvalidExpectError(t *testing.T) {
	b := bytes.NewBufferString("")
	indexerDataDir := t.TempDir()
	tempConfigFile := indexerDataDir + "/indexer-alt.yml"
	os.WriteFile(tempConfigFile, []byte(";;;"), fs.ModePerm)
	daemonConfig := &daemonConfig{}
	daemonConfig.flags = pflag.NewFlagSet("indexer", 0)
	daemonConfig.indexerDataDir = indexerDataDir
	daemonConfig.configFile = tempConfigFile
	logger.SetOutput(b)
	err := runDaemon(daemonConfig)
	errorStr := fmt.Sprintf("invalid config file (%s): While parsing config: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `;;;` into map[string]interface {}", tempConfigFile)
	assert.EqualError(t, err, errorStr)
}

func TestConfigSpecifiedTwiceExpectError(t *testing.T) {
	indexerDataDir := t.TempDir()
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
		indexerDataDir := t.TempDir()

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
	indexerDataDir := t.TempDir()

	userSuppliedPath := filepath.Join(indexerDataDir, "foobar.yml")
	cfg := &daemonConfig{}
	cfg.indexerDataDir = indexerDataDir
	cfg.suppliedAPIConfigFile = userSuppliedPath

	err := loadIndexerParamConfig(cfg)
	assert.NoError(t, err)
}

func TestLoadAPIConfigGivenAutoLoadExpectSuccess(t *testing.T) {
	for _, configFiletype := range config.FileTypes {
		indexerDataDir := t.TempDir()

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
	assert.NoError(t, configureIndexerDataDir(""))
}

func TestIndexerDataDirCreateFailExpectError(t *testing.T) {
	invalidDir := filepath.Join("foo", "bar")

	assert.Error(t, configureIndexerDataDir(invalidDir))
}

func TestIndexerPidFileExpectSuccess(t *testing.T) {
	indexerDataDir := t.TempDir()

	pidFilePath := path.Join(indexerDataDir, "pidFile")
	assert.NoError(t, util.CreateIndexerPidFile(log.New(), pidFilePath))
}

func TestIndexerPidFileCreateFailExpectError(t *testing.T) {
	for _, configFiletype := range config.FileTypes {
		indexerDataDir := t.TempDir()
		autoloadPath := filepath.Join(indexerDataDir, autoLoadIndexerConfigFileName+"."+configFiletype)
		os.WriteFile(autoloadPath, []byte{}, fs.ModePerm)

		invalidDir := filepath.Join(indexerDataDir, "foo", "bar")
		cfg := &daemonConfig{}
		cfg.pidFilePath = invalidDir

		cfg.flags = pflag.NewFlagSet("indexer", 0)
		cfg.indexerDataDir = indexerDataDir

		assert.ErrorContains(t, runDaemon(cfg), "pid file")
		assert.Error(t, util.CreateIndexerPidFile(log.New(), cfg.pidFilePath))
	}
}

func TestIndexerMissingDataDir(t *testing.T) {
	cfg := &daemonConfig{}
	cfg.flags = pflag.NewFlagSet("indexer", 0)
	assert.EqualError(t, runDaemon(cfg), "indexer data directory was not provided")
}

func TestOptionalIndexerDataDir(t *testing.T) {
	cfg := &daemonConfig{}
	cfg.flags = pflag.NewFlagSet("indexer", 0)
	cfg.noAlgod = true
	// gets to the error beyond the indexer data dir check.
	assert.EqualError(t, runDaemon(cfg), "no import db set")
}
