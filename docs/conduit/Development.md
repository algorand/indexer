# Creating A Plugin

There are three different interfaces to implement, depending on what sort of functionality you are adding:
* Importer: for sourcing data into the system.
* Processor: for manipulating data as it goes through the system.
* Exporter: for sending processed data somewhere.

All plugins should be implemented in the respective `importers`, `processors`, or `exporters` package.

# Registering a plugin

## Constructor

Each plugin must implement a constructor which can instantiate itself.
```
// Constructor is the ExporterConstructor implementation for the "noop" exporter
type Constructor struct{}

// New initializes a noopExporter
func (c *Constructor) New() exporters.Exporter {
	return &noopExporter{
		round: 0,
	}
}
```

## Register the Constructor

The constructor is registered to the system by name in the init this is how the configuration is able to dynamically create pipelines:
```
func init() {
	exporters.RegisterExporter(noopExporterMetadata.ExpName, &Constructor{})
}
```

There are similar interfaces for each plugin type.

## Load the Plugin

Each plugin package contains an `all.go` file. Add your plugin to the import statement, this causes the init function to be called and ensures the plugin is registered.

# Implement the interface

Generally speaking, you can follow the code in one of the existing plugins.
