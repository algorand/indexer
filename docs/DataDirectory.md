# Indexer Data Directory

The Indexer data directory is the location where the Indexer can store and/or load data needed for runtime operation and configuration. It is a required argument for Indexer daemon operation. Supply it to the Indexer via the `--data-dir` flag.

# Storage Requirements

As of mid-2022, approximately 20 GiB for Mainnet.

# Configuration Files

The data directory is the first place to check for different configuration files, for example:
- `indexer.yml` - Indexer Configuration File
- `api_config.yml` - API Parameter Enable/Disable Configuration File

## Read-Only

Indexer is effectively a read-only client to the PostgreSQL database. While the configuration must be provided in a data directory, it is only used for configuration.
