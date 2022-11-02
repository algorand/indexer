package noop

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
)

const implementationName = "noop"

// package-wide init function
func init() {
	processors.RegisterProcessor(implementationName, processors.ProcessorConstructorFunc(func() processors.Processor {
		return &Processor{}
	}))
}

// Processor noop
type Processor struct{}

// Metadata noop
func (p *Processor) Metadata() processors.ProcessorMetadata {
	return processors.MakeProcessorMetadata(implementationName, "noop processor", false)
}

// Config noop
func (p *Processor) Config() plugins.PluginConfig {
	return ""
}

// Init noop
func (p *Processor) Init(_ context.Context, _ data.InitProvider, _ plugins.PluginConfig, _ *logrus.Logger) error {
	return nil
}

// Close noop
func (p *Processor) Close() error {
	return nil
}

// Process noop
func (p *Processor) Process(input data.BlockData) (data.BlockData, error) {
	return input, nil
}
