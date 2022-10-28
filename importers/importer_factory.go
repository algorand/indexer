package importers

import (
	"fmt"
	"sort"
)

// Constructor must be implemented by each Importer.
// It provides a basic no-arg constructor for instances of an ImporterImpl.
type Constructor interface {
	// New should return an instantiation of a Importer.
	// Configuration values should be passed and can be processed during `Init()`.
	New() Importer
}

// ImporterConstructorFunc is Constructor implementation for importers
type ImporterConstructorFunc func() Importer

// New initializes an importer constructor
func (f ImporterConstructorFunc) New() Importer {
	return f()
}

// importerImpls is a k/v store from importer names to their constructor implementations.
// This layer of indirection allows for different importer integrations to be compiled in or compiled out by `go build --tags ...`
var importerImpls = make(map[string]Constructor)

// RegisterImporter is used to register Constructor implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// driver's are configured and used.
func RegisterImporter(name string, constructor Constructor) error {
	if _, ok := importerImpls[name]; ok {
		return fmt.Errorf("importer already exists")
	}
	importerImpls[name] = constructor
	return nil
}

// ImporterBuilderByName returns a Importer constructor for the name provided
func ImporterBuilderByName(name string) (Constructor, error) {
	constructor, ok := importerImpls[name]
	if !ok {
		return nil, fmt.Errorf("no Importer Constructor for %s", name)
	}

	return constructor, nil
}

// ImporterNames returns the names of all importers registered
func ImporterNames() []string {
	var returnValue []string
	for k := range importerImpls {
		returnValue = append(returnValue, k)
	}
	sort.Strings(returnValue)
	return returnValue
}
