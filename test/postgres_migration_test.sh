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

# $1 - the DB to query
function omnibus_migration_tests() {
    sql_test "[sql] key encoding migration" $1 \
      "select txn from txn where round=12 AND intra=4" \
      '{"gd": {"foo": {"at": 1, "bs": "YmFy"}}' 

    # This account was manually modified in the SQL dump to contain a spending
    # account containing sentinal address 'aaaaaaa...'
    sql_test "[sql] param cleanup on close" $1 \
      "select created_at,account_data,closed_at from account where addr=decode('ma6DZC6bVLXRvN2Bjn4QSS0Mk8UXvfVdouaojiNzyqo=','base64')" \
      '7||9' 

    # This account localstate was modified in the SQL dump to contain an empty
    # tkv array value.
    sql_test "[sql] account app cleanup (empty tkv value)" $1 \
      "select created_at,localstate,closed_at from account_app where addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=','base64')" \
      '57|{"hsch": {"nbs": 1, "nui": 1}}|59' 

    # This app localstate was modified in the SQL dump to contain an empty
    # gs array value.
    sql_test "[sql] app cleanup (empty gs value)" $1 \
      "select created_at,params,closed_at from app where index=24" \
      '7|{}|7' 
}

###############
## RUN TESTS ##
###############

# Rewards tests
print_alert "Migration Rewards Tests (test1)"
kill_indexer
initialize_db test1 migrations/cumulative_rewards_dump.txt
start_indexer test1
wait_for_migrated
cumulative_rewards_tests

# Create Delete Tests
print_alert "Create Delete Tests (test2)"
kill_indexer
initialize_db test2 migrations/create_delete.2.2.1.txt
start_indexer test2
wait_for_migrated
create_delete_tests test2
omnibus_migration_tests test2

