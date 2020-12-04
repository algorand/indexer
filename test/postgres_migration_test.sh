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

set -e

# This script only works when CWD is 'test/migrations'
rootdir=`dirname $0`
pushd $rootdir > /dev/null
pwd

source common.sh
trap cleanup EXIT

start_postgres

###############
## RUN TESTS ##
###############


kill_indexer
initialize_db test2 migrations/create_delete.2.2.1.txt
start_indexer test2
sleep infinity

# Test 1
kill_indexer
initialize_db test1 migrations/cumulative_rewards_dump.txt
# Sleeping here is useful for troubleshooting these tests with a debugger.
# sleep infinity
start_indexer test1
wait_for_ready
call_and_verify 'Ensure migration updated specific account rewards.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI' 200 '"rewards":80000539878'
call_and_verify 'Ensure migration updated specific account rewards @ round = 810.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=810' 200 '"rewards":80000539878'
call_and_verify 'Ensure migration updated specific account rewards @ round = 800.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=800' 200 '"rewards":68000335902'
call_and_verify 'Ensure migration updated specific account rewards @ round = 500.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=500' 200 '"rewards":28000055972'
call_and_verify 'Ensure migration updated specific account rewards @ round = 100.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=100' 200 '"rewards":7999999996'
