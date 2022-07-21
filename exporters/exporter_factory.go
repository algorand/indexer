package exporters

import (
	"fmt"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
)

// ExporterConstructor must be implemented by each Exporter.
// It provides a basic no-arg constructor for instances of an ExporterImpl.
type ExporterConstructor interface {
	// New should return an instantiation of an Exporter.
	// Configuration values should be passed and can be processed during `Connect()`.
	New() Exporter
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

// ExporterByName is used to construct an Exporter object by name.
// Returns an Exporter object, an availability channel that closes when the database
// becomes available, and an error object.
func ExporterByName(name string, dataDir string, logger *logrus.Logger) (Exporter, error) {
	var constructor ExporterConstructor
	var ok bool
	if constructor, ok = exporterImpls[name]; !ok {
		return nil, fmt.Errorf("no Exporter Constructor for %s", name)
	}
	val := constructor.New()
	cfg := plugins.LoadConfig(logger, dataDir, val.Metadata())
	if err := val.Connect(cfg, logger); err != nil {
		return nil, err
	}
	return val, nil
}
