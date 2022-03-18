# Integration Test Scripts

** NOTE: These are already out of date. They need to stop using e2edata and instead launch an algod private network based on the private network snapshot.**

These scripts are designed to run tests covering the standard import routines using curated data sets. This document describes how they work, how to create new tests, and how to debug problems.

# How it works

These scripts utilize postgres running in a docker container to streamline setting things up.

There are three scripts:
* common.sh - shared functionality between the scripts
* postgres_integration_test.sh - the standard block processing path

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

rest_test `<description>` `<command>` `<expected status code>` `<substring>`

* description - informative description displayed when the testcase runs
* command - indexer command, like `/v2/accounts`
* expected status code - HTTP status code
* substring - A simple pattern is used to verify results, find a substring in your expected results to put here

sql_test `<description>` `<database>` `<query>` `<substring>`

* description - informative description displayed when the testcase runs
* database - the database to query
* query - indexer command, like `/v2/accounts`
* substring - A simple pattern is used to verify results, find a substring in your expected results to put here. You may need to organize the select values carefully if you want to verify empty columns.

# Debugging

Add true to the end of the `algorand-indexer import` call, use the provided command to initialize indexer.

It may be useful to edit one of the entry point scripts to make sure the dataset you're interested in is loaded first.


# Creating a new test

When you have setup for an integration test, use the provided `rest_test` / `sql_test` functions to write your tests.

This test loads an e2edata block archive + genesis dataset, you create one using the [buildtestdata.sh](../misc/buildtestdata.sh) script. See top-level comments in [buildtestdata.sh](../misc/buildtestdata.sh) for local environment setup instructions.

The archive will be buried in the `E2EDATA` directory somewhere. That file is passed to start_indexer_with_blocks, then you just need to write the tests.

Because this process depends on the e2e scripts in go-algorand, you'll need to add a test script there manipulates state for your test.
