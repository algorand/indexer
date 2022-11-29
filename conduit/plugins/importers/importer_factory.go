package importers

import (
	"fmt"
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

// Importers are the constructors to build importer plugins.
var Importers = make(map[string]Constructor)

// Register is used to register Constructor implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// drivers are configured and used.
func Register(name string, constructor Constructor) {
	Importers[name] = constructor
}

// ImporterBuilderByName returns a Importer constructor for the name provided
func ImporterBuilderByName(name string) (Constructor, error) {
	constructor, ok := Importers[name]
	if !ok {
		return nil, fmt.Errorf("no Importer Constructor for %s", name)
	}

	return constructor, nil
}
