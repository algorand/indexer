package processors

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/plugins"
)

// ProcessorConstructor must be implemented by each Processor.
// It provides a basic no-arg constructor for instances of an ProcessorImpl.
type ProcessorConstructor interface {
	// New should return an instantiation of a Processor.
	// Configuration values should be passed and can be processed during `Init()`.
	New() Processor
}

// processorImpls is a k/v store from processor names to their constructor implementations.
// This layer of indirection allows for different processor integrations to be compiled in or compiled out by `go build --tags ...`
var processorImpls = make(map[string]ProcessorConstructor)

// RegisterProcessor is used to register ProcessorConstructor implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// driver's are configured and used.
func RegisterProcessor(name string, constructor ProcessorConstructor) error {
	if _, ok := processorImpls[name]; ok {
		return fmt.Errorf("processor already exists")
	}
	processorImpls[name] = constructor
	return nil
}

// ProcessorByName is used to construct an Processor object by name.
// Returns a Processor object
func ProcessorByName(ctx context.Context, name string, dataDir string, initProvider data.InitProvider, logger *logrus.Logger) (Processor, error) {
	var constructor ProcessorConstructor
	var ok bool
	if constructor, ok = processorImpls[name]; !ok {
		return nil, fmt.Errorf("no Processor Constructor for %s", name)
	}
	processor := constructor.New()
	cfg := plugins.LoadConfig(logger, dataDir, processor.Metadata())
	if err := processor.Init(ctx, initProvider, cfg, logger); err != nil {
		return nil, err
	}
	return processor, nil
}

// ProcessorBuilderByName returns a Processor constructor for the name provided
func ProcessorBuilderByName(name string) (ProcessorConstructor, error) {
	constructor, ok := processorImpls[name]
	if !ok {
		return nil, fmt.Errorf("no Processor Constructor for %s", name)
	}

	return constructor, nil
}
