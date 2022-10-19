package conduit

import (
	"context"
	"fmt"
	"github.com/algorand/indexer/util/metrics"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand/data/basics"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
	"github.com/algorand/indexer/util"
)

func init() {
	viper.SetConfigType("yaml")
}

// NameConfigPair is a generic structure used across plugin configuration ser/de
type NameConfigPair struct {
	Name   string                 `yaml:"name"`
	Config map[string]interface{} `yaml:"config"`
}

// PipelineConfig stores configuration specific to the conduit pipeline
type PipelineConfig struct {
	ConduitConfig *Config

	CPUProfile  string `yaml:"cpu-profile"`
	PIDFilePath string `yaml:"pid-filepath"`

	PipelineLogLevel string `yaml:"log-level"`
	// Store a local copy to access parent variables
	Importer   NameConfigPair   `yaml:"importer"`
	Processors []NameConfigPair `yaml:"processors"`
	Exporter   NameConfigPair   `yaml:"exporter"`
}

// Valid validates pipeline config
func (cfg *PipelineConfig) Valid() error {
	if cfg.ConduitConfig == nil {
		return fmt.Errorf("PipelineConfig.Valid(): conduit configuration was nil")
	}

	if _, err := log.ParseLevel(cfg.PipelineLogLevel); err != nil {
		return fmt.Errorf("PipelineConfig.Valid(): pipeline log level (%s) was invalid: %w", cfg.PipelineLogLevel, err)
	}

	if len(cfg.Importer.Config) == 0 {
		return fmt.Errorf("PipelineConfig.Valid(): importer configuration was empty")
	}

	if len(cfg.Exporter.Config) == 0 {
		return fmt.Errorf("PipelineConfig.Valid(): exporter configuration was empty")
	}

	return nil
}

