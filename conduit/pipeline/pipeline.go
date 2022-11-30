package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/conduit/plugins/importers"
	"github.com/algorand/indexer/conduit/plugins/processors"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/util"
	"github.com/algorand/indexer/util/metrics"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
)

func init() {
	viper.SetConfigType("yaml")
}

// NameConfigPair is a generic structure used across plugin configuration ser/de
type NameConfigPair struct {
	Name   string                 `yaml:"name"`
	Config map[string]interface{} `yaml:"config"`
}

// Metrics configs for turning on Prometheus endpoint /metrics
type Metrics struct {
	Mode string `yaml:"mode"`
	Addr string `yaml:"addr"`
}

// Config stores configuration specific to the conduit pipeline
type Config struct {
	ConduitConfig *conduit.Config

	CPUProfile  string `yaml:"cpu-profile"`
	PIDFilePath string `yaml:"pid-filepath"`

	LogFile          string `yaml:"log-file"`
	PipelineLogLevel string `yaml:"log-level"`
	// Store a local copy to access parent variables
	Importer   NameConfigPair   `yaml:"importer"`
	Processors []NameConfigPair `yaml:"processors"`
	Exporter   NameConfigPair   `yaml:"exporter"`
	Metrics    Metrics          `yaml:"metrics"`
}

// Valid validates pipeline config
func (cfg *Config) Valid() error {
	if cfg.ConduitConfig == nil {
		return fmt.Errorf("Config.Valid(): conduit configuration was nil")
	}

	if _, err := log.ParseLevel(cfg.PipelineLogLevel); err != nil {
		return fmt.Errorf("Config.Valid(): pipeline log level (%s) was invalid: %w", cfg.PipelineLogLevel, err)
	}

	if len(cfg.Importer.Config) == 0 {
		return fmt.Errorf("Config.Valid(): importer configuration was empty")
	}

	if len(cfg.Exporter.Config) == 0 {
		return fmt.Errorf("Config.Valid(): exporter configuration was empty")
	}

	return nil
}

// MakePipelineConfig creates a pipeline configuration
func MakePipelineConfig(logger *log.Logger, cfg *conduit.Config) (*Config, error) {
	if cfg == nil {
		return nil, fmt.Errorf("MakePipelineConfig(): empty conduit config")
	}

	// double check that it is valid
	if err := cfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): %w", err)
	}
	pCfg := Config{PipelineLogLevel: logger.Level.String(), ConduitConfig: cfg}

	// Search for pipeline configuration in data directory
	autoloadParamConfigPath := filepath.Join(cfg.ConduitDataDir, conduit.DefaultConfigName)

	_, err := os.Stat(autoloadParamConfigPath)
	paramConfigFound := err == nil

	if !paramConfigFound {
		return nil, fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", conduit.DefaultConfigName, cfg.ConduitDataDir)
	}

	logger.Infof("Auto-loading Conduit Configuration: %s", autoloadParamConfigPath)

	file, err := os.ReadFile(autoloadParamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): reading config error: %w", err)
	}
	err = yaml.Unmarshal(file, &pCfg)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): config file (%s) was mal-formed yaml: %w", autoloadParamConfigPath, err)
	}

	if err := pCfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): config file (%s) had mal-formed schema: %w", autoloadParamConfigPath, err)
	}

	return &pCfg, nil

}

// Pipeline is a struct that orchestrates the entire
// sequence of events, taking in importers, processors and
// exporters and generating the result
type Pipeline interface {
	Init() error
	Start()
	Stop()
	Error() error
	Wait()
}

type pipelineImpl struct {
	ctx      context.Context
	cf       context.CancelFunc
	wg       sync.WaitGroup
	cfg      *Config
	logger   *log.Logger
	profFile *os.File
	err      error
	mu       sync.RWMutex

	initProvider *data.InitProvider

	importer         *importers.Importer
	processors       []*processors.Processor
	exporter         *exporters.Exporter
	completeCallback []conduit.OnCompleteFunc

	pipelineMetadata state

	metricsCallback []conduit.ProvideMetricsFunc
}

