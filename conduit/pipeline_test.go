package conduit

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/processors"
)

// TestPipelineConfigValidity tests the Valid() function for the PipelineConfig
func TestPipelineConfigValidity(t *testing.T) {
	tests := []struct {
		name        string
		toTest      PipelineConfig
		errContains string
	}{
		{"valid", PipelineConfig{
			ConduitConfig:    &Config{ConduitDataDir: ""},
			pipelineLogLevel: "info",
			Importer:         nameConfigPair{"test", map[string]interface{}{"a": "a"}},
			Processors:       nil,
			Exporter:         nameConfigPair{"test", map[string]interface{}{"a": "a"}},
		}, ""},

		{"valid 2", PipelineConfig{
			ConduitConfig:    &Config{ConduitDataDir: ""},
			pipelineLogLevel: "info",
			Importer:         nameConfigPair{"test", map[string]interface{}{"a": "a"}},
			Processors:       []nameConfigPair{{"test", map[string]interface{}{"a": "a"}}},
			Exporter:         nameConfigPair{"test", map[string]interface{}{"a": "a"}},
		}, ""},

		{"empty config", PipelineConfig{ConduitConfig: nil}, "PipelineConfig.Valid(): conduit configuration was nil"},
		{"invalid log level", PipelineConfig{ConduitConfig: &Config{ConduitDataDir: ""}, pipelineLogLevel: "asdf"}, "PipelineConfig.Valid(): pipeline log level (asdf) was invalid:"},
		{"importer config was 0",
			PipelineConfig{
				ConduitConfig:    &Config{ConduitDataDir: ""},
				pipelineLogLevel: "info",
				Importer:         nameConfigPair{"test", map[string]interface{}{}},
			}, "PipelineConfig.Valid(): importer configuration was empty"},

		{"exporter config was 0",
			PipelineConfig{
				ConduitConfig:    &Config{ConduitDataDir: ""},
				pipelineLogLevel: "info",
				Importer:         nameConfigPair{"test", map[string]interface{}{"a": "a"}},
				Processors:       []nameConfigPair{{"test", map[string]interface{}{"a": "a"}}},
				Exporter:         nameConfigPair{"test", map[string]interface{}{}},
			}, "PipelineConfig.Valid(): exporter configuration was empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.toTest.Valid()

			if test.errContains == "" {
				assert.Nil(t, err)
				return
			}

			assert.Contains(t, err.Error(), test.errContains)
		})
	}
}

// TestMakePipelineConfig tests making the pipeline configuration
func TestMakePipelineConfig(t *testing.T) {

	l := log.New()

	_, err := MakePipelineConfig(l, nil)
	assert.Equal(t, fmt.Errorf("MakePipelineConfig(): empty conduit config"), err)

	// "" for dir will use os.TempDir()
	dataDir, err := ioutil.TempDir("", "conduit_data_dir")
	assert.Nil(t, err)
	defer os.RemoveAll(dataDir)

	validConfigFile := `---
LogLevel: info
Importer:
  Name: "algod"
  Config:
    netaddr: "http://127.0.0.1:8080"
    token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
Processors:
  - Name: "noop"
    Config:
      catchpoint: "7560000#3OUX3TLXZNOK6YJXGETKRRV2MHMILF5CCIVZUOJCT6SLY5H2WWTQ"
Exporter:
  Name: "noop"
  Config:
    connectionstring: ""`

	err = os.WriteFile(filepath.Join(dataDir, autoLoadParameterConfigName), []byte(validConfigFile), 0777)
	assert.Nil(t, err)

	cfg := &Config{ConduitDataDir: dataDir}

	pCfg, err := MakePipelineConfig(l, cfg)
	assert.Nil(t, err)
	assert.Equal(t, pCfg.PipelineLogLevel, log.InfoLevel)
	assert.Equal(t, pCfg.Valid(), nil)
	assert.Equal(t, pCfg.Importer.Name, "algod")
	assert.Equal(t, pCfg.Importer.Config["token"], "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302")
	assert.Equal(t, pCfg.Processors[0].Name, "noop")
	assert.Equal(t, pCfg.Processors[0].Config["catchpoint"], "7560000#3OUX3TLXZNOK6YJXGETKRRV2MHMILF5CCIVZUOJCT6SLY5H2WWTQ")
	assert.Equal(t, pCfg.Exporter.Name, "noop")
	assert.Equal(t, pCfg.Exporter.Config["connectionstring"], "")

	// "" for dir will use os.TempDir()
	// invalidDataDir has no auto load file
	invalidDataDir, err := ioutil.TempDir("", "conduit_data_dir")
	assert.Nil(t, err)
	defer os.RemoveAll(invalidDataDir)

	cfgBad := &Config{ConduitDataDir: invalidDataDir}
	_, err = MakePipelineConfig(l, cfgBad)
	assert.Equal(t, err,
		fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", autoLoadParameterConfigName, cfgBad.ConduitDataDir))

}

// a unique block data to validate with tests
var uniqueBlockData = data.BlockData{
	BlockHeader: bookkeeping.BlockHeader{
		Round: 1337,
	},
}

type mockImporter struct {
	mock.Mock
	importers.Importer
}

func (m *mockImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	m.Called(rnd)
	return uniqueBlockData, nil
}

type mockProcessor struct {
	mock.Mock
	processors.Processor
	finalRound basics.Round
}

func (m *mockProcessor) Process(input data.BlockData) (data.BlockData, error) {
	m.Called(input)
	input.BlockHeader.Round++
	return input, nil
}
func (m *mockProcessor) OnComplete(input data.BlockData) error {
	m.finalRound = input.BlockHeader.Round
	m.Called(input)
	return nil
}

type mockExporter struct {
	mock.Mock
	exporters.Exporter
}

func (m *mockExporter) Receive(exportData data.BlockData) error {
	m.Called(exportData)
	return nil
}

// TestPipelineRun tests that running the pipeline calls the correct functions with mocking
func TestPipelineRun(t *testing.T) {

	mImporter := mockImporter{}
	mImporter.On("GetBlock", mock.Anything).Return(uniqueBlockData, nil)
	mProcessor := mockProcessor{}
	processorData := uniqueBlockData
	processorData.BlockHeader.Round++
	mProcessor.On("Process", mock.Anything).Return(processorData)
	mProcessor.On("OnComplete", mock.Anything).Return(nil)
	mExporter := mockExporter{}
	mExporter.On("Receive", mock.Anything).Return(nil)

	var pImporter importers.Importer = &mImporter
	var pProcessor processors.Processor = &mProcessor
	var pExporter exporters.Exporter = &mExporter

	ctx, cf := context.WithCancel(context.Background())

	pImpl := pipelineImpl{
		ctx:          ctx,
		cfg:          &PipelineConfig{},
		logger:       log.New(),
		initProvider: nil,
		importer:     &pImporter,
		processors:   []*processors.Processor{&pProcessor},
		exporter:     &pExporter,
		round:        0,
	}

	go func() {
		time.Sleep(1 * time.Second)
		cf()
	}()

	err := pImpl.RunPipeline()
	assert.Nil(t, err)

	mock.AssertExpectationsForObjects(t, &mImporter, &mProcessor, &mExporter)

}
