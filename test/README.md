# Integration Test Scripts

These scripts are designed to run tests covering the standard import routines and the migrations using curated data sets. This document describes how they work, how to create new tests, and how to debug problems.

# How it works

These scripts utilize postgres running in a docker container to streamline setting things up.

There are three scripts:
* common.sh - shared functionality between the scripts
* postgres_integration_test.sh - the standard block processing path
* postgres_migration_test.sh - the migration DB processing path

### common.sh - init functions

**postgres:**

* start_postgres - starts the postgres docker container
* create_db - create a DB on the postgres server
* initialize_db - initialize a DB with a pg_dump file

**indexer:**

* start_indexer - starts indexer against a given database
* start_indexer_with_blocks - runs an `import` with a given e2edata archive and then starts indexer
* wait_for_ready - waits for indexer to report is-migrating = false

**cleanup:**
cleanup - throw a `trap cleanup EXIT` into your script to make sure postgres/indexer are killed on error.
kill_indexer - stops the current indexer process
kill_container - stops the postgres container

### common.sh - test helpers

call_and_verify `<description>` `<command>` `<expected status code>` `<substring>`

* description - informative description displayed when the testcase runs
* command - indexer command, like `/v2/accounts`
* expected status code - HTTP status code
* substring - A simple pattern is used to verify results, find a substring in your expected results to put here

query_and_verify `<description>` `<database>` `<query>` `<substring>`

* description - informative description displayed when the testcase runs
* database - the database to query
* query - indexer command, like `/v2/accounts`
* substring - A simple pattern is used to verify results, find a substring in your expected results to put here. You may need to organize the select values carefully if you want to verify empty columns.

# Debugging

### Integration debugging
This one doesn't have any convenience helpers yet. You'll need to break before the `algorand-indexer import` call and startup your debugger in import mode.

### Migration Debugging
If you pass a second argument to `start_indexer` the command will print the `algorand-indexer` arguments and hang before starting indexer. This makes it pretty easy to run a test in a debugger.

It may be useful to edit one of the entry point scripts to make sure the dataset you're interested in is loaded first.


# Creating a new test

When you have setup for a migration or integration test, use the provided `call_and_verify` / `query_and_verify` functions to write your tests.

### Integration test

This test loads an e2edata block archive + genesis dataset, you create one using the [buildtestdata.sh](../misc/buildtestdata.sh) script. You need to configure it with where to place the resulting archive, and your go-algorand directory. Here is an example script to setup everything:
```bash
#!/usr/bin/env bash

rm -rf ve3
export GOALGORAND="${GOPATH}/src/github.com/algorand/go-algorand"
export E2EDATA="${HOME}/algorand/indexer/e2edata"
export BUILD_BLOCK_ARCHIVE="yes please"
rm -rf "$E2EDATA"
mkdir -p "$E2EDATA"
python3 -m venv ve3
ve3/bin/pip install py-algorand-sdk
. ve3/bin/activate
./misc/buildtestdata.sh
```

The archive will be buried in the `E2EDATA` directory somewhere. That file is passed to start_indexer_with_blocks, then you just need to write the tests.

Because this process depends on the e2e scripts in go-algorand, you'll need to add a test script there manipulates state for your test.

### Migration test

You'll need to create an indexer postgres DB dump for this one. Make sure you're using an older version of indexer to ensure that your migration will run. Once you have the right binary built there are a couple ways to generate the file:

1. Manually. Suppose there is a very specific test that you wish to perform, for example testing rewards accumulating over time. You could start a private network (manually, or with something like sandbox) and then call `pg_dump` on the database.
2. Using an e2edata archive and `create_dump.sh` you can easily generate the text file. Simply build the version of indexer you need and run `create_dump.sh e2edata.tar.bz2 dumpfile.txt`

The resulting text file from either of these methods can be used with `initialize_db` to setup for a migration test.