// state contains the pipeline state.
type state struct {
	GenesisHash string `json:"genesis-hash"`
	Network     string `json:"network"`
	NextRound   uint64 `json:"next-round"`
}

func (p *pipelineImpl) Error() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.err
}

func (p *pipelineImpl) setError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

func (p *pipelineImpl) registerLifecycleCallbacks() {
	if v, ok := (*p.importer).(conduit.Completed); ok {
		p.completeCallback = append(p.completeCallback, v.OnComplete)
	}
	for _, processor := range p.processors {
		if v, ok := (*processor).(conduit.Completed); ok {
			p.completeCallback = append(p.completeCallback, v.OnComplete)
		}
	}
	if v, ok := (*p.exporter).(conduit.Completed); ok {
		p.completeCallback = append(p.completeCallback, v.OnComplete)
	}
}

func (p *pipelineImpl) registerPluginMetricsCallbacks() {
	if v, ok := (*p.importer).(conduit.PluginMetrics); ok {
		p.metricsCallback = append(p.metricsCallback, v.ProvideMetrics)
	}
	for _, processor := range p.processors {
		if v, ok := (*processor).(conduit.PluginMetrics); ok {
			p.metricsCallback = append(p.metricsCallback, v.ProvideMetrics)
		}
	}
	if v, ok := (*p.exporter).(conduit.PluginMetrics); ok {
		p.metricsCallback = append(p.metricsCallback, v.ProvideMetrics)
	}
}

