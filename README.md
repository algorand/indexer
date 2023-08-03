<div style="text-align:center" align="center">
  <picture>
    <img src="./docs/assets/algorand_logo_mark_black.svg" alt="Algorand" width="400">
    <source media="(prefers-color-scheme: dark)" srcset="./docs/assets/algorand_logo_mark_white.svg">
    <source media="(prefers-color-scheme: light)" srcset="./docs/assets/algorand_logo_mark_black.svg">
  </picture>

[![CircleCI](https://img.shields.io/circleci/build/github/algorand/indexer/develop?label=develop)](https://circleci.com/gh/algorand/indexer/tree/develop)
[![CircleCI](https://img.shields.io/circleci/build/github/algorand/indexer/master?label=master)](https://circleci.com/gh/algorand/indexer/tree/master)
![Github](https://img.shields.io/github/license/algorand/indexer)
[![Contribute](https://img.shields.io/badge/contributor-guide-blue?logo=github)](https://github.com/algorand/go-algorand/blob/master/CONTRIBUTING.md)
</div>

# Algorand Indexer

The Indexer is an API that provides search capabilities to Algorand on-chain data. Data is written by [Conduit](https://github.com/algorand/conduit/blob/master/docs/tutorials/IndexerWriter.md) to populate a PostgreSQL compaatible database.

## Building from source ##

Development is done using the [Go Programming Language](https://golang.org/), the version is specified in the project's [go.mod](go.mod) file.

Run `make` to build Indexer, the binary is located at `cmd/algorand-indexer/algorand-indexer`.

# Requirements

All recommendations here should be be used as a starting point. Further benchmarking should be done to verify performance is acceptible for any application using Indexer.

## Versions

* Database: [Postgres 14](https://www.postgresql.org/download/)

## System

For a simple deployment the following configuration works well:
* Network: Indexer, Algod and PostgreSQL should all be on the same network.
* Indexer: 2 CPU and 8 GB of ram.
* Conduit: 2 CPU and 8 GB of ram.
* Database: When hosted on AWS a `db.r5.xlarge` instance works well.
* Storage: 20 GiB

A database with replication can be used to scale read volume. Configure single Conduit writer with multiple Indexer readers.

# Quickstart

Indexer is part of the [sandbox](https://github.com/algorand/sandbox) private network configurations, which you can use to get started.

# Features

- Search and filter accounts, transactions, assets, and asset balances with many different parameters.
- Pagination of results.
- Enriched transaction and account data:
  - Confirmation round (block containing the transaction)
  - Confirmation time
  - Signature type
  - Close amounts
  - Create/delete rounds.
- Human readable field names instead of the space optimized protocol level names.

# Contributing

Contributions welcome! Please refer to our [CONTRIBUTING](https://github.com/algorand/go-algorand/blob/master/CONTRIBUTING.md) document.

<!-- USAGE_START_MARKER -->

# Usage

Configure Indexer with a PostgreSQL compatible database. Conduit should be used to populate the database and one or more Indexer daemons can be run to serve the API. For further isolation, a `readonly` user can be created for the database.
```
~$ algorand-indexer daemon --data-dir /tmp --postgres "user=readonly password=YourPasswordHere {other connection string options for your database}"
```

The Postgres backend does specifically note the username "readonly" and changes behavior to avoid writing to the database. But the primary benefit is that Postgres can enforce restricted access to this user. This can be configured with:
```sql
CREATE USER readonly LOGIN PASSWORD 'YourPasswordHere';
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO readonly;
```

## Authorization

When `--token your-token` is provided, an authentication header is required. For example:
```
~$ curl localhost:8980/transactions -H "X-Indexer-API-Token: your-token"
```

## Disabling Parameters

The Indexer has the ability to selectively enable or disable parameters for endpoints.  Disabling a "required" parameter will result in the entire endpoint being disabled while disabling an "optional" parameter will cause an error to be returned only if the parameter is provided.

### Viewing the Current Configuration

The Indexer has a default set of disabled parameters.  To view the disabled parameters issue:
```
~$ algorand-indexer api-config
```

This will output ONLY the disabled parameters in a YAML configuration.  To view all parameters (both enabled and disabled) issue:

```
~$ algorand-indexer api-config --all
```

### Interpreting The Configuration

Below is a snippet of the output from `algorand-indexer api-config`:

```
/v2/accounts:
    optional:
        - currency-greater-than: disabled
        - currency-less-than: disabled
/v2/assets/{asset-id}/transactions:
    optional:
        - note-prefix: disabled
        - tx-type: disabled
        - sig-type: disabled
        - before-time: disabled
        - after-time: disabled
        - currency-greater-than: disabled
        - currency-less-than: disabled
        - address-role: disabled
        - exclude-close-to: disabled
        - rekey-to: disabled
    required:
        - asset-id: disabled
```

Seeing this we know that the `/v2/accounts` endpoint will return an error if either `currency-greater-than` or `currency-less-than` is provided.  Additionally, because a "required" parameter is provided for `/v2/assets/{asset-id}/transactions` then we know this entire endpoint is disabled.  The optional parameters are provided so that you can understand what else is disabled if you enable all "required" parameters.

**NOTE: An empty parameter configuration file results in all parameters being ENABLED.**

For more information on disabling parameters see the [Disabling Parameters Guide](docs/DisablingParametersGuide.md).

## Metrics

The `/metrics` endpoint is configured with the `--metrics-mode` option and configures if and how [Prometheus](https://prometheus.io/) formatted metrics are generated.

There are different settings:

| Setting | Description |
| ------- | ----------- |
| ON      | Metrics for each REST endpoint in addition to application metrics. |
| OFF     | No metrics endpoint. |
| VERBOSE | Separate metrics for each combination of query parameters. This option should be used with caution, there are many combinations of query parameters which could cause extra memory load depending on usage patterns. |

## Connection Pool Settings

One can set the maximum number of connections allowed in the local connection pool by using the `--max-conn` setting.  It is recommended to set this number to be below the database server connection pool limit.

If the maximum number of connections/active queries is reached, subsequent connections will wait until a connection becomes available, or timeout according to the read-timeout setting.

# Settings

Settings can be provided from the command line, a configuration file, or an environment variable

| Command Line Flag (long)      | (short) | Config File                   | Environment Variable                  |
|-------------------------------|---------|-------------------------------|---------------------------------------|
| postgres                      | P       | postgres-connection-string    | INDEXER_POSTGRES_CONNECTION_STRING    |
| data-dir                      | i       | data                          | INDEXER_DATA                          |
| pidfile                       |         | pidfile                       | INDEXER_PIDFILE                       |
| server                        | S       | server-address                | INDEXER_SERVER_ADDRESS                |
| token                         | t       | api-token                     | INDEXER_API_TOKEN                     |
| metrics-mode                  |         | metrics-mode                  | INDEXER_METRICS_MODE                  |
| logfile                       | f       | logfile                       | INDEXER_LOGFILE                       |
| loglevel                      | l       | loglevel                      | INDEXER_LOGLEVEL                      |
| max-conn                      |         | max-conn                      | INDEXER_MAX_CONN                      |
| write-timeout                 |         | write-timeout                 | INDEXER_WRITE_TIMEOUT                 |
| read-timeout                  |         | read-timeout                  | INDEXER_READ_TIMEOUT                  |
| max-api-resources-per-account |         | max-api-resources-per-account | INDEXER_MAX_API_RESOURCES_PER_ACCOUNT |
| max-transactions-limit        |         | max-transactions-limit        | INDEXER_MAX_TRANSACTIONS_LIMIT        |
| default-transactions-limit    |         | default-transactions-limit    | INDEXER_DEFAULT_TRANSACTIONS_LIMIT    |
| max-accounts-limit            |         | max-accounts-limit            | INDEXER_MAX_ACCOUNTS_LIMIT            |
| default-accounts-limit        |         | default-accounts-limit        | INDEXER_DEFAULT_ACCOUNTS_LIMIT        |
| max-assets-limit              |         | max-assets-limit              | INDEXER_MAX_ASSETS_LIMIT              |
| default-assets-limit          |         | default-assets-limit          | INDEXER_DEFAULT_ASSETS_LIMIT          |
| max-balances-limit            |         | max-balances-limit            | INDEXER_MAX_BALANCES_LIMIT            |
| default-balances-limit        |         | default-balances-limit        | INDEXER_DEFAULT_BALANCES_LIMIT        |
| max-applications-limit        |         | max-applications-limit        | INDEXER_MAX_APPLICATIONS_LIMIT        |
| default-applications-limit    |         | default-applications-limit    | INDEXER_DEFAULT_APPLICATIONS_LIMIT    |
| enable-all-parameters         |         | enable-all-parameters         | INDEXER_ENABLE_ALL_PARAMETERS         |

## Command line

The command line arguments always take priority over the config file and environment variables.

```
~$ ./algorand-indexer daemon --data-dir /tmp --pidfile /var/lib/algorand/algorand-indexer.pid --postgres "host=mydb.mycloud.com user=postgres password=password dbname=mainnet"`
```

## Data Directory

The Indexer data directory is the location where the Indexer can store and/or load data needed for configuration. The data directory is effectively stateless, so it does not need to be persisted across deployments as long as the configuration is supplied.

**It is a required argument for Indexer daemon operation. Supply it to the Indexer via the `--data-dir`/`-i` flag.**

For more information on the data directory see [Indexer Data Directory](docs/DataDirectory.md).

### Auto-Loading Configuration

The Indexer will scan the data directory at startup and load certain configuration files if they are present.  The files are as follows:

- `indexer.yml` - Indexer Configuration File
- `api_config.yml` - API Parameter Enable/Disable Configuration File

**NOTE:** It is not allowed to supply both the command line flag AND have an auto-loading configuration file in the data directory.  Doing so will result in an error.

To see an example of how to use the data directory to load a configuration file check out the [Disabling Parameters Guide](docs/DisablingParametersGuide.md).

## Example environment variable

Environment variables are also available to configure indexer. Environment variables override settings in the config file and are overridden by command line arguments.

The same indexer configuration from earlier can be made in bash with the following:
```
~$ export INDEXER_POSTGRES_CONNECTION_STRING="host=mydb.mycloud.com user=postgres password=password dbname=mainnet"
~$ export INDEXER_PIDFILE="/var/lib/algorand/algorand-indexer.pid"
~$ export INDEXER_DATA="/tmp"
~$ ./algorand-indexer daemon
```


## Configuration file
Default values are placed in the configuration file. They can be overridden with environment variables and command line arguments.

The configuration file must named **indexer.yml** and placed in the data directory (see above). The filepath may be set on the CLI using `--configfile` or `-c` but this functionality is deprecated.

Here is an example **indexer.yml** file:
```
postgres-connection-string: "host=mydb.mycloud.com user=postgres password=password dbname=mainnet"
pidfile: "/var/lib/algorand/algorand-indexer.pid"
```

Place this file in the data directory (`/tmp/data-dir` in this example) and supply it to the Indexer daemon:
```
~$ ./algorand-indexer daemon --data-dir /tmp/data-dir
```


# Systemd

`/lib/systemd/system/algorand-indexer.service` can be partially overridden by creating `/etc/systemd/system/algorand-indexer.service.d/local.conf`. The most common things to override will be the command line and pidfile. The overriding local.conf file might be this:

```
[Service]
ExecStart=
ExecStart=/usr/bin/algorand-indexer daemon --data-dir /tmp --pidfile /var/lib/algorand/algorand-indexer.pid --postgres "host=mydb.mycloud.com user=postgres password=password dbname=mainnet"
PIDFile=/var/lib/algorand/algorand-indexer.pid

```

The systemd unit file can be found in source at [misc/systemd/algorand-indexer.service](misc/systemd/algorand-indexer.service)

Note that the service assumes an `algorand` group and user. If the [Algorand package](https://github.com/algorand/go-algorand/) has already been installed on the same machine, this group and user has already been created.  However, if the Indexer is running stand-alone, the group and user will need to be created before starting the daemon:

```
adduser --system --group --home /var/lib/algorand --no-create-home algorand
```

Once configured, turn on your daemon with:

```bash
sudo systemctl enable algorand-indexer
sudo systemctl start algorand-indexer
```

If you wish to run multiple indexers on one server under systemd, see the comments in `/lib/systemd/system/algorand-indexer@.service` or [misc/systemd/algorand-indexer@.service](misc/systemd/algorand-indexer@.service)

# Unique Database Configurations

## Load balancing
If Conduit is deployed with a clustered database using multiple readers behind a load balancer, query discrepancies are possible due to database replication lag. Users should check the `current-round` response field and be prepared to retry queries when stale data is detected.

## Custom indices
Different application workloads will require different custom indices in order to make queries perform well. More information is available in [PostgresqlIndexes.md](docs/PostgresqlIndexes.md).

## Transaction results order

The order transactions are returned in depends on whether or not an account address filter is used.

When searching by an account, results are returned most recent first. The intent being that a wallet application would want to display the most recent transactions. A special index is used to make this case performant.

For all other transaction queries, results are returned oldest first. This is because it is the physical order they would normally be written in, so it is going to be faster.

<!-- USAGE_END_MARKER_LINE -->
