# Algorand Indexer

The Indexer reads committed blocks from the Algorand blockchain and mantains a database of transactions and accounts that are searchable and indexed on several features.

## Basic Operation

The most common usage of the Indexer is expect to be getting validated blocks from a local `algod` Algorand node, adding them to a [PostgreSQL](https://www.postgresql.org/) database, and serving an API to make available a variety of prepared queries. Some users may wish to directly write SQL queries of the database.

Once you have setup a PostgreSQL database for this purpose, and supposing that a local Algorand node's data directory is at `/var/lib/algorand`, the Indexer can be started like this:

```bash
algorand-indexer daemon --postgres "user=postgres password=YourPasswordHere dbname=foo {other connection options for your database}" --algod /var/lib/algorand
```

* **The Algorand Node should be an archival/relay node** that keeps a copy of all the blocks in the entire history of the block chain. This way the indexer can get any block and create an index of the entire history of the block chain.
* As of 2020 April, storing all the raw blocks is about 100 GB and the PostgreSQL database of transactions and accounts is about 1 GB. (Much of that size difference is the Indexer ignoring cryptographic signature data; relying on `algod` to validate blocks. Dropping that, the Indexer can focus on the 'what happened' details of transactions and accounts.)
* If you don't wish to run an archival Algorand node forever storing all the blocks, it's possible to start a new Algorand node, start the Indexer in the same moment, and they should procede to catch-up together through all the historical comitted blocks up through the current latest blocks.

## Read-only Indexer Server

It is possible to set up one Postgres database with one writer and many readers. The Indexer pulling new data from algod can be started as above. Starting the indexer daemon without $ALGORAND_DATA or -d/--algod/--algod-net/--algod-token will start it without writing new data to the database. For further isolation, create a `readonly` postgres user. Indexer does specifically note the username "readonly" and change behavior to not try to write to the database. The primary benefit is that Postgres can enforce restricted access to this user:

```sql
CREATE USER readonly LOGIN PASSWORD 'YourPasswordHere';
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO readonly;
```

Then start the Indexer:

```bash
algorand-indexerindexer daemon --postgres "user=readonly password=YourPasswordHere {other connection options for your database}" --no-algod
```

## Systemd

`/lib/systemd/system/algorand-indexer.service` can be partially overridden by creating `/etc/systemd/system/algorand-indexer.service.d/local.conf`. The most common things to override will be the command line and pidfile. The overriding local.conf file might be this:

```
[Service]
ExecStart=/usr/bin/algorand-indexer daemon --pidfile /var/lib/algorand/algorand-indexer.pid --algod /var/lib/algorand --postgres "host=mydb.mycloud.com user=postgres password=password dbname=mainnet"
PIDFile=/var/lib/algorand/algorand-indexer.pid

```

The systemd unit file can be found in source at [misc/systemd/algorand-indexer.service](misc/systemd/algorand-indexer.service)

Once configured, turn on your daemon with:

```bash
sudo systemctl enable algorand-indexer
sudo systemctl start algorand-indexer
```

If you wish to run multiple indexers on one server under systemd, see the comments in `/lib/systemd/system/algorand-indexer@.service` or [misc/systemd/algorand-indexer@.service](misc/systemd/algorand-indexer@.service)
