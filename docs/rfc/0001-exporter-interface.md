# Exporter Interface RFC

- Contribution Name: Exporter Interface/Plugin Framework
- Implementation Owner: Eric Warehime
- RFC PR: [algorand/indexer#1061](https://github.com/algorand/indexer/pull/1061)

## Summary

[summary]: #summary

This RFC outlines a general set of properties and components which will be implemented in order to establish
a framework for modular data export from Indexer. It will describe separate methods for including modules (plugins) 
within the Indexer repository, as well as formats and configurations for external plugins and how they will be
integrated into an Indexer's data pipeline at runtime.

## Problem Statement

[problem-statement]: #problem-statement

Users of Indexer want a way to easily output block data to one or more destinations. This may include standard databases
such as cloud-managed databases, self-managed SQL clusters, a streaming platform such as Kafka, etc. It may also include
more complex destinations such as indexing Algorand data via [The Graph](https://github.com/graphprotocol/indexer), or a hosted GraphQL platform such as
[Fauna](https://github.com/fauna).

The current implementation of Indexer has a single output (which can optionally be disabled) which is heavily tied to
Postgresql. While there can potentially be many exporter implementations created in the repo which use the existing
interface and extend functionality to new data outputs, we will be unable to implement all of them and maintain them
adequately. And any complex data outputs such as those mentioned above will be blocked--users cannot easily use the
existing interface which Indexer exposes internally to customize their data output.

Once we have a fully implemented exporter plugin interface, users of Indexer will be able to easily develop and
run plugins which encapsulate their own business logic for data destinations. Some of these will be generic enough that
we will maintain them centrally in our Indexer repository. Others will be specific to a user's needs and will be
maintained either publicly or privately  by them. This plugin specification will also give us a runtime framework and
configuration necessary for creating other plugin systems, such as input plugins and intermediate plugins.

## Design proposal

[design-proposal]: #design-proposal

### Indexer-Native Exporter Plugins  
Exporter plugins that are native to the Indexer (maintained within the Indexer repository) will necessarily behave
differently than those which are maintained and built separately and loaded into the pipeline at runtime. The following
section will outline the properties of so called "Indexer-native" plugins, which will then be extended in the subsequent
section to incorporate non-native plugins.

***Plugin Interface***  
Exporter plugins that are native to the Indexer (maintained within the Indexer repository) will each implementation the exporter interface:
```Go
package exporters

// ExporterConfig will act as a placeholder for now. It will end up providing an interface for
// serialization/deserialization of config files.
// Derived types will provide plugin-specific data fields.
type ExporterConfig interface {}


// Exporter defines the methods invoked during the plugin lifecycle of a data exporter.
type Exporter interface { 
    // Metadata associated with each Exporter.
	Metadata() ExporterMetadata
	
    // Connect will be called during initialization, before block data starts going through the pipeline.
    // Typically used for things like initializating network connections.
    // The ExporterConfig passed to Connect will contain the Unmarhsalled config file specific to this plugin.
    Connect(cfg ExporterConfig) error
	
	// Config returns the configuration options used to create an Exporter.
	// Initialized during `Connect`, it will return nil until the Exporter has been Connected.
	Config() ExporterConfig
  
    // Disconnect will be called during termination of the Indexer process.
    // There is no guarantee that plugin lifecycle hooks will be invoked in any specific order in relation to one another.
    Disconnect() error
  
    // Receive is called for each block to be processed by the exporter.
    Receive(exportData ExportData) error

    // Round returns the next round not yet processed by the Exporter. Atomically updated when Receive successfully completes.
    Round() uint64
}
```

***Plugin Config***
* A config file that defines all parameters that can be supplied to the plugin, and which provides the default values
that will be used for each parameter. A yaml file stored inside the Indexer data directory stores all of the config data
for a given plugin. The Indexer will use the type specified in the Indexer config to look for a plugin config which
satisfies that class, and then load that as the selected plugin. Supplying multiple plugin configs for the selected type
of exporter will result in a random config being chosen. In the future we may evolve this to support multiple plugins
of the same type via a method of differentiation.
```YAML
name: "postgresql-exporter"
type: "IndexerPostgresqlExporter"
properties:
  username: "foo"
  password: "bar"
  host: "127.0.0.1"
  port: "1234"
  dbname: "indexer"
```

**Indexer Config**

The Indexer's config will need to be changed to incorporate plugins. Future RFCs can decide how to string multiple
pipelines and plugins together in interesting ways. For now, we will have a single pipeline per indexer--i.e. running the
indexer will only run a single data pipeline and therefore have a single exporter. In that way the initial changes will
be easy to make--one new field in the config which provides an internal plugin name. Config files for the selected
plugin will then be resolved and parsed at runtime. The default configuration for Indexer will select the Postgresql
plugin in order to maintain backwards compatibility with the existing Indexer pipeline.

***Plugin Execution Framework***
The plugin execution framework will have the following features.
* Guaranteed block processing

The existing Indexer implementation uses the postgres database for multiple purposes--both for storing data about the
state of the system such as the most recently processed round or the migration status, as well as for storing the actual
block data being exported. The new plugin execution framework will not be able to rely on the export destination to
store state, so it will use its own data store to flush state to disk. One of the main purposes of this will be to
ensure only-once, in order block processing, but it can also be exposed to plugins that need to store state, or for
other purposes as they are uncovered.

* Built In Data Format Definitions

Though not being defined here, we can imagine that intermediate plugins in our execution framework might perform common
data operations such as aggregation, filtering, annotation, etc. The result of some of these operations will be a subset
of full block data, while others may result in block data which does not conform to the block specification at all.
In order to accommodate customization of the data, we will provide multiple data formats--the standard Block interface
used today, as well as more generic forms that plugins can deserialize based on their needs. A given plugin will need to
specify the data format it expects for both input data and output if it provides any (exporters obviously don't have
output data in this sense). The system will ensure that the constructed pipeline has compatible data formats during
initialization.

Here is the data that will be initially passed into an Exporter.
```GO
package exporters

type BlockExportData struct {
	// Block is the block data written to the blockchain.
	Block bookkeping.Block
	
	//Delta contains a list of account changes resulting from the block. Processor plugins may have modified this data.
	Delta ledgercore.StateDelta
	
	// Certificate contains voting data that certifies the block. The certificate is non deterministic,
	// a node stops collecting votes once the voting threshold is reached.
	Certificate agreement.Certificate
}
```

### External Plugins

In order to enable users to develop and maintain their own plugins external to the Indexer repository, the plugin
framework will allow users to specify an arbitrary process as part of a pipeline. To allow users to configure a process
as a plugin, Indexer will provide standard internal plugins that will wrap the processes and manage their state. Users
will create a config file that tells Indexer how to start the process, its data formats, and any other relevant
arguments specific to the plugin's execution. Other than that the external plugin will be opaque to the Indexer--it
will only know how to send and receive data and ensure that a block was acted on by the plugin.


External plugin development will then require the Indexer to provide a lightweight process manager, process plugin
templates, and a way to facilitate streaming of block data in and out of these processes (in addition to any other state
data that will be useful for a process plugin to communicate).

### End State

Once this RFC is implemented, the Indexer will have a set of exporter plugins defined that can be substituted for one
another. The default plugin will be the existing Postgresql exporter, but we will also initially support a no-op
exporter and a stdout exporter which will drop blocks and echo them to stdout respectively.

We will also be able to create exporter plugins external to the Indexer repository. In order to enable this we will have
implemented a minimal version of the execution framework which is described in various parts above, and there will be
a specific exporter implementation which will manage external plugin processes and provide them with data.


### Prior art

[prior-art]: #prior-art

A lot of this RFC takes inspiration from the [Telegraf](https://github.com/influxdata/telegraf) metrics pipelines--a
framework for server side metrics collection. They provide configurable plugins for metrics inputs, outputs, and
aggregators/processors. Their plugin system also provides "shims" which use their custom process manager to enable
external processes for use as plugins. These shims read/write serialized data in the plugin's specified data format to
stdin/stdout in order communicate with the processes. Their system uses a polling model in order to receive data from
external plugins, and has configurable flushing to control sending data to plugins.


Kafka also has a plugin-like system with Kafka Connect. It allows external parties to define Conectors which export or
import data to their Kafka pipeline. The users have to implement this as a subclass/interface which is provided by
Kafka. Because Kafka runs in the JVM it can correspond directly with custom classes using this interface, but obviously
it requires users to write Java code and use the Kafka library as a dependency which is much more restrictive than the
external process approach.


### Unresolved questions

[unresolved-questions]: #unresolved-questions

- How should process communication be handled?
  - Telegraf uses stdin/stdout to communicate w/ processes. Kafka is mostly built around APIs, similarly things like
docker/linkerd/containerd use HTTP endpoints to standardize communication (though they mostly use this for config data
instead of streaming data). 
  - I support using sockets--very standard and well known programming interface,and has useful libraries built
around it unlike using process input/output which may have additional OS-specific challenges.

### Future possibilities

[future-possibilities]: #future-possibilities

This is the first RFC in the series which will be focused on our pipeline/plugin architecture for the Indexer. Future
additions will modify and extend the framework in order to accommodate additional requirements of new plugin types and
new plugins. 
