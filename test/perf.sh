#!/usr/bin/env bash

# Create a read-only connection to the test DB and run some queries that
# should complete in a reasonable amount of time.

set -e

if [ $# -ne 1 ]; then
  echo "Expect a single connection string argument."
  exit 1
fi


# This script only works when CWD is 'test'
rootdir=`dirname $0`
pushd $rootdir > /dev/null

source common.sh
trap cleanup EXIT

start_indexer_with_connection_string "$1" "read-only" > /dev/null

wait_for_started

# Disable the test instead of exiting with an error.
set +e
if ! IGNORED="$(wait_for_migrated '1')"; then
  print_alert "Migration required, perf test disabled."
  exit 0
fi
set -e

print_alert "Running performance tests"
call_and_verify "account endpoint" \
  "/v2/accounts" \
  200 \
  "{" \
  5
call_and_verify "transactions endpoint" \
  "/v2/transactions" \
  200 \
  "{" \
  5
call_and_verify "assets endpoint" \
  "/v2/assets" \
  200 \
  "{" \
  5
call_and_verify "applications endpoint" \
  "/v2/applications" \
  200 \
  "{" \
  5
call_and_verify "busy account transactions" \
  "/v2/accounts/5K6J3Z54656IR7YY65WNJT54UW6RBZZYL5LWQUTG4RWOTRTRBE2MR2AODQ/transactions" \
  200 \
  "{" \
  5