// Init prepares the pipeline for processing block data
func (p *pipelineImpl) Init() error {
	p.logger.Infof("Starting Pipeline Initialization")

	if p.cfg.CPUProfile != "" {
		p.logger.Infof("Creating CPU Profile file at %s", p.cfg.CPUProfile)
		var err error
		profFile, err := os.Create(p.cfg.CPUProfile)
		if err != nil {
			p.logger.WithError(err).Errorf("%s: create, %v", p.cfg.CPUProfile, err)
			return err
		}
		p.profFile = profFile
		err = pprof.StartCPUProfile(profFile)
		if err != nil {
			p.logger.WithError(err).Errorf("%s: start pprof, %v", p.cfg.CPUProfile, err)
			return err
		}
	}

	if p.cfg.PIDFilePath != "" {
		err := util.CreateIndexerPidFile(p.logger, p.cfg.PIDFilePath)
		if err != nil {
			return err
		}
	}

	// TODO Need to change interfaces to accept config of map[string]interface{}

	// Initialize Importer
	importerLogger := log.New()
	// Make sure we are thread-safe
	importerLogger.SetOutput(p.logger.Out)
	importerName := (*p.importer).Metadata().Name
	importerLogger.SetFormatter(makePluginLogFormatter(plugins.Importer, importerName))

	configs, err := yaml.Marshal(p.cfg.Importer.Config)
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not serialize Importer.Config: %w", err)
	}
	genesis, err := (*p.importer).Init(p.ctx, plugins.PluginConfig(configs), importerLogger)
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not initialize importer (%s): %w", importerName, err)
	}

	// initialize or load pipeline metadata
	gh := crypto.HashObj(genesis).String()
	p.pipelineMetadata.GenesisHash = gh
	p.pipelineMetadata.Network = string(genesis.Network)
	p.pipelineMetadata, err = p.initializeOrLoadBlockMetadata()
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not read metadata: %w", err)
	}
	if p.pipelineMetadata.GenesisHash != gh {
		return fmt.Errorf("Pipeline.Start(): genesis hash in metadata does not match expected value: actual %s, expected %s", gh, p.pipelineMetadata.GenesisHash)
	}
	// overriding NextRound if NextRoundOverride is set
	if p.cfg.ConduitConfig.NextRoundOverride > 0 {
		p.logger.Infof("Overriding default next round from %d to %d.", p.pipelineMetadata.NextRound, p.cfg.ConduitConfig.NextRoundOverride)
		p.pipelineMetadata.NextRound = p.cfg.ConduitConfig.NextRoundOverride
	}

	p.logger.Infof("Initialized Importer: %s", importerName)

	// InitProvider
	round := basics.Round(p.pipelineMetadata.NextRound)
	var initProvider data.InitProvider = conduit.MakePipelineInitProvider(&round, genesis)
	p.initProvider = &initProvider

	// Initialize Processors
	for idx, processor := range p.processors {
		processorLogger := log.New()
		// Make sure we are thread-safe
		processorLogger.SetOutput(p.logger.Out)
		processorLogger.SetFormatter(makePluginLogFormatter(plugins.Processor, (*processor).Metadata().Name))
		configs, err = yaml.Marshal(p.cfg.Processors[idx].Config)
		if err != nil {
			return fmt.Errorf("Pipeline.Start(): could not serialize Processors[%d].Config : %w", idx, err)
		}
		err := (*processor).Init(p.ctx, *p.initProvider, plugins.PluginConfig(configs), processorLogger)
		processorName := (*processor).Metadata().Name
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not initialize processor (%s): %w", processorName, err)
		}
		p.logger.Infof("Initialized Processor: %s", processorName)
	}

	// Initialize Exporter
	exporterLogger := log.New()
	// Make sure we are thread-safe
	exporterLogger.SetOutput(p.logger.Out)
	exporterLogger.SetFormatter(makePluginLogFormatter(plugins.Exporter, (*p.exporter).Metadata().Name))

	configs, err = yaml.Marshal(p.cfg.Exporter.Config)
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not serialize Exporter.Config : %w", err)
	}
	err = (*p.exporter).Init(p.ctx, *p.initProvider, plugins.PluginConfig(configs), exporterLogger)
	exporterName := (*p.exporter).Metadata().Name
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not initialize Exporter (%s): %w", exporterName, err)
	}
	p.logger.Infof("Initialized Exporter: %s", exporterName)

	// Register callbacks.
	p.registerLifecycleCallbacks()

	// start metrics server
	if p.cfg.Metrics.Mode == "ON" {
		p.registerPluginMetricsCallbacks()
		for _, cb := range p.metricsCallback {
			collectors := cb()
			for _, c := range collectors {
				_ = prometheus.Register(c)
			}
		}
		go p.startMetricsServer()
	}

	return err
}

func (p *pipelineImpl) Stop() {
	p.cf()
	p.wg.Wait()

	if p.profFile != nil {
		if err := p.profFile.Close(); err != nil {
			p.logger.WithError(err).Errorf("%s: could not close CPUProf file", p.profFile.Name())
		}
		pprof.StopCPUProfile()
	}

	if p.cfg.PIDFilePath != "" {
		if err := os.Remove(p.cfg.PIDFilePath); err != nil {
			p.logger.WithError(err).Errorf("%s: could not remove pid file", p.cfg.PIDFilePath)
		}
	}

	if err := (*p.importer).Close(); err != nil {
		// Log and continue on closing the rest of the pipeline
		p.logger.Errorf("Pipeline.Stop(): Importer (%s) error on close: %v", (*p.importer).Metadata().Name, err)
	}

	for _, processor := range p.processors {
		if err := (*processor).Close(); err != nil {
			// Log and continue on closing the rest of the pipeline
			p.logger.Errorf("Pipeline.Stop(): Processor (%s) error on close: %v", (*processor).Metadata().Name, err)
		}
	}

	if err := (*p.exporter).Close(); err != nil {
		p.logger.Errorf("Pipeline.Stop(): Exporter (%s) error on close: %v", (*p.exporter).Metadata().Name, err)
	}
}

