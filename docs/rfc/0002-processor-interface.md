# Processor Interface RFC

- Contribution Name: Processor Interface/Plugin Framework
- Implementation Owner: Stephen Akiki

## Problem Statement

Users of the Indexer each have their own needs when it comes to the data gathered by the Indexer. Whether it is
extracting certain fields of information or transforming it entirely, the data gathered is something that every user
consumes in a different way.

Currently, users must "drink from a firehouse" of information in that all information is presented at all times. No
custom "pre-processing" of the data can be done so no derivative information can be created. This means that the user
must save all the data and only after that is accomplished can they use the information. This can result in an
incredible amount of "wasted" storage if the user only needs to save a derived value and not all underlying data.

Additionally, there might be various transformations that are useful enough that one would wish to "pipe" them from one
to another, similar to how commands are piped in Linux. This would allow for modules to be saved and re-used across
Indexer deployments.

## Proposal

### Plugin Interface

A sample interface and pipeline effort is shown below:

```go
package processor

// Config will act as a placeholder for now. It will end up providing an interface for
// serialization/deserialization of config files.
// Derived types will provide plugin-specific data fields.
type Config string

type MetaData string

type Processor interface {
	// Metadata associated with each Processor.
	Metadata() MetaData

	// Config returns the configuration options used to create an Processor.
	Config() Config

	// Init will be called during initialization, before block data starts going through the pipeline.
	// Typically, used for things like initializing network connections.
	// The Context passed to Init() will be used for deadlines, cancel signals and other early terminations
	// The Config passed to Init() will contain the unmarshalled config file specific to this plugin.
	Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig) error

	// Close will be called during termination of the Indexer process.
	Close() error

	// Process will be called with provided optional inputs.  It is up to the plugin to check that required inputs are provided.
	Process(input data.BlockData) (data.BlockData, error)
}
```

### Interface Description

#### Metadata and Configuration

The `Metadata()` and `Config()` functions are used to get the supplied information of the interface. The information
will be returned as a string, which can be used along-side conduit tooling to de-serialize into appropriate objects.

Plugin Metadata shall adhere to the Conduit `PluginMetadata` interface and return the appropriate type depending on the
defined plugin.  For more info, see Conduit documentation.

Likewise, plugin Config data shall be a string that is convertable from the YAML specification into a Go-Lang object.
This is also a common interface defined in the Conduit framework.

#### Init and Close

The `Init()` and `Close()` functions serve as "hooks" into the Conduit start up interface. They allow the plugin writer
to write startup and shutdown functions in a sequence defined by the Conduit framework. An example would be to have a
processor setup stateful information (like a database or special file) during the `Init()` call and flush to disk and/or
close that stateful information on `Close()`.

The `Init()` function is given a configuration parameter which it can use to determine how to initialize. The
configuration is a string type that can be de-serialized by the Conduit framework into an object that can be
programmatically accessed. It is also given a `context` parameter which will be used by the Conduit framework to
terminate the runtime early if needed.  Finally, a `InitProvider` parameter is provided so that a processor can access
information needed from the blockchain.

Note: the `Close()` function will only be called on an orderly shutdown of the Conduit framework. The Conduit framework
can not guarantee this function is called if any abnormal shutdown (i.e. signals, un-caught panics) occurs.



Conduit documentation will delve into further detail about order of operations and how these functions will be called
relative to the lifecycle of the plugin and the framework itself.

#### Process

The `Process()` function is the heart of any processor. It is given an input parameter of type `BlockData` and is
expected to perform processing based on these inputs. However, to propagate the processing done on the input data one
must return a corresponding `BlockData` object in the return value.

The `BlockData` object is a common object shared among all plugins within the Conduit framework. It is passed into the
processor plugin via the input in an already initialized and correctly formed state.

Valid processing of the block is signified by having the returned `error` value be nil. If this is the case then the
output `BlockData` will be assumed to be correctly processed and be free of any errors. **It is imperative that if
the `error` value is nil then the `BlockData` is assumed to contain all valid data.**

It is valid to return the same `BlockData` values provided in the input as the output if one wishes to perform a "no
operation" on the block. However, note that further plugins (whether more processors and/or exporters) will take action
on this data.

It is also valid to return an "empty" block. An empty block is one in which `BlockData.Empty()` returns true without any
errors. Returning an empty block could indicate that all data has been filtered out which would cause all downstream
plugins to perform no operation.

Below is a collection of common scenarios and the appropriate way to fill out the data in the output `BlockData`:

**Scenario 1: An error occurred when processing the block**

The values within the output `BlockData` can be anything, however the returned error must be non-nil. This represents
that the data contained within the `BlockData` is incorrect and/or could not be processed. Depending on the error type
returned, this could result in a retry of the pipeline but isn't guaranteed to.

**Scenario 2: Some processing occurred successfully**

The values within the output `BlockData` match what the processing says should have happened and the returned error must
be nil.

**Scenario 3: Processing filtered out all data**

If a processor has filtered out all data, then it will have the `Payset` field set to nil. This will indicate that no
transactions are available to process.

##### Default Output Values

If a plugin does not modify a variable of the output `BlockData` then the value of the variable shall be what the
corresponding variable was in the input `BlockData`. 

#### On Complete

Often it is useful to know when a block has passed through the entire Conduit framework and has successfully been written by an Exporter.  For instance,
a stateful Processor plugin might want to commit block information to a private database once it is certain that the block information has been fully processed.

The `OnComplete()` function will be called by the Conduit framework when the exporter plugin has finished executing **and** did not return any errors.  The data supplied to this function
will be the final block data written to disk.  Note that this block data may have been processed by other Processor plugins since it was last "seen" by any specific Processor plugin.

It is implemented as a dynamic interface and can be added to any plugin.
```go
type Completed interface {
	// OnComplete will be called by the Conduit framework when the exporter has successfully written the block
	// The input will be the finalized information written to disk
	OnComplete(input data.BlockData) error
}
```
