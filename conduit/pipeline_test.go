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
	"github.com/algorand/indexer/plugins"
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
			PipelineLogLevel: "info",
			Importer:         NameConfigPair{"test", map[string]interface{}{"a": "a"}},
			Processors:       nil,
			Exporter:         NameConfigPair{"test", map[string]interface{}{"a": "a"}},
		}, ""},

		{"valid 2", PipelineConfig{
			ConduitConfig:    &Config{ConduitDataDir: ""},
			PipelineLogLevel: "info",
			Importer:         NameConfigPair{"test", map[string]interface{}{"a": "a"}},
			Processors:       []NameConfigPair{{"test", map[string]interface{}{"a": "a"}}},
			Exporter:         NameConfigPair{"test", map[string]interface{}{"a": "a"}},
		}, ""},

		{"empty config", PipelineConfig{ConduitConfig: nil}, "PipelineConfig.Valid(): conduit configuration was nil"},
		{"invalid log level", PipelineConfig{ConduitConfig: &Config{ConduitDataDir: ""}, PipelineLogLevel: "asdf"}, "PipelineConfig.Valid(): pipeline log level (asdf) was invalid:"},
		{"importer config was 0",
			PipelineConfig{
				ConduitConfig:    &Config{ConduitDataDir: ""},
				PipelineLogLevel: "info",
				Importer:         NameConfigPair{"test", map[string]interface{}{}},
			}, "PipelineConfig.Valid(): importer configuration was empty"},

		{"exporter config was 0",
			PipelineConfig{
				ConduitConfig:    &Config{ConduitDataDir: ""},
				PipelineLogLevel: "info",
				Importer:         NameConfigPair{"test", map[string]interface{}{"a": "a"}},
				Processors:       []NameConfigPair{{"test", map[string]interface{}{"a": "a"}}},
				Exporter:         NameConfigPair{"test", map[string]interface{}{}},
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
log-level: info
importer:
  name: "algod"
  config:
    netaddr: "http://127.0.0.1:8080"
    token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
processors:
  - name: "noop"
    config:
      catchpoint: "7560000#3OUX3TLXZNOK6YJXGETKRRV2MHMILF5CCIVZUOJCT6SLY5H2WWTQ"
exporter:
  name: "noop"
  config:
    connectionstring: ""`

	err = os.WriteFile(filepath.Join(dataDir, DefaultConfigName), []byte(validConfigFile), 0777)
	assert.Nil(t, err)

	cfg := &Config{ConduitDataDir: dataDir}

	pCfg, err := MakePipelineConfig(l, cfg)
	assert.Nil(t, err)
	assert.Equal(t, pCfg.PipelineLogLevel, "info")
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
		fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", DefaultConfigName, cfgBad.ConduitDataDir))

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
	returnError bool
}

func (m *mockImporter) Init(_ context.Context, _ plugins.PluginConfig, _ *log.Logger) (*bookkeeping.Genesis, error) {
	return &bookkeeping.Genesis{}, nil
}

func (m *mockImporter) Close() error {
	return nil
}

func (m *mockImporter) Metadata() importers.ImporterMetadata {
	return importers.ImporterMetadata{ImpName: "mockImporter"}
}

func (m *mockImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var err error
	if m.returnError {
		err = fmt.Errorf("importer")
	}
	m.Called(rnd)
	// Return an error to make sure we
	return uniqueBlockData, err
}

type mockProcessor struct {
	mock.Mock
	processors.Processor
	finalRound      basics.Round
	returnError     bool
	onCompleteError bool
}

func (m *mockProcessor) Init(_ context.Context, _ data.InitProvider, _ plugins.PluginConfig, _ *log.Logger) error {
	return nil
}

func (m *mockProcessor) Close() error {
	return nil
}

func (m *mockProcessor) Metadata() processors.ProcessorMetadata {
	return processors.MakeProcessorMetadata("mockProcessor", "", false)
}

func (m *mockProcessor) Process(input data.BlockData) (data.BlockData, error) {
	var err error
	if m.returnError {
		err = fmt.Errorf("process")
	}
	m.Called(input)
	input.BlockHeader.Round++
	return input, err
}
func (m *mockProcessor) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = input.BlockHeader.Round
	m.Called(input)
	return err
}

type mockExporter struct {
	mock.Mock
	exporters.Exporter
	returnError bool
}

func (m *mockExporter) Metadata() exporters.ExporterMetadata {
	return exporters.ExporterMetadata{
		ExpName: "mockExporter",
	}
}

func (m *mockExporter) Init(_ context.Context, _ plugins.PluginConfig, _ *log.Logger) error {
	return nil
}

func (m *mockExporter) Close() error {
	return nil
}

func (m *mockExporter) HandleGenesis(_ bookkeeping.Genesis) error {
	return nil
}

func (m *mockExporter) Round() uint64 {
	return 0
}

func (m *mockExporter) Receive(exportData data.BlockData) error {
	var err error
	if m.returnError {
		err = fmt.Errorf("receive")
	}
	m.Called(exportData)
	return err
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
		cf:           cf,
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

	pImpl.Start()
	pImpl.Wait()
	assert.NoError(t, pImpl.Error())

	assert.Equal(t, mProcessor.finalRound, uniqueBlockData.BlockHeader.Round+1)

	mock.AssertExpectationsForObjects(t, &mImporter, &mProcessor, &mExporter)

}

// TestPipelineCpuPidFiles tests that cpu and pid files are created when specified
func TestPipelineCpuPidFiles(t *testing.T) {

	var pImporter importers.Importer = &mockImporter{}
	var pProcessor processors.Processor = &mockProcessor{}
	var pExporter exporters.Exporter = &mockExporter{}

	pidFilePath := filepath.Join(t.TempDir(), "pidfile")
	cpuFilepath := filepath.Join(t.TempDir(), "cpufile")

	pImpl := pipelineImpl{
		cfg: &PipelineConfig{
			Importer: NameConfigPair{
				Name:   "",
				Config: map[string]interface{}{},
			},
			Processors: []NameConfigPair{
				{
					Name:   "",
					Config: map[string]interface{}{},
				},
			},
			Exporter: NameConfigPair{
				Name:   "",
				Config: map[string]interface{}{},
			},
		},
		logger:       log.New(),
		initProvider: nil,
		importer:     &pImporter,
		processors:   []*processors.Processor{&pProcessor},
		exporter:     &pExporter,
		round:        0,
	}

	err := pImpl.Init()
	assert.NoError(t, err)

	// Test that file is not created
	_, err = os.Stat(pidFilePath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(cpuFilepath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	// Test that they were created

	pImpl.cfg.PIDFilePath = pidFilePath
	pImpl.cfg.CPUProfile = cpuFilepath

	err = pImpl.Init()
	assert.NoError(t, err)

	// Test that file is created
	_, err = os.Stat(cpuFilepath)
	assert.Nil(t, err)

	_, err = os.Stat(pidFilePath)
	assert.Nil(t, err)
}

// TestPipelineErrors tests the pipeline erroring out at different stages
func TestPipelineErrors(t *testing.T) {

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
		cf:           cf,
		cfg:          &PipelineConfig{},
		logger:       log.New(),
		initProvider: nil,
		importer:     &pImporter,
		processors:   []*processors.Processor{&pProcessor},
		exporter:     &pExporter,
		round:        0,
	}

	mImporter.returnError = true

	go pImpl.Start()
	time.Sleep(time.Millisecond)
	pImpl.cf()
	pImpl.Wait()
	assert.Error(t, pImpl.Error(), fmt.Errorf("importer"))

	mImporter.returnError = false
	mProcessor.returnError = true
	pImpl.ctx, pImpl.cf = context.WithCancel(context.Background())
	pImpl.setError(nil)
	go pImpl.Start()
	time.Sleep(time.Millisecond)
	pImpl.cf()
	pImpl.Wait()
	assert.Error(t, pImpl.Error(), fmt.Errorf("process"))

	mProcessor.returnError = false
	mProcessor.onCompleteError = true
	pImpl.ctx, pImpl.cf = context.WithCancel(context.Background())
	pImpl.setError(nil)
	go pImpl.Start()
	time.Sleep(time.Millisecond)
	pImpl.cf()
	pImpl.Wait()
	assert.Error(t, pImpl.Error(), fmt.Errorf("on complete"))

	mProcessor.onCompleteError = false
	mExporter.returnError = true
	pImpl.ctx, pImpl.cf = context.WithCancel(context.Background())
	pImpl.setError(nil)
	go pImpl.Start()
	time.Sleep(time.Millisecond)
	pImpl.cf()
	pImpl.Wait()
	assert.Error(t, pImpl.Error(), fmt.Errorf("exporter"))

}