func (p *pipelineImpl) addMetrics(block data.BlockData, importTime time.Duration) {
	metrics.BlockImportTimeSeconds.Observe(importTime.Seconds())
	metrics.ImportedTxnsPerBlock.Observe(float64(len(block.Payset)))
	metrics.ImportedRoundGauge.Set(float64(block.Round()))
	txnCountByType := make(map[string]int)
	for _, txn := range block.Payset {
		txnCountByType[string(txn.Txn.Type)]++
	}
	for k, v := range txnCountByType {
		metrics.ImportedTxns.WithLabelValues(k).Set(float64(v))
	}
}

// Start pushes block data through the pipeline
func (p *pipelineImpl) Start() {
	p.wg.Add(1)
	retry := 0
	go func() {
		defer p.wg.Done()
		// We need to add a separate recover function here since it launches its own go-routine
		defer HandlePanic(p.logger)
		for {
		pipelineRun:
			metrics.PipelineRetryCount.Observe(float64(retry))
			select {
			case <-p.ctx.Done():
				return
			default:
				{
					p.logger.Infof("Pipeline round: %v", p.pipelineMetadata.NextRound)
					// fetch block
					importStart := time.Now()
					blkData, err := (*p.importer).GetBlock(p.pipelineMetadata.NextRound)
					if err != nil {
						p.logger.Errorf("%v", err)
						p.setError(err)
						retry++
						goto pipelineRun
					}
					metrics.ImporterTimeSeconds.Observe(time.Since(importStart).Seconds())
					// Start time currently measures operations after block fetching is complete.
					// This is for backwards compatibility w/ Indexer's metrics
					// run through processors
					start := time.Now()
					for _, proc := range p.processors {
						processorStart := time.Now()
						blkData, err = (*proc).Process(blkData)
						if err != nil {
							p.logger.Errorf("%v", err)
							p.setError(err)
							retry++
							goto pipelineRun
						}
						metrics.ProcessorTimeSeconds.WithLabelValues((*proc).Metadata().Name).Observe(time.Since(processorStart).Seconds())
					}
					// run through exporter
					exporterStart := time.Now()
					err = (*p.exporter).Receive(blkData)
					if err != nil {
						p.logger.Errorf("%v", err)
						p.setError(err)
						retry++
						goto pipelineRun
					}

					// Increment Round, update metadata
					p.pipelineMetadata.NextRound++
					_ = p.encodeMetadataToFile()

					// Callback Processors
					for _, cb := range p.completeCallback {
						err = cb(blkData)
						if err != nil {
							p.logger.Errorf("%v", err)
							p.setError(err)
							retry++
							goto pipelineRun
						}
					}
					metrics.ExporterTimeSeconds.Observe(time.Since(exporterStart).Seconds())
					// Ignore round 0 (which is empty).
					if p.pipelineMetadata.NextRound > 1 {
						p.addMetrics(blkData, time.Since(start))
					}
					p.setError(nil)
					retry = 0
				}
			}

		}
	}()
}

func (p *pipelineImpl) Wait() {
	p.wg.Wait()
}

func metadataPath(dataDir string) string {
	return path.Join(dataDir, "metadata.json")
}

func (p *pipelineImpl) encodeMetadataToFile() error {
	pipelineMetadataFilePath := metadataPath(p.cfg.ConduitConfig.ConduitDataDir)
	tempFilename := fmt.Sprintf("%s.temp", pipelineMetadataFilePath)
	file, err := os.Create(tempFilename)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to create temp metadata file: %w", err)
	}
	defer file.Close()
	err = json.NewEncoder(file).Encode(p.pipelineMetadata)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to write temp metadata: %w", err)
	}

	err = os.Rename(tempFilename, pipelineMetadataFilePath)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to replace metadata file: %w", err)
	}
	return nil
}

