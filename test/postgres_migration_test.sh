#!/usr/bin/env bash

# Tests migrations by hooking up indexer to a preconfigured postres database.
#
# Postgres is run inside a docker container, and initialized with a dump file
# created with pg_dump.
#
# Creating the dumpfile is outside the scope of this script. The SDK test
# environment can be used for this. Make sure that the version of indexer being
# used does not include the new migration. Once started submit transactions
# that will allow the migration to be tested. When finished use "pg_dump" to
# save the postgres state.
#
# Copy the postgres dump file into this directory and write some tests to make
# sure the migration works.

# Add 2nd 'halt' argument to start_indexer function to pause execution for
# running in the debugger.

set -e

# This script only works when CWD is 'test'
rootdir=`dirname $0`
pushd $rootdir > /dev/null
pwd

source common.sh
trap cleanup EXIT

start_postgres

###############
## RUN TESTS ##
###############

# Rewards tests
kill_indexer
initialize_db test1 migrations/cumulative_rewards_dump.txt
start_indexer test1
wait_for_migrated
cumulative_rewards_tests

# Create Delete Tests
kill_indexer
initialize_db test2 migrations/create_delete.2.2.1.txt
start_indexer test2
wait_for_migrated
create_delete_tests test2