// MakePipelineConfig creates a pipeline configuration
func MakePipelineConfig(logger *log.Logger, cfg *Config) (*PipelineConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("MakePipelineConfig(): empty conduit config")
	}

	// double check that it is valid
	if err := cfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): %w", err)
	}

	pCfg := PipelineConfig{PipelineLogLevel: logger.Level.String(), ConduitConfig: cfg}

	// Search for pipeline configuration in data directory
	autoloadParamConfigPath := filepath.Join(cfg.ConduitDataDir, DefaultConfigName)

	_, err := os.Stat(autoloadParamConfigPath)
	paramConfigFound := err == nil

	if !paramConfigFound {
		return nil, fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", DefaultConfigName, cfg.ConduitDataDir)
	}

	logger.Infof("Auto-loading Conduit Configuration: %s", autoloadParamConfigPath)

	file, err := os.Open(autoloadParamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): error opening file: %w", err)
	}
	defer file.Close()

	err = viper.ReadConfig(file)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): reading config error: %w", err)
	}

	err = viper.Unmarshal(&pCfg)
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
	cfg      *PipelineConfig
	logger   *log.Logger
	profFile *os.File
	err      error
	mu       sync.RWMutex

	initProvider *data.InitProvider

	importer   *importers.Importer
	processors []*processors.Processor
	exporter   *exporters.Exporter
	round      basics.Round
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

	// Initialize Exporter first since the pipeline derives its round from the Exporter
	exporterLogger := log.New()
	// Make sure we are thread-safe
	exporterLogger.SetOutput(p.logger.Out)
	exporterLogger.SetFormatter(makePluginLogFormatter(plugins.Exporter, (*p.exporter).Metadata().Name()))

	jsonEncode := string(json.Encode(p.cfg.Exporter.Config))
	err := (*p.exporter).Init(p.ctx, plugins.PluginConfig(jsonEncode), exporterLogger)
	exporterName := (*p.exporter).Metadata().Name()
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not initialize Exporter (%s): %w", exporterName, err)
	}
	p.logger.Infof("Initialized Exporter: %s", exporterName)

	// Initialize Importer
	importerLogger := log.New()
	// Make sure we are thread-safe
	importerLogger.SetOutput(p.logger.Out)
	importerName := (*p.importer).Metadata().Name()
	importerLogger.SetFormatter(makePluginLogFormatter(plugins.Importer, importerName))

	// TODO modify/fix ?
	jsonEncode = string(json.Encode(p.cfg.Importer.Config))
	genesis, err := (*p.importer).Init(p.ctx, plugins.PluginConfig(jsonEncode), importerLogger)

	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not initialize importer (%s): %w", importerName, err)
	}
	p.round = basics.Round((*p.exporter).Round())
	err = (*p.exporter).HandleGenesis(*genesis)
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): exporter could not handle genesis (%s): %w", exporterName, err)
	}
	p.logger.Infof("Initialized Importer: %s", importerName)

	// Initialize Processors
	var initProvider data.InitProvider = &PipelineInitProvider{
		currentRound: &p.round,
		genesis:      genesis,
	}
	p.initProvider = &initProvider

	for idx, processor := range p.processors {
		processorLogger := log.New()
		// Make sure we are thread-safe
		processorLogger.SetOutput(p.logger.Out)
		processorLogger.SetFormatter(makePluginLogFormatter(plugins.Processor, (*processor).Metadata().Name()))
		jsonEncode = string(json.Encode(p.cfg.Processors[idx].Config))
		err := (*processor).Init(p.ctx, *p.initProvider, plugins.PluginConfig(jsonEncode), processorLogger)
		processorName := (*processor).Metadata().Name()
		if err != nil {
			return fmt.Errorf("Pipeline.Start(): could not initialize processor (%s): %w", processorName, err)
		}
		p.logger.Infof("Initialized Processor: %s", processorName)
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
		p.logger.Errorf("Pipeline.Stop(): Importer (%s) error on close: %v", (*p.importer).Metadata().Name(), err)
	}

	for _, processor := range p.processors {
		if err := (*processor).Close(); err != nil {
			// Log and continue on closing the rest of the pipeline
			p.logger.Errorf("Pipeline.Stop(): Processor (%s) error on close: %v", (*processor).Metadata().Name(), err)
		}
	}

	if err := (*p.exporter).Close(); err != nil {
		p.logger.Errorf("Pipeline.Stop(): Exporter (%s) error on close: %v", (*p.exporter).Metadata().Name(), err)
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
	go func() {
		defer p.wg.Done()
		// We need to add a separate recover function here since it launches its own go-routine
		defer HandlePanic(p.logger)
		for {
		pipelineRun:
			select {
			case <-p.ctx.Done():
				return
			default:
				{
					p.logger.Infof("Pipeline round: %v", p.round)
					// fetch block
					blkData, err := (*p.importer).GetBlock(uint64(p.round))
					if err != nil {
						p.logger.Errorf("%v", err)
						p.setError(err)
						goto pipelineRun
					}
					// Start time currently measures operations after block fetching is complete.
					// This is for backwards compatibility w/ Indexer's metrics
					start := time.Now()
					// run through processors
					for _, proc := range p.processors {
						blkData, err = (*proc).Process(blkData)
						if err != nil {
							p.logger.Errorf("%v", err)
							p.setError(err)
							goto pipelineRun
						}
					}
					// run through exporter
					err = (*p.exporter).Receive(blkData)
					if err != nil {
						p.logger.Errorf("%v", err)
						p.setError(err)
						goto pipelineRun
					}
					// Callback Processors
					for _, proc := range p.processors {
						err = (*proc).OnComplete(blkData)
						if err != nil {
							p.logger.Errorf("%v", err)
							p.setError(err)
							goto pipelineRun
						}
					}
					importTime := time.Since(start)
					// Ignore round 0 (which is empty).
					if p.round > 0 {
						p.addMetrics(blkData, importTime)
					}
					// Increment Round
					p.setError(nil)
					p.round++
				}
			}

		}
	}()
}

func (p *pipelineImpl) Wait() {
	p.wg.Wait()
}

// MakePipeline creates a Pipeline
func MakePipeline(ctx context.Context, cfg *PipelineConfig, logger *log.Logger) (Pipeline, error) {

	if cfg == nil {
		return nil, fmt.Errorf("MakePipeline(): pipeline config was empty")
	}

	if err := cfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipeline(): %w", err)
	}

	if logger == nil {
		return nil, fmt.Errorf("MakePipeline(): logger was empty")
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
