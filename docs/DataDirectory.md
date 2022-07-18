# Indexer Data Directory

The Indexer data directory is the location where the Indexer can store and/or load data needed for runtime operation and configuration. It is a required argument for Indexer daemon operation. Supply it to the Indexer via the `--data-dir` flag.

# Storage Requirements

As of mid-2022, approximately 15GiB for Mainnet.

# Configuration Files

The data directory is the first place to check for different configuration files, for example:
- `indexer.yml` - Indexer Configuration File
- `api_config.yml` - API Parameter Enable/Disable Configuration File

# Account Cache

Indexer writers keep an account cache in the data directory. This cache is used during block processing to compute things like the new account balances after processing transactions. Prior to this local cache, the database was queried on each round to fetch the initial account states.

## Read-Only Mode

The account cache is not required when in read-only mode. While the data directory is still required, it will only be used for configuration.

# Initialization

If a new data directory must be created, the following process should be used:
1. Review the Indexer log to find the most recent round that was processed. For example, `22212765` in the following line:
   ```
   {"level":"info","msg":"round r=22212765 (49 txn) imported in 139.782694ms","time":"2022-07-18T19:23:13Z"} 
   ```
2. Lookup the most recent catchpoint **without going over** from the list for your network:
   [Mainnet]()
   [Testnet]()
   [Betanet]()
3. Supply the catchpoint label when starting Indexer using the command line setting `--catchpoint 6500000#1234567890ABCDEF01234567890ABCDEF0`, setting `catchpoint` in `indexer.yml`, or setting the `INDEXER_CATCHPOINT` environment variable.

While Indexer starts, you can see progress information printed periodically in the log file.
