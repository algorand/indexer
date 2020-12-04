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

If you pass a second argument to `start_indexer` the command will print the `algorand-indexer` arguments and hang before starting indexer. This makes it pretty easy to run a test in a debugger.

It may be useful to edit one of the entry point scripts to make sure the dataset you're interested in is loaded first.

# Creating a new test
