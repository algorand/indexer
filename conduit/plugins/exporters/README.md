# Exporter Plugins
## Developing a new Exporter ##
Each Exporter lives in its own module inside the `exporters` directory. We have provided an `example` exporter in `exporters/example` which should be used as a starting point.
Run `cp -r exporters/example exporters/my_exporter` to begin defining a new exporter definition. Interface methods in the example exporter have not been defined, but the Exporter interface attempts to give some background their purpose.
```
type Exporter interface {
	// Metadata associated with each Exporter.
	Metadata() ExporterMetadata

	// Init will be called during initialization, before block data starts going through the pipeline.
	// Typically used for things like initializing network connections.
	// The ExporterConfig passed to Connect will contain the Unmarhsalled config file specific to this plugin.
	// Should return an error if it fails--this will result in the Indexer process terminating.
	Init(cfg plugins.PluginConfig, logger *logrus.Logger) error

	// Config returns the configuration options used to create an Exporter.
	// Initialized during Connect, it should return nil until the Exporter has been Connected.
	Config() plugins.PluginConfig

	// Close will be called during termination of the Indexer process.
	// There is no guarantee that plugin lifecycle hooks will be invoked in any specific order in relation to one another.
	// Returns an error if it fails which will be surfaced in the logs, but the process is already terminating.
	Close() error

	// Receive is called for each block to be processed by the exporter.
	// Should return an error on failure--retries are configurable.
	Receive(exportData data.BlockData) error
	
}
```

## Exporter Configuration
In order to avoid requiring the Conduit framework from knowing how to configure every single plugin, it is necessary to be able to construct every plugin without configuration data.
The configuration for a plugin will be passed into the constructed exporter object via the `Init` function as a `plugin.PluginConfig` (string) object where it can be deserialized inside the plugin.

Existing plugins and their configurations use yaml for their serialization format, so all new exporters should also define their configs via a yaml object. For an example of this, take a look at the [postgresql exporter config](postgresql/postgresql_exporter_config.go).

## Testing
The provided example has a few basic test definitions which can be used as a starting point for unit testing. All exporter modules should include unit tests out of the box.
Support for integration testing is TBD.

## Common Code
Any code necessary for the execution of an Exporter should be defined within the exporter's directory. For example, a MongoDB exporter located in `exporters/mongodb/` might have connection utils located in `exporters/mongodb/connections/`.
If code can be useful across multiple exporters it may be placed in its own module external to the exporters.

## Work In Progress
* There is currently no defined process for constructing a Conduit pipeline, so you cannot actually use your new Exporter. We are working on the Conduit framework which will allow users to easily construct block data pipelines.
* Config files are read in based on a predefined path which is different for each plugin. That means users' Exporter configs will need to be defined in their own file.
* We are working to determine the best way to support integration test definitions for Exporter plugins.