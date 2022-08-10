package conduit

import (
	"context"
	"fmt"
	"github.com/algorand/go-algorand/data/basics"
	"os"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"path/filepath"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
)

const autoLoadParameterConfigName = "conduit.yml"

type nameConfigPair struct {
	Name   string                 `yaml:"Name"`
	Config map[string]interface{} `yaml:"Config"`
}

// PipelineConfig stores configuration specific to the conduit pipeline
type PipelineConfig struct {
	ConduitConfig *Config

	PipelineLogLevel string `yaml:"LogLevel"`
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
	autoloadParamConfigPath := filepath.Join(cfg.ConduitDataDir, autoLoadParameterConfigName)

	_, err := os.Stat(autoloadParamConfigPath)
	paramConfigFound := err == nil

	if !paramConfigFound {
		return nil, fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", autoLoadParameterConfigName, cfg.ConduitDataDir)
	}

	logger.Infof("Auto-loading Conduit Configuration: %s", autoloadParamConfigPath)

	configStr, err := ioutil.ReadFile(autoloadParamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): %w", err)
	}

	err = yaml.Unmarshal(configStr, &pCfg)
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
}

func (p *pipelineImpl) Start() error {
	p.logger.Infof("Starting Pipeline Initialization")

	// TODO Need to change interfaces to accept config of map[string]interface{}

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
	jsonEncode := string(json.Encode(p.cfg.Importer.Config))
	genesis, err := (*p.importer).Init(p.ctx, plugins.PluginConfig(jsonEncode), importerLogger)

	currentRound := basics.Round(0)

	var initProvider data.InitProvider = &PipelineInitProvider{
		currentRound: &currentRound,
		genesis:      genesis,
	}

	p.initProvider = &initProvider

	importerName := (*p.importer).Metadata().Name()
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not initialize importer (%s): %w", importerName, err)
	}
	p.logger.Infof("Initialized Importer: %s", importerName)

	for _, processor := range p.processors {

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

		err := (*processor).Init(p.ctx, *p.initProvider, "")
		processorName := (*processor).Metadata().Name()
		if err != nil {
			return fmt.Errorf("Pipeline.Start(): could not initialize processor (%s): %w", processorName, err)
		}
		p.logger.Infof("Initialized Processor: %s", processorName)

	}

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

	err = (*p.exporter).Init("", p.logger)
	ExporterName := (*p.exporter).Metadata().Name()
	if err != nil {
		return fmt.Errorf("Pipeline.Start(): could not initialize Exporter (%s): %w", ExporterName, err)
	}
	p.logger.Infof("Initialized Exporter: %s", ExporterName)

	return nil
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
