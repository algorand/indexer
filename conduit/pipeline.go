package conduit

import (
	"context"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"path/filepath"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
)

func init() {
	viper.SetConfigType("yaml")
}

const autoLoadParameterConfigName = "conduit.yml"

type nameConfigPair struct {
	Name   string                 `yaml:"Name"`
	Config map[string]interface{} `yaml:"Config"`
}

// PipelineConfig stores configuration specific to the conduit pipeline
type PipelineConfig struct {
	ConduitConfig *Config

	pipelineLogLevel string `yaml:"LogLevel"`
	PipelineLogLevel log.Level
	// Store a local copy to access parent variables
	Importer   nameConfigPair   `yaml:"Importer"`
	Processors []nameConfigPair `yaml:"Processors"`
	Exporter   nameConfigPair   `yaml:"Exporter"`
}

// Valid validates pipeline config
func (cfg *PipelineConfig) Valid() error {
	if cfg.ConduitConfig == nil {
		return fmt.Errorf("PipelineConfig.Valid(): conduit configuration was nil")
	}

	if _, err := log.ParseLevel(cfg.pipelineLogLevel); err != nil {
		return fmt.Errorf("PipelineConfig.Valid(): pipeline log level (%s) was invalid: %w", cfg.pipelineLogLevel, err)
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

	pCfg := PipelineConfig{pipelineLogLevel: logger.Level.String(), ConduitConfig: cfg}

	// Search for pipeline configuration in data directory
	autoloadParamConfigPath := filepath.Join(cfg.ConduitDataDir, autoLoadParameterConfigName)

	_, err := os.Stat(autoloadParamConfigPath)
	paramConfigFound := err == nil

	if !paramConfigFound {
		return nil, fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", autoLoadParameterConfigName, cfg.ConduitDataDir)
	}

	logger.Infof("Auto-loading Conduit Configuration: %s", autoloadParamConfigPath)

	file, err := os.Open(autoloadParamConfigPath)

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

	pCfg.PipelineLogLevel, err = log.ParseLevel(pCfg.pipelineLogLevel)
	if err != nil {
		// Belt and suspenders.  Valid() should have caught this
		return nil, fmt.Errorf("MakePipelineConfig(): config file (%s) had mal-formed log level: %w", autoloadParamConfigPath, err)
	}

	return &pCfg, nil

}

// Pipeline is a struct that orchestrates the entire
// sequence of events, taking in importers, processors and
// exporters and generating the result
type Pipeline interface {
	Start() error
	Stop() error
}

type pipelineImpl struct {
	ctx    context.Context
	cfg    *PipelineConfig
	logger *log.Logger

	initProvider *data.InitProvider

	importer   *importers.Importer
	processors []*processors.Processor
	exporter   *exporters.Exporter
	round      basics.Round
}

func (p *pipelineImpl) Start() error {
	p.logger.Infof("Starting Pipeline Initialization")

	// TODO Need to change interfaces to accept config of map[string]interface{}

	exporterLogger := log.New()
	exporterLogger.SetFormatter(
		PluginLogFormatter{
			Formatter: &log.JSONFormatter{
				DisableHTMLEscape: true,
			},
			Type: "Exporter",
			Name: (*p.exporter).Metadata().Name(),
		},
	)

	jsonEncode := string(json.Encode(p.cfg.Exporter.Config))
	err := (*p.exporter).Init(plugins.PluginConfig(jsonEncode), exporterLogger)
	exporterName := (*p.exporter).Metadata().Name()
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not initialize Exporter (%s): %w", exporterName, err)
	}
	p.logger.Infof("Initialized Exporter: %s", exporterName)

	importerLogger := log.New()
	importerLogger.SetFormatter(
		PluginLogFormatter{
			Formatter: &log.JSONFormatter{
				DisableHTMLEscape: true,
			},
			Type: "Importer",
			Name: (*p.importer).Metadata().Name(),
		},
	)

	// TODO modify/fix ?
	jsonEncode = string(json.Encode(p.cfg.Importer.Config))
	genesis, err := (*p.importer).Init(p.ctx, plugins.PluginConfig(jsonEncode), importerLogger)

	importerName := (*p.importer).Metadata().Name()
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not initialize importer (%s): %w", importerName, err)
	}
	p.round = basics.Round((*p.exporter).Round())
	err = (*p.exporter).HandleGenesis(*genesis)
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): exporter could not handle genesis (%s): %w", exporterName, err)
	}
	p.logger.Infof("Initialized Importer: %s", importerName)

	var initProvider data.InitProvider = &PipelineInitProvider{
		currentRound: &p.round,
		genesis:      genesis,
	}

	p.initProvider = &initProvider

	for idx, processor := range p.processors {

		processorLogger := log.New()
		processorLogger.SetFormatter(
			PluginLogFormatter{
				Formatter: &log.JSONFormatter{
					DisableHTMLEscape: true,
				},
				Type: "Processor",
				Name: (*processor).Metadata().Name(),
			},
		)
		jsonEncode = string(json.Encode(p.cfg.Processors[idx]))
		err := (*processor).Init(p.ctx, *p.initProvider, plugins.PluginConfig(jsonEncode), processorLogger)
		processorName := (*processor).Metadata().Name()
		if err != nil {
			return fmt.Errorf("Pipeline.Start(): could not initialize processor (%s): %w", processorName, err)
		}
		p.logger.Infof("Initialized Processor: %s", processorName)

	}

	return p.RunPipeline()
}

func (p *pipelineImpl) Stop() error {
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

	return nil
}

// RunPipeline pushes block data through the pipeline
func (p *pipelineImpl) RunPipeline() error {
	for {
		// TODO Retries?
		p.logger.Infof("Pipeline round: %v", p.round)
		// fetch block
		blkData, err := (*p.importer).GetBlock(uint64(p.round))
		if err != nil {
			p.logger.Errorf("%v\n", err)
			return err
		}
		// run through processors
		for _, proc := range p.processors {
			blkData, err = (*proc).Process(blkData)
			if err != nil {
				p.logger.Errorf("%v\n", err)
				return err
			}
		}
		// run through exporter
		err = (*p.exporter).Receive(blkData)
		if err != nil {
			p.logger.Errorf("%v\n", err)
			return err
		}
		// Callback Processors
		for _, proc := range p.processors {
			err = (*proc).OnComplete(blkData)
			if err != nil {
				p.logger.Errorf("%v\n", err)
				return err
			}
		}
		// Increment Round
		p.round++
	}
}

// MakePipeline creates a Pipeline
func MakePipeline(cfg *PipelineConfig, logger *log.Logger) (Pipeline, error) {

	if cfg == nil {
		return nil, fmt.Errorf("MakePipeline(): pipeline config was empty")
	}

	if err := cfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipeline(): %w", err)
	}

	if logger == nil {
		return nil, fmt.Errorf("MakePipeline(): logger was empty")
	}

	pipeline := &pipelineImpl{
		ctx:          context.Background(),
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
