package exporters

import (
	"fmt"
)

// ExporterConstructor must be implemented by each Exporter.
// It provides a basic no-arg constructor for instances of an ExporterImpl.
type ExporterConstructor interface {
	// New should return an instantiation of an Exporter.
	// Configuration values should be passed and can be processed during `Init()`.
	New() Exporter
}

// ExporterConstructorFunc is Constructor implementation for exporters
type ExporterConstructorFunc func() Exporter

// New initializes an exporter constructor
func (f ExporterConstructorFunc) New() Exporter {
	return f()
}

// Exporters are the constructors to build exporter plugins.
var Exporters = make(map[string]ExporterConstructor)

// Register is used to register ExporterConstructor implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// drivers are configured and used.
func Register(name string, constructor ExporterConstructor) {
	Exporters[name] = constructor
}

// ExporterBuilderByName returns a Processor constructor for the name provided
func ExporterBuilderByName(name string) (ExporterConstructor, error) {
	constructor, ok := Exporters[name]
	if !ok {
		return nil, fmt.Errorf("no Exporter Constructor for %s", name)
	}

	return constructor, nil
}
