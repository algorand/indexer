package pipeline

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/algorand/go-algorand/crypto"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/conduit"
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
	genesis         bookkeeping.Genesis
	finalRound      basics.Round
	returnError     bool
	onCompleteError bool
}

func (m *mockImporter) Init(_ context.Context, _ plugins.PluginConfig, _ *log.Logger) (*bookkeeping.Genesis, error) {
	return &m.genesis, nil
}

func (m *mockImporter) Close() error {
	return nil
}

func (m *mockImporter) Metadata() conduit.Metadata {
	return conduit.Metadata{Name: "mockImporter"}
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

func (m *mockImporter) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = input.BlockHeader.Round
	m.Called(input)
	return err
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

func (m *mockProcessor) Metadata() conduit.Metadata {
	return conduit.Metadata{
		Name: "mockProcessor",
	}
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
	finalRound      basics.Round
	returnError     bool
	onCompleteError bool
}

func (m *mockExporter) Metadata() conduit.Metadata {
	return conduit.Metadata{
		Name: "mockExporter",
	}
}

func (m *mockExporter) Init(_ context.Context, _ data.InitProvider, _ plugins.PluginConfig, _ *log.Logger) error {
	return nil
}

func (m *mockExporter) Close() error {
	return nil
}

func (m *mockExporter) Receive(exportData data.BlockData) error {
	var err error
	if m.returnError {
		err = fmt.Errorf("receive")
	}
	m.Called(exportData)
	return err
}

func (m *mockExporter) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = input.BlockHeader.Round
	m.Called(input)
	return err
}

type mockedImporterNew struct{}

func (c mockedImporterNew) New() importers.Importer { return &mockImporter{} }

type mockedExporterNew struct{}

