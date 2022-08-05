package conduit

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"path/filepath"

	"github.com/algorand/go-algorand/util"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/processors"
)

const autoLoadParameterConfigName = "conduit.yml"

// PipelineConfig stores configuration specific to the conduit pipeline
type PipelineConfig struct {
	ConduitConfig *Config
	// Store a local copy to access parent variables
	Importer   map[string]interface{}   `yaml:"Importer"`
	Processors []map[string]interface{} `yaml:"Processors"`
	Exporter   map[string]interface{}   `yaml:"Exporter"`
}

// Valid validates pipeline config
func (cfg *PipelineConfig) Valid() error {
	if cfg.ConduitConfig == nil {
		return fmt.Errorf("conduit configuration was nil")
	}

	if len(cfg.Importer) == 0 {
		return fmt.Errorf("importer configuration was empty")
	}

	if len(cfg.Processors) == 0 {
		return fmt.Errorf("processor configuration was empty")
	}

	if len(cfg.Exporter) == 0 {
		return fmt.Errorf("exporter configuration was empty")
	}

	return nil
}

// MakePipelineConfig creates a pipeline configuration
func MakePipelineConfig(logger *log.Logger, cfg *Config) (*PipelineConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("empty conduit config")
	}

	// double check that it is valid
	if err := cfg.Valid(); err != nil {
		return nil, err
	}

	pCfg := PipelineConfig{ConduitConfig: cfg}

	// Search for pipeline configuration in data directory
	autoloadParamConfigPath := filepath.Join(cfg.ConduitDataDir, autoLoadParameterConfigName)
	paramConfigFound := util.FileExists(autoloadParamConfigPath)

	if !paramConfigFound {
		return nil, fmt.Errorf("could not find %s in data directory (%s)", autoLoadParameterConfigName, cfg.ConduitDataDir)
	}

	logger.Infof("Auto-loading Conduit Configuration: %s", autoloadParamConfigPath)

	configStr, err := ioutil.ReadFile(autoloadParamConfigPath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(configStr, &pCfg)
	if err != nil {
		return nil, fmt.Errorf("config file (%s) was mal-formed yaml: %w", autoloadParamConfigPath, err)
	}

	if err := pCfg.Valid(); err != nil {
		return nil, fmt.Errorf("config file (%s) had mal-formed schema: %w", autoloadParamConfigPath, err)
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

	// TODO need to change config to map[string]interface{}

	err := (*p.importer).Init(p.ctx, "", p.logger)
	importerName := (*p.importer).Metadata().Name()
	if err != nil {
		return fmt.Errorf("could not initialize importer (%s): %w", importerName, err)
	}
	p.logger.Infof("Initialized Importer: %s", importerName)

	for _, processor := range p.processors {
		err := (*processor).Init(p.ctx, *p.initProvider, "")
		processorName := (*processor).Metadata().Name()
		if err != nil {
			return fmt.Errorf("could not initialize processor (%s): %w", processorName, err)
		}
		p.logger.Infof("Initialized Processor: %s", processorName)

	}

	err = (*p.exporter).Init("", p.logger)
	ExporterName := (*p.exporter).Metadata().Name()
	if err != nil {
		return fmt.Errorf("could not initialize Exporter (%s): %w", ExporterName, err)
	}
	p.logger.Infof("Initialized Exporter: %s", ExporterName)

	return nil
}

func (p *pipelineImpl) Stop() error {
	if err := (*p.importer).Close(); err != nil {
		// Log and continue on closing the rest of the pipeline
		p.logger.Errorf("Importer (%s) error on close: %v", (*p.importer).Metadata().Name(), err)
	}

	for _, processor := range p.processors {
		if err := (*processor).Close(); err != nil {
			// Log and continue on closing the rest of the pipeline
			p.logger.Errorf("Processor (%s) error on close: %v", (*processor).Metadata().Name(), err)
		}
	}

	if err := (*p.exporter).Close(); err != nil {
		p.logger.Errorf("Exporter (%s) error on close: %v", (*p.exporter).Metadata().Name(), err)
	}

	return nil
}

// MakePipeline creates a Pipeline
func MakePipeline(cfg *PipelineConfig, logger *log.Logger, initProvider *data.InitProvider) (Pipeline, error) {

	if cfg == nil {
		return nil, fmt.Errorf("pipeline config was empty")
	}

	if err := cfg.Valid(); err != nil {
		return nil, err
	}

	if logger == nil {
		return nil, fmt.Errorf("logger was empty")
	}

	if initProvider == nil {
		return nil, fmt.Errorf("init provider was empty")
	}

	pipeline := &pipelineImpl{
		ctx:          context.Background(),
		cfg:          cfg,
		logger:       logger,
		initProvider: initProvider,
		importer:     nil,
		processors:   []*processors.Processor{},
		exporter:     nil,
	}

	importerName, ok := cfg.Importer["Name"]
	if !ok {
		return nil, fmt.Errorf("invalid schema, importer has no 'Name' attribute")
	}

	importerBuilder, err := importers.ImporterBuilderByName(importerName.(string))
	if err != nil {
		return nil, fmt.Errorf("could not find importer builder with name: %s", importerName.(string))
	}

	importer := importerBuilder.New()
	pipeline.importer = &importer
	logger.Infof("Found Importer: %s", importerName.(string))

	// ---

	for _, processorConfig := range cfg.Processors {
		processorName, ok := processorConfig["Name"]
		if !ok {
			return nil, fmt.Errorf("invalid schema, processor has no 'Name' attribute")
		}

		processorBuilder, err := processors.ProcessorBuilderByName(processorName.(string))
		if err != nil {
			return nil, fmt.Errorf("could not find processor builder with name: %s", processorName.(string))
		}

		processor := processorBuilder.New()
		pipeline.processors = append(pipeline.processors, &processor)
		logger.Infof("Found Processor: %s", processorName.(string))
	}

	// ---

	exporterName, ok := cfg.Exporter["Name"]
	if !ok {
		return nil, fmt.Errorf("invalid schema, exporter has no 'Name' attribute")
	}

	exporterBuilder, err := exporters.ExporterBuilderByName(exporterName.(string))
	if err != nil {
		return nil, fmt.Errorf("could not find exporter builder with name: %s", exporterName.(string))
	}

	exporter := exporterBuilder.New()
	pipeline.exporter = &exporter
	logger.Infof("Found Exporter: %s", exporterName.(string))

	return pipeline, nil
}
