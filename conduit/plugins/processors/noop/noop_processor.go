package noop

import (
	"context"
	_ "embed" // used to embed config

	"github.com/sirupsen/logrus"

	"github.com/algorand/indexer/conduit"
	"github.com/algorand/indexer/conduit/data"
	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/processors"
)

// PluginName to use when configuring.
const PluginName = "noop"

// package-wide init function
func init() {
	processors.Register(PluginName, processors.ProcessorConstructorFunc(func() processors.Processor {
		return &Processor{}
	}))
}

// Processor noop
type Processor struct{}

//go:embed sample.yaml
var sampleConfig string

// Metadata noop
func (p *Processor) Metadata() conduit.Metadata {
	return conduit.Metadata{
		Name:         PluginName,
		Description:  "noop processor",
		Deprecated:   false,
		SampleConfig: sampleConfig,
	}
}

// Config noop
func (p *Processor) Config() string {
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
