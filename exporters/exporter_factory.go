package exporters

import (
	"fmt"
	"sort"
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

// exporterImpls is a k/v store from exporter names to their constructor implementations.
// This layer of indirection allows for different exporter integrations to be compiled in or compiled out by `go build --tags ...`
var exporterImpls = make(map[string]ExporterConstructor)

// RegisterExporter is used to register ExporterConstructor implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// driver's are configured and used.
func RegisterExporter(name string, constructor ExporterConstructor) {
	exporterImpls[name] = constructor
}

// ExporterBuilderByName returns a Processor constructor for the name provided
func ExporterBuilderByName(name string) (ExporterConstructor, error) {
	constructor, ok := exporterImpls[name]
	if !ok {
		return nil, fmt.Errorf("no Exporter Constructor for %s", name)
	}

	return constructor, nil
}

// ExporterNames returns the names of all exporters registered
func ExporterNames() []string {
	var returnValue []string
	for k := range exporterImpls {
		returnValue = append(returnValue, k)
	}
	sort.Strings(returnValue)
	return returnValue
}
