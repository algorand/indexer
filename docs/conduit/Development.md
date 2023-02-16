# Creating A Plugin

There are three different interfaces to implement, depending on what sort of functionality you are adding:
* Importer: for sourcing data into the system.
* Processor: for manipulating data as it goes through the system.
* Exporter: for sending processed data somewhere.

All plugins should be implemented in the respective `importers`, `processors`, or `exporters` package.

# Registering a plugin

## Register the Constructor

The constructor is registered to the system by name in the init this is how the configuration is able to dynamically create pipelines:
```
func init() {
	exporters.RegisterExporter(noopExporterMetadata.ExpName, exporters.ExporterConstructorFunc(func() exporters.Exporter {
		return &noopExporter{}
	}))
}
```

There are similar interfaces for each plugin type.

## Load the Plugin

Each plugin package contains an `all.go` file. Add your plugin to the import statement, this causes the init function to be called and ensures the plugin is registered.

# Implement the interface

Generally speaking, you can follow the code in one of the existing plugins.

# Lifecycle

## Init

Each plugin will have it's `Init` function called once as the pipline is constructed.

The context provided to this function should be saved, and used to terminate any long-running operations if necessary.

## Per-round function

Each plugin type has a function which is called once per round:
* Importer: `GetBlock` called when a particular round is required. Generally this will be increasing over time.
* Processor: `Process` called to process a round.
* Exporter: `Receive` for consuming a round.

## Close

Called during a graceful shutdown. We make every effort to call this function, but it is not guaranteed.

## Hooks

There are special lifecycle hooks that can be registered on any plugin by implementing additional interfaces.

### Completed

When all processing has completed for a round, the `OnComplete` function is called on any plugin that implements it.

```go
// Completed is called by the conduit pipeline after every exporter has
// finished. It can be used for things like finalizing state.
type Completed interface {
	// OnComplete will be called by the Conduit framework when the pipeline
	// finishes processing a round.
	OnComplete(input data.BlockData) error
}
```

### PluginMetrics

After the pipeline has been initialized, and before it has been started, plugins may provide prometheus metric handlers. The subsystem is a configurable value that should be passed into the Prometheus metric constructors.
The ProvideMetrics function will only be called once.

```go
// PluginMetrics is for defining plugin specific metrics
type PluginMetrics interface {
	ProvideMetrics(subsystem string) []prometheus.Collector
}
```