func (c mockedExporterNew) New() exporters.Exporter { return &mockExporter{} }

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
	var cbComplete conduit.Completed = &mProcessor

	ctx, cf := context.WithCancel(context.Background())

	pImpl := pipelineImpl{
		ctx:              ctx,
		cf:               cf,
		logger:           log.New(),
		initProvider:     nil,
		importer:         &pImporter,
		processors:       []*processors.Processor{&pProcessor},
		exporter:         &pExporter,
		completeCallback: []conduit.OnCompleteFunc{cbComplete.OnComplete},
		pipelineMetadata: PipelineMetaData{
			NextRound:   0,
			GenesisHash: "",
		},
		pipelineMetadataFilePath: filepath.Join(t.TempDir(), "metadata.json"),
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
			ConduitConfig: &Config{
				Flags:          nil,
				ConduitDataDir: t.TempDir(),
			},
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
		pipelineMetadata: PipelineMetaData{
			GenesisHash: "",
			Network:     "",
			NextRound:   0,
		},
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
	var cbComplete conduit.Completed = &mProcessor

	ctx, cf := context.WithCancel(context.Background())
	pImpl := pipelineImpl{
		ctx:              ctx,
		cf:               cf,
		cfg:              &PipelineConfig{},
		logger:           log.New(),
		initProvider:     nil,
		importer:         &pImporter,
		processors:       []*processors.Processor{&pProcessor},
		exporter:         &pExporter,
		completeCallback: []conduit.OnCompleteFunc{cbComplete.OnComplete},
		pipelineMetadata: PipelineMetaData{},
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

func Test_pipelineImpl_registerLifecycleCallbacks(t *testing.T) {
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
		processors:   []*processors.Processor{&pProcessor, &pProcessor},
		exporter:     &pExporter,
	}

	// Each plugin implements the Completed interface, so there should be 4
	// plugins registered (one of them is registered twice)
	pImpl.registerLifecycleCallbacks()
	assert.Len(t, pImpl.completeCallback, 4)
}

// TestBlockMetaDataFile tests that metadata.json file is created as expected
func TestBlockMetaDataFile(t *testing.T) {

	var pImporter importers.Importer = &mockImporter{}
	var pProcessor processors.Processor = &mockProcessor{}
	var pExporter exporters.Exporter = &mockExporter{}

	datadir := t.TempDir()
	pImpl := pipelineImpl{
		cfg: &PipelineConfig{
			ConduitConfig: &Config{
				Flags:          nil,
				ConduitDataDir: datadir,
			},
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
		pipelineMetadata: PipelineMetaData{
			NextRound: 3,
		},
	}

	err := pImpl.Init()
	assert.NoError(t, err)

	// Test that file is created
	blockMetaDataFile := filepath.Join(datadir, "metadata.json")
	_, err = os.Stat(blockMetaDataFile)
	assert.NoError(t, err)

	// Test that file loads correctly
	metaData, err := pImpl.initializeOrLoadBlockMetadata()
	assert.NoError(t, err)
	assert.Equal(t, pImpl.pipelineMetadata.GenesisHash, metaData.GenesisHash)
	assert.Equal(t, pImpl.pipelineMetadata.NextRound, metaData.NextRound)
	assert.Equal(t, pImpl.pipelineMetadata.Network, metaData.Network)

	// Test that file encodes correctly
	pImpl.pipelineMetadata.GenesisHash = "HASH"
	pImpl.pipelineMetadata.NextRound = 7
	err = pImpl.encodeMetadataToFile()
	assert.NoError(t, err)
	metaData, err = pImpl.initializeOrLoadBlockMetadata()
	assert.NoError(t, err)
	assert.Equal(t, "HASH", metaData.GenesisHash)
	assert.Equal(t, uint64(7), metaData.NextRound)
	assert.Equal(t, pImpl.pipelineMetadata.Network, metaData.Network)

	// invalid file directory
	pImpl.cfg.ConduitConfig.ConduitDataDir = "datadir"
	metaData, err = pImpl.initializeOrLoadBlockMetadata()
	assert.Contains(t, err.Error(), "Init(): error creating file")
	err = pImpl.encodeMetadataToFile()
	assert.Contains(t, err.Error(), "encodeMetadataToFile(): failed to create temp metadata file")
}

func TestGenesisHash(t *testing.T) {
	var pImporter importers.Importer = &mockImporter{genesis: bookkeeping.Genesis{Network: "test"}}
	var pProcessor processors.Processor = &mockProcessor{}
	var pExporter exporters.Exporter = &mockExporter{}
	datadir := t.TempDir()
	pImpl := pipelineImpl{
		cfg: &PipelineConfig{
			ConduitConfig: &Config{
				Flags:          nil,
				ConduitDataDir: datadir,
			},
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
		pipelineMetadata: PipelineMetaData{
			GenesisHash: "",
			Network:     "",
			NextRound:   3,
		},
	}

	// write genesis hash to metadata.json
	err := pImpl.Init()
	assert.NoError(t, err)

	// read genesis hash from metadata.json
	blockmetaData, err := pImpl.initializeOrLoadBlockMetadata()
	assert.NoError(t, err)
	assert.Equal(t, blockmetaData.GenesisHash, crypto.HashObj(&bookkeeping.Genesis{Network: "test"}).String())
	assert.Equal(t, blockmetaData.Network, "test")

	// mock a different genesis hash
	pImporter = &mockImporter{genesis: bookkeeping.Genesis{Network: "dev"}}
	pImpl.importer = &pImporter
	err = pImpl.Init()
	assert.Contains(t, err.Error(), "genesis hash in metadata does not match")
}

func TestInitError(t *testing.T) {
	var pImporter importers.Importer = &mockImporter{genesis: bookkeeping.Genesis{Network: "test"}}
	var pProcessor processors.Processor = &mockProcessor{}
	var pExporter exporters.Exporter = &mockExporter{}
	datadir := "data"
	pImpl := pipelineImpl{
		cfg: &PipelineConfig{
			ConduitConfig: &Config{
				Flags:          nil,
				ConduitDataDir: datadir,
			},
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
				Name:   "unknown",
				Config: map[string]interface{}{},
			},
		},
		logger:       log.New(),
		initProvider: nil,
		importer:     &pImporter,
		processors:   []*processors.Processor{&pProcessor},
		exporter:     &pExporter,
		pipelineMetadata: PipelineMetaData{
			GenesisHash: "",
			Network:     "",
			NextRound:   3,
		},
	}

	// could not read metadata
	err := pImpl.Init()
	assert.Contains(t, err.Error(), "could not read metadata")
}

func TestPipelineMetricsConfigs(t *testing.T) {
	var pImporter importers.Importer = &mockImporter{}
	var pProcessor processors.Processor = &mockProcessor{}
	var pExporter exporters.Exporter = &mockExporter{}
	ctx, cf := context.WithCancel(context.Background())
	pImpl := pipelineImpl{
		cfg: &PipelineConfig{
			ConduitConfig: &Config{
				Flags:          nil,
				ConduitDataDir: t.TempDir(),
			},
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
			Metrics: Metrics{},
		},
		logger:       log.New(),
		initProvider: nil,
		importer:     &pImporter,
		processors:   []*processors.Processor{&pProcessor},
		exporter:     &pExporter,
		cf:           cf,
		ctx:          ctx,
		pipelineMetadata: PipelineMetaData{
			GenesisHash: "",
			Network:     "",
			NextRound:   0,
		},
	}
	defer pImpl.cf()

	getMetrics := func() (*http.Response, error) {
		resp0, err0 := http.Get(fmt.Sprintf("http://localhost%s/metrics", pImpl.cfg.Metrics.Addr))
		return resp0, err0
	}
	// metrics should be OFF by default
	err := pImpl.Init()
	assert.NoError(t, err)
	time.Sleep(1 * time.Second)
	_, err = getMetrics()
	assert.Error(t, err)

	// metrics mode OFF
	pImpl.cfg.Metrics = Metrics{
		Mode: "OFF",
		Addr: ":8081",
	}
	pImpl.Init()
	time.Sleep(1 * time.Second)
	_, err = getMetrics()
	assert.Error(t, err)

	// metrics mode ON
	pImpl.cfg.Metrics = Metrics{
		Mode: "ON",
		Addr: ":8081",
	}
	pImpl.Init()
	time.Sleep(1 * time.Second)
	resp, err := getMetrics()
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// TestPipelineLogFile tests that log file is created when specified
func TestPipelineLogFile(t *testing.T) {

	var mockedImpNew mockedImporterNew
	var mockedExpNew mockedExporterNew
	importers.RegisterImporter("mockedImporter", mockedImpNew)
	exporters.RegisterExporter("mockedExporter", mockedExpNew)

	logfilePath := path.Join(t.TempDir(), "conduit.log")
	configs := &PipelineConfig{
		ConduitConfig: &Config{
			Flags:          nil,
			ConduitDataDir: t.TempDir(),
		},
		Importer: NameConfigPair{
			Name:   "mockedImporter",
			Config: map[string]interface{}{"key": "value"},
		},
		Exporter: NameConfigPair{
			Name:   "mockedExporter",
			Config: map[string]interface{}{"key": "value"},
		},
		PipelineLogLevel: "INFO",
	}

	_, err := MakePipeline(context.Background(), configs, log.New())
	require.NoError(t, err)

	// Test that file is not created
	_, err = os.Stat(logfilePath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	// Test that it is created
	configs.LogFile = logfilePath
	_, err = MakePipeline(context.Background(), configs, log.New())
	require.NoError(t, err)

	_, err = os.Stat(logfilePath)
	assert.Nil(t, err)
}
