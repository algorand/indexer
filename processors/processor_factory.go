package processors

import (
	"fmt"
)

// ProcessorConstructor must be implemented by each Processor.
// It provides a basic no-arg constructor for instances of an ProcessorImpl.
type ProcessorConstructor interface {
	// New should return an instantiation of a Processor.
	// Configuration values should be passed and can be processed during `Init()`.
	New() Processor
}

// ProcessorConstructorFunc is Constructor implementation for processors
type ProcessorConstructorFunc func() Processor

// New initializes a processor constructor
func (f ProcessorConstructorFunc) New() Processor {
	return f()
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

// ProcessorBuilderByName returns a Processor constructor for the name provided
func ProcessorBuilderByName(name string) (ProcessorConstructor, error) {
	constructor, ok := processorImpls[name]
	if !ok {
		return nil, fmt.Errorf("no Processor Constructor for %s", name)
	}

	return constructor, nil
}
