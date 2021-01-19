#!/usr/bin/env bash

# Test block processing by hooking up indexer to preconfigured block datasets.

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
# Test 1
print_alert "Integration Test 1"
kill_indexer
start_indexer_with_blocks createdestroy blockdata/create_destroy.tar.bz2
wait_for_ready
create_delete_tests createdestroy
