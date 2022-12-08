# Getting Started


## Installation

### Install from Source

1. Checkout the repo, or download the source, `git clone https://github.com/algorand/indexer.git && cd indexer`
2. Run `make conduit`.
3. The binary is created at `cmd/conduit/conduit`.

### Go Install

Go installs of the indexer repo do not currently work because of its use of the `replace` directive to support the 
go-algorand submodule. 

**In Progress**
There is ongoing work to remove go-algorand entirely as a dependency of indexer/conduit. Once
that work is complete users should be able to use `go install` to install binaries for this project.

## Quick Start

Conduit requires a configuration file to setup and run a data pipeline. To generate an initial skeleton for a conduit
config file, you can run `./conduit init`. This will setup a sample data directory with a config located at
`data/conduit.yml`.

You will need to manually edit the data in the config file, filling in correct values, to get conduit running properly.

Once you have a valid config file, simply run `./conduit -d data` to launch conduit with the `data` directory.


# Configuration and Plugins
Conduit comes with an initial set of plugins available for use in pipelines. For more information on the possible
plugins and how to include these plugins in your pipeline's configuration file see [Configuration.md](Configuration.md).
