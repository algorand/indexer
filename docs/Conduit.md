<div style="text-align:center" align="center">
  <picture>
    <img src="./assets/algorand_logo_mark_black.svg" alt="Algorand" width="400">
    <source media="(prefers-color-scheme: dark)" srcset="./assets/algorand_logo_mark_white.svg">
    <source media="(prefers-color-scheme: light)" srcset="./assets/algorand_logo_mark_black.svg">
  </picture>

[![CircleCI](https://img.shields.io/circleci/build/github/algorand/indexer/develop?label=develop)](https://circleci.com/gh/algorand/indexer/tree/develop)
[![CircleCI](https://img.shields.io/circleci/build/github/algorand/indexer/master?label=master)](https://circleci.com/gh/algorand/indexer/tree/master)
![Github](https://img.shields.io/github/license/algorand/indexer)
[![Contribute](https://img.shields.io/badge/contributor-guide-blue?logo=github)](https://github.com/algorand/go-algorand/blob/master/CONTRIBUTING.md)
</div>

# Algorand Conduit

Conduit is a framework which provides reusable components necessary for ingesting blocks from the Algorand blockchain into external applications. It is designed around a modular plugin system that allows users to configure their own data pipelines for filtering, aggregation, and storage of transactions and accounts on any Algorand network.

## Building from source

Development is done using the [Go Programming Language](https://golang.org/), the version is specified in the project's [go.mod](go.mod) file.

Run `make` to build Conduit, the binary is located at `cmd/algorand-indexer/conduit`.

# Quickstart

See the [Getting Started](conduit/GettingStarted.md) page.

# Configuration

See the [Configuration](conduit/Configuration.md) page.

# Develoment

See the [Development](conduit/Development.md) page for building a plugin.

# Features

# Plugin System
A Conduit pipeline is composed of 3 components, [Importers](../importers/README.md), [Processors](../processors/README.md), and [Exporters](../exporters/README.md).
Every pipeline must define exactly 1 Importer, 1 Exporter, and can optionally define a series of 0 or more Processors.

The original Algorand Indexer has been defined as a Conduit pipeline via the [algorand-indexer](../cmd/algorand-indexer/daemon.go) executable, see [Migrating from Indexer](#migrating-from-indexer)

# Contributing

Contributions welcome! Please refer to our [CONTRIBUTING](https://github.com/algorand/go-algorand/blob/master/CONTRIBUTING.md) document for general contribution guidelines, and individual plugin documentation for contributing to new and existing Conduit plugins.

## RFCs
If you have an idea for how to improve Conduit that would require significant changes, open a [Feature Request Issue](https://github.com/algorand/indexer/issues/new/choose) to begin discussion. If the proposal is accepted, the next step is to define the technical direction and answer implementation questions via a PR containing an [RFC](./rfc/template.md).  

You do _not_ need to open an RFC for adding a new plugin--follow the development guide for the corresponding plugin type.

<!-- USAGE_START_MARKER -->

# Common Setups

The most common usage of Conduit is expecting to be getting validated blocks from a local `algod` Algorand node, adding them to a database (such as [PostgreSQL](https://www.postgresql.org/)), and serving an API to make available a variety of prepared queries. Some users may wish to directly write SQL queries of the database.

Conduit works by fetching blocks one at a time via the configured Importer, sending the block data through the configured Processors, and terminating block handling via an Exporter (traditionally a database).


<!-- USAGE_END_MARKER_LINE -->

# Migrating from Indexer

Indexer was built in a way that strongly coupled it to Postgresql, and the defined REST API. We've built Conduit in a way which is backwards compatible with the preexisting Indexer application. Running the `algorand-indexer` binary will use Conduit to construct a pipeline that replicates the Indexer functionality.

Going forward we will continue to maintain the Indexer application, however our main focus will be enabling and optimizing a multitude of use cases through the Conduit pipeline design rather the singular Indexer pipeline.