func (p *pipelineImpl) initializeOrLoadBlockMetadata() (state, error) {
	pipelineMetadataFilePath := metadataPath(p.cfg.ConduitConfig.ConduitDataDir)
	if stat, err := os.Stat(pipelineMetadataFilePath); errors.Is(err, os.ErrNotExist) || (stat != nil && stat.Size() == 0) {
		fmt.Println(err)
		fmt.Println(stat)
		if stat != nil && stat.Size() == 0 {
			err = os.Remove(pipelineMetadataFilePath)
			if err != nil {
				return p.pipelineMetadata, fmt.Errorf("Init(): error creating file: %w", err)
			}
		}
		err = p.encodeMetadataToFile()
		if err != nil {
			return p.pipelineMetadata, fmt.Errorf("Init(): error creating file: %w", err)
		}
	} else {
		if err != nil {
			return p.pipelineMetadata, fmt.Errorf("error opening file: %w", err)
		}
		var data []byte
		data, err = os.ReadFile(pipelineMetadataFilePath)
		if err != nil {
			return p.pipelineMetadata, fmt.Errorf("error reading metadata: %w", err)
		}
		err = json.Unmarshal(data, &p.pipelineMetadata)
		if err != nil {
			return p.pipelineMetadata, fmt.Errorf("error reading metadata: %w", err)
		}
	}
	return p.pipelineMetadata, nil
}

// start a http server serving /metrics
func (p *pipelineImpl) startMetricsServer() {
	http.Handle("/metrics", promhttp.Handler())
	_ = http.ListenAndServe(p.cfg.Metrics.Addr, nil)
	p.logger.Infof("conduit metrics serving on %s", p.cfg.Metrics.Addr)
}

// MakePipeline creates a Pipeline
func MakePipeline(ctx context.Context, cfg *Config, logger *log.Logger) (Pipeline, error) {

	if cfg == nil {
		return nil, fmt.Errorf("MakePipeline(): pipeline config was empty")
	}

	if err := cfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipeline(): %w", err)
	}

	if logger == nil {
		return nil, fmt.Errorf("MakePipeline(): logger was empty")
	}

	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return nil, fmt.Errorf("MakePipeline(): %w", err)
		}
		logger.SetOutput(f)
	}

	logLevel, err := log.ParseLevel(cfg.PipelineLogLevel)
	if err != nil {
		// Belt and suspenders.  Valid() should have caught this
		return nil, fmt.Errorf("MakePipeline(): config had mal-formed log level: %w", err)
	}
	logger.SetLevel(logLevel)

	cancelContext, cancelFunc := context.WithCancel(ctx)

	pipeline := &pipelineImpl{
		ctx:          cancelContext,
		cf:           cancelFunc,
		cfg:          cfg,
		logger:       logger,
		initProvider: nil,
		importer:     nil,
		processors:   []*processors.Processor{},
		exporter:     nil,
	}

	importerName := cfg.Importer.Name

	importerBuilder, err := importers.ImporterBuilderByName(importerName)
	if err != nil {
		return nil, fmt.Errorf("MakePipeline(): could not find importer builder with name: %s", importerName)
	}

	importer := importerBuilder.New()
	pipeline.importer = &importer
	logger.Infof("Found Importer: %s", importerName)

	// ---

	for _, processorConfig := range cfg.Processors {
		processorName := processorConfig.Name

		processorBuilder, err := processors.ProcessorBuilderByName(processorName)
		if err != nil {
			return nil, fmt.Errorf("MakePipeline(): could not find processor builder with name: %s", processorName)
		}

		processor := processorBuilder.New()
		pipeline.processors = append(pipeline.processors, &processor)
		logger.Infof("Found Processor: %s", processorName)
	}

	// ---

	exporterName := cfg.Exporter.Name

	exporterBuilder, err := exporters.ExporterBuilderByName(exporterName)
	if err != nil {
		return nil, fmt.Errorf("MakePipeline(): could not find exporter builder with name: %s", exporterName)
	}

	exporter := exporterBuilder.New()
	pipeline.exporter = &exporter
	logger.Infof("Found Exporter: %s", exporterName)

	return pipeline, nil
}
