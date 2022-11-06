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

// Processors are the constructors to build processor plugins.
var Processors = make(map[string]ProcessorConstructor)

// Register is used to register ProcessorConstructor implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// drivers are configured and used.
func Register(name string, constructor ProcessorConstructor) {
	Processors[name] = constructor
}

// ProcessorBuilderByName returns a Processor constructor for the name provided
func ProcessorBuilderByName(name string) (ProcessorConstructor, error) {
	constructor, ok := Processors[name]
	if !ok {
		return nil, fmt.Errorf("no Processor Constructor for %s", name)
	}

	return constructor, nil
}
