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

# The cleanup hook ensures these containers are removed when the script exits.
CONTAINERS=(test-container)

NET=localhost:8980
CURL_TEMPFILE=curl_out.txt
PIDFILE=testindexerpidfile

function print_alert() {
  printf "\n=====\n===== $1\n=====\n"
}

##################
## Test Helpers ##
##################

function fail_and_exit {
  print_alert "Failed test - $1 ($2): $3"
  exit 1
}

function base_call() {
  curl -o "$CURL_TEMPFILE" -w "%{http_code}" -q -s "$NET$1"
}

function wait_for_ready() {
  local n=0

  set +e
  until [ "$n" -ge 20 ]
  do
    curl -s -q "$NET/health" | grep '"migration-status":"Migrations Complete"' > /dev/null 2>&1 && break
    n=$((n+1))
    sleep 1
  done
  set -e

  curl -s -q "$NET/health" | grep '"migration-status":"Migrations Complete"' > /dev/null 2>&1
}

# $1 - test description.
# $2 - query
# $3 - expected status code
# $4 - substring that should be in the response
function call_and_verify {
  local CODE

  set +e
  CODE=$(base_call "$2")
  if [[ $? != 0 ]]; then
    echo "ERROR"
    cat $CURL_TEMPFILE
    return
    fail_and_exit "$1" "$2" "curl had a non-zero exit code."
  fi
  set -e

  RES=$(cat "$CURL_TEMPFILE")
  if [[ "$CODE" != "$3" ]]; then
    fail_and_exit "$1" "$2" "unexpected HTTP status code expected $3 (actual $CODE)"
  fi
  if [[ "$RES" != *"$4"* ]]; then
    fail_and_exit "$1" "$2" "unexpected response. should contain '$4', actual: $RES"
  fi

  print_alert "Passed test: $1"
}

#####################
## Indexer Helpers ##
#####################

# $1 - postgres dbname
function start_indexer() {
  ALGORAND_DATA= ../../cmd/algorand-indexer/algorand-indexer daemon -P "host=localhost user=algorand password=algorand dbname=$1 port=5434 sslmode=disable" --pidfile $PIDFILE > /dev/null 2>&1 &
}

function kill_indexer() {
  if test -f "$PIDFILE"; then
    kill -9 $(cat "$PIDFILE") > /dev/null 2>&1 || true
    rm $PIDFILE
  fi
}

####################
## Docker helpers ##
####################

# $1 - name of docker container to kill.
function kill_container() {
  print_alert "Killing container - $1"
  docker rm -f $1 > /dev/null 2>&1 || true
}

# $1 - docker container name.
# $2 - postgres database name.
# $3 - pg_dump file to import into the database.
function setup_postgres() {
  if [ $# -ne 3 ]; then
    print_alert "Unexpected number of arguments to setup_postgres."
    exit 1
  fi

  local CONTAINER_NAME=$1
  local DATABASE=$2
  local DUMPFILE=$3

  # Cleanup from last time
  kill_container $CONTAINER_NAME

  print_alert "Starting - $CONTAINER_NAME ($DATABASE)"
  # Start postgres container...
  docker run \
    -d \
    --name $CONTAINER_NAME \
    -e POSTGRES_USER=algorand \
    -e POSTGRES_PASSWORD=algorand \
    -e PGPASSWORD=algorand \
    -p 5434:5432 \
    postgres

  sleep 5

  print_alert "Started - $CONTAINER_NAME ($DATABASE) + $DUMPFILE"

  # Create DB and load some data into it.
  docker exec -it $CONTAINER_NAME psql -Ualgorand -c "create database $DATABASE"
  #docker exec -i $CONTAINER_NAME psql -Ualgorand -c "\\l"
  docker exec -i $CONTAINER_NAME psql -Ualgorand -d $DATABASE < $DUMPFILE > /dev/null 2>&1
}

function cleanup() {
  for i in ${CONTAINERS[*]}; do
    kill_container $i
  done
  rm $CURL_TEMPFILE > /dev/null 2>&1 || true
  kill_indexer
}

trap cleanup EXIT

###############
## RUN TESTS ##
###############

# Test 1
kill_indexer
setup_postgres ${CONTAINERS[0]} test1 cumulative_rewards_dump.txt
# Sleeping here is useful for troubleshooting these tests with a debugger.
# sleep infinity
start_indexer test1
wait_for_ready
call_and_verify 'Ensure migration updated specific account rewards.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI' 200 '"rewards":80000539878'
call_and_verify 'Ensure migration updated specific account rewards @ round = 810.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=810' 200 '"rewards":80000539878'
call_and_verify 'Ensure migration updated specific account rewards @ round = 800.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=800' 200 '"rewards":68000335902'
call_and_verify 'Ensure migration updated specific account rewards @ round = 500.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=500' 200 '"rewards":28000055972'
call_and_verify 'Ensure migration updated specific account rewards @ round = 100.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=100' 200 '"rewards":7999999996'
