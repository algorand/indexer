## Migrating from Indexer to Conduit

The [Algorand Indexer](https://github.com/algorand/indexer) provides both a block processing pipeline to ingest block
data from an Algorand node into a Postgresql database, and a rest API which serves that data.

The [Conduit](https://github.com/algorand/indexer/blob/develop/docs/Conduit.md) project provides a modular pipeline
system allowing users to construct block processing pipelines for a variety of use cases as opposed to the single,
bespoke Indexer construction.

### Migration
Talking about a migration from Indexer to Conduit is in some ways difficult because they only have partial overlap in
their applications. For example, Conduit does _not_ include a rest API or 