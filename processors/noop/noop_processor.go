package noop

import (
	"context"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
)

const implementationName = "noop"

// package-wide init function
func init() {
	processors.RegisterProcessor(implementationName, &Constructor{})
}

// Constructor is the ProcessorConstructor implementation for the "noop" processor
type Constructor struct{}

// New initializes a noop constructor
func (c *Constructor) New() processors.Processor {
	return &Processor{}
}

type Processor struct{}

func (p *Processor) Metadata() processors.ProcessorMetadata {
	return processors.MakeProcessorMetadata(implementationName, "noop processor", false)
}

func (p *Processor) Config() plugins.PluginConfig {
	return ""
}

func (p *Processor) Init(_ context.Context, _ data.InitProvider, _ plugins.PluginConfig) error {
	return nil
}

func (p *Processor) Close() error {
	return nil
}

func (p *Processor) Process(input data.BlockData) (data.BlockData, error) {
	return input, nil
}

func (p *Processor) OnComplete(_ data.BlockData) error {
	return nil
}
