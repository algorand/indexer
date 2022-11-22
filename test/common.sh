#!/usr/bin/env bash

# The cleanup hook ensures these containers are removed when the script exits.
POSTGRES_CONTAINER=test-container
export INDEXER_DATA=/tmp/e2e_test/
TEST_DATA_DIR=$(mktemp)

NET=localhost:8981
CURL_TEMPFILE=curl_out.txt
PIDFILE=testindexerpidfile
CONNECTION_STRING="host=localhost user=algorand password=algorand dbname=DB_NAME_HERE port=5434 sslmode=disable"
MAX_TIME=20
# Set to to prevent cleanup so you can look at the DB or run queries.
HALT_ON_FAILURE=


###################
## Print Helpers ##
###################
function print_alert() {
  printf "\n=====\n===== $1\n=====\n"
}

function print_health() {
    curl -q -s "$NET/health?pretty"
}

##################
## Test Helpers ##
##################
function sleep_forever {
  # sleep infinity doesn't work on mac...
  sleep 1000000000000000
}

function fail_and_exit {
  print_alert "Failed test - $1 ($2): $3"
  echo ""
  print_health
  if [ ! -z $HALT_ON_FAILURE ]; then
    sleep_forever
  fi
  exit 1
}

# $1 - database
# $2 - query
function base_query() {
  #export PGPASSWORD=algorand
  #psql -XA -h localhost -p 5434 -h localhost -U algorand $1 -c "$2"
  docker exec $POSTGRES_CONTAINER psql -XA -Ualgorand $1 -c "$2"
}

# SQL Test - query and veryify results
# $1 - max runtime in seconds, default value = 20
# $2 - test description.
# $3 - database
# $4 - query
# $5 - substring that should be in the response
function sql_test_timeout {
  local MAX_TIME_BEFORE=MAX_TIME
  MAX_TIME=$1
  shift
  sql_test "$@"
  MAX_TIME=$MAX_TIME_BEFORE
}

# SQL Test - query and veryify results
# $1 - test description.
# $2 - database
# $3 - query
# $4... - substring(s) that should be in the response
function sql_test {
  local DESCRIPTION=$1
  shift
  local DATABASE=$1
  shift
  local QUERY=$1
  shift
  local SUBSTRING

  local START=$SECONDS

  set +e
  RES=$(base_query $DATABASE "$QUERY")
  if [[ $? != 0 ]]; then
    echo "ERROR from psql: $RESULT"
    fail_and_exit "$DESCRIPTION" "$QUERY" "psql had a non-zero exit code."
  fi
  set -e

  # Check results
  for SUBSTRING in "$@"; do
    if [[ "$RES" != *"$SUBSTRING"* ]]; then
      fail_and_exit "$DESCRIPTION" "$QUERY" "unexpected response. should contain '$SUBSTRING', actual: '$RES'"
    fi
  done

  local ELAPSED=$(($SECONDS - $START))
  if [[ $ELAPSED -gt $MAX_TIME ]]; then
    fail_and_exit "$DESCRIPTION" "$QUERY" "query duration too long, $ELAPSED > $MAX_TIME"
  fi

  print_alert "Passed test: $DESCRIPTION"
}

# rest_test helper
function base_curl() {
  curl -o "$CURL_TEMPFILE" -w "%{http_code}" -q -s "$NET$1"
}

# CURL Test - query and veryify results
# $1 - max runtime in seconds, default value = 20
# $2 - test description.
# $3 - query
# $4 - match result
# $5 - expected status code
# $6... - substring that should be in the response
function rest_test_timeout {
  local MAX_TIME_BEFORE=MAX_TIME
  MAX_TIME=$1
  shift
  rest_test "$@"
  MAX_TIME=$MAX_TIME_BEFORE
}

# CURL Test - query and veryify results
# $1 - test description.
# $2 - query
# $3 - expected status code
# $4 - match result
# $5... - substring(s) that should be in the response
function rest_test {
  local DESCRIPTION=$1
  shift
  local QUERY=$1
  shift
  local EXPECTED_CODE=$1
  shift
  local MATCH_RESULT=$1
  shift
  local SUBSTRING

  local START=$SECONDS

  set +e
  local CODE=$(base_curl "$QUERY")
  if [[ $? != 0 ]]; then
    cat $CURL_TEMPFILE
    fail_and_exit "$DESCRIPTION" "$QUERY" "curl had a non-zero exit code."
  fi
  set -e

  local RES=$(cat "$CURL_TEMPFILE")
  if [[ "$CODE" != "$EXPECTED_CODE" ]]; then
    fail_and_exit "$DESCRIPTION" "$QUERY" "unexpected HTTP status code expected $EXPECTED_CODE (actual $CODE): $RES"
  fi

  local ELAPSED=$(($SECONDS - $START))
  if [[ $ELAPSED -gt $MAX_TIME ]]; then
    fail_and_exit "$DESCRIPTION" "$QUERY" "query duration too long, $ELAPSED > $MAX_TIME"
  fi

  # Check result substrings
  for SUBSTRING in "$@"; do
    if [[ $MATCH_RESULT = true ]]; then
      if [[ "$RES" != *"$SUBSTRING"* ]]; then
        fail_and_exit "$DESCRIPTION" "$QUERY" "unexpected response. should contain '$SUBSTRING', actual: $RES"
      fi
    else
      if [[ "$RES" == *"$SUBSTRING"* ]]; then
        fail_and_exit "$DESCRIPTION" "$QUERY" "unexpected response. should NOT contain '$SUBSTRING', actual: $RES"
      fi
    fi
  done

  print_alert "Passed test: $DESCRIPTION"
}

#####################
## Indexer Helpers ##
#####################

# Suppresses output if the command succeeds
# $1 command to run
function suppress() {
  /bin/rm --force /tmp/suppress.out 2> /dev/null
  ${1+"$@"} > /tmp/suppress.out 2>&1 || cat /tmp/suppress.out
  /bin/rm /tmp/suppress.out
}

# $1 - connection string
# $2 - if set, puts in read-only mode
function start_indexer_with_connection_string() {
  if [ ! -z $2 ]; then
    # strictly read-only
    RO="--no-algod"
  else
    # we may start up from canned data, but need to update for the current running binary.
    RO="--allow-migration"
  fi
  mkdir -p $INDEXER_DATA
  ALGORAND_DATA= ../cmd/algorand-indexer/algorand-indexer daemon \
    -S $NET "$RO" \
    -P "$1" \
    -i "$TEST_DATA_DIR" \
    --enable-all-parameters \
    "$RO" \
    --pidfile $PIDFILE 2>&1 > /dev/null &
}

# $1 - postgres dbname
# $2 - if set, halts execution
function start_indexer() {
  if [ ! -z $2 ]; then
    echo "daemon -i "$TEST_DATA_DIR" -S $NET -P \"${CONNECTION_STRING/DB_NAME_HERE/$1}\""
    sleep_forever
  fi

  start_indexer_with_connection_string "${CONNECTION_STRING/DB_NAME_HERE/$1}"
}


# $1 - postgres dbname
# $2 - e2edata tar.bz2 archive
# $3 - if set, halts execution
function start_indexer_with_blocks() {
  if [ ! -f $2 ]; then
    echo "Cannot find $2"
    exit
  fi

  create_db $1

  local TEMPDIR=$(mktemp -d -t ci-XXXXXXX)
  tar -xf "$2" -C $TEMPDIR

  if [ ! -z $3 ]; then
    echo "Start args 'import -P \"${CONNECTION_STRING/DB_NAME_HERE/$1}\" --genesis \"$TEMPDIR/algod/genesis.json\" $TEMPDIR/blocktars/*'"
    sleep_forever
  fi
  ALGORAND_DATA= ../cmd/algorand-indexer/algorand-indexer import \
    -P "${CONNECTION_STRING/DB_NAME_HERE/$1}" \
    --genesis "$TEMPDIR/algod/genesis.json" \
    $TEMPDIR/blocktars/*

  rm -rf $TEMPDIR

  start_indexer $1 $3
}

# $1 - number of attempts
function wait_for_started() {
  wait_for '"round":' "$1"
}

# $1 - number of attempts
function wait_for_migrated() {
  wait_for '"migration-required":false' "$1"
}

# $1 - number of attempts
function wait_for_available() {
  wait_for '"db-available":true' "$1"
}

# Query indexer for 20 seconds waiting for migration to complete.
# Exit with error if still not ready.
# $1 - string to look for
# $2 - number of attempts (optional, default = 20)
function wait_for() {
  local n=0

  set +e
  local READY
  until [ "$n" -ge ${2:-20} ] || [ ! -z $READY ]; do
    curl -q -s "$NET/health" | grep "$1" > /dev/null 2>&1 && READY=1
    n=$((n+1))
    sleep 1
  done
  set -e

  if [ -z $READY ]; then
    echo "Error: timed out waiting for $1."
    print_health
    exit 1
  fi
}

# Kill indexer using the PIDFILE
function kill_indexer() {
  if test -f "$PIDFILE"; then
    kill -9 $(cat "$PIDFILE") > /dev/null 2>&1 || true
    rm $PIDFILE
    rm -rf $INDEXER_DATA
    rm -rf $TEST_DATA_DIR/*
    pwd
    ls -l
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

function start_postgres() {
  if [ $# -ne 0 ]; then
    print_alert "Unexpected number of arguments to start_postgres."
    exit 1
  fi

  # Cleanup from last time
  kill_container $POSTGRES_CONTAINER

  print_alert "Starting - $POSTGRES_CONTAINER"
  # Start postgres container...
  docker run \
    -d \
    --name $POSTGRES_CONTAINER \
    -e POSTGRES_USER=algorand \
    -e POSTGRES_PASSWORD=algorand \
    -e PGPASSWORD=algorand \
    -p 5434:5432 \
    postgres

  sleep 5

  print_alert "Started - $POSTGRES_CONTAINER"
}

# $1 - postgres database name.
function create_db() {
  local DATABASE=$1

  # Create DB
  docker exec -it $POSTGRES_CONTAINER psql -Ualgorand -c "create database $DATABASE"
}

# $1 - postgres database name.
# $2 - pg_dump file to import into the database.
function initialize_db() {
  local DATABASE=$1
  local DUMPFILE=$2
  print_alert "Initializing database ($DATABASE) with $DUMPFILE"

  # load some data into it.
  create_db $DATABASE
  #docker exec -i $POSTGRES_CONTAINER psql -Ualgorand -c "\\l"
  docker exec -i $POSTGRES_CONTAINER psql -Ualgorand -d $DATABASE < $DUMPFILE > /dev/null 2>&1
}

function cleanup() {
  kill_container $POSTGRES_CONTAINER
  rm $CURL_TEMPFILE > /dev/null 2>&1 || true
  kill_indexer
}

#####################
## User Interaction #
#####################

# Interactive yes/no prompt
function ask () {
    # https://djm.me/ask
    local prompt default reply

    if [ "${2:-}" = "Y" ]; then
        prompt="Y/n"
        default=Y
    elif [ "${2:-}" = "N" ]; then
        prompt="y/N"
        default=N
    else
        prompt="y/n"
        default=
    fi

    while true; do

        # Ask the question (not using "read -p" as it uses stderr not stdout)
        echo -n "$1 [$prompt] "

        # Read the answer (use /dev/tty in case stdin is redirected from somewhere else)
        read reply </dev/tty

        # Default?
        if [ -z "$reply" ]; then
            reply=$default
        fi

        # Check if the reply is valid
        case "$reply" in
            Y*|y*) return 0 ;;
            N*|n*) return 1 ;;
        esac

    done
}

############################################################################
## Integration tests are sometimes useful to run after a migration as well #
############################################################################
function cumulative_rewards_tests() {
    rest_test 'Ensure migration updated specific account rewards.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI' 200 true '"rewards":80000539878'
    # Rewards / Rewind is now disabled
    #rest_test 'Ensure migration updated specific account rewards @ round = 810.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=810' 200 '"rewards":80000539878'
    #rest_test 'Ensure migration updated specific account rewards @ round = 800.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=800' 200 '"rewards":68000335902'
    #rest_test 'Ensure migration updated specific account rewards @ round = 500.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=500' 200 '"rewards":28000055972'
    #rest_test 'Ensure migration updated specific account rewards @ round = 100.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=100' 200 '"rewards":7999999996'

    # One disabled test...
    rest_test 'Ensure migration updated specific account rewards @ round = 810.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=810' 200 true '"rewards":0'
}

# $1 - the DB to query
function create_delete_tests() {
    #####################
    # Application Tests #
    #####################
    sql_test "[sql] app create (app-id=203)" $1 \
      "select deleted, created_at, closed_at, index from app WHERE index = 203" \
      "f|55||203"
    rest_test "[rest] app create (app-id=203)" \
      "/v2/applications/203?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"created-at-round": 55'

    sql_test "[sql] app create & delete (app-id=82)" $1 \
      "select deleted, created_at, closed_at, index from app WHERE index = 82" \
      "t|13|37|82"
    rest_test "[rest] app create & delete (app-id=82)" \
      "/v2/applications/82?pretty" \
      404 \
      true \
      ''
    rest_test "[rest] app create & delete (app-id=82)" \
      "/v2/applications/82?pretty&include-all=true" \
      200 \
      true \
      '"deleted": true' \
      '"created-at-round": 13' \
      '"deleted-at-round": 37'

    rest_test "[rest - account/application] account with a deleted application excluded" \
      "/v2/accounts/XNMIHFHAZ2GE3XUKISNMOYKNFDOJXBJMVHRSXVVVIK3LNMT22ET2TA4N4I?pretty" \
      200 \
      false \
      '"id": 82'
    rest_test "[rest - account/application] account with a deleted application included" \
      "/v2/accounts/XNMIHFHAZ2GE3XUKISNMOYKNFDOJXBJMVHRSXVVVIK3LNMT22ET2TA4N4I?pretty&include-all=true" \
      200 \
      true \
      '"id": 82'

    rest_test "[rest - account/application] account with application" \
      "/v2/accounts/D2BFTG5GO2PUCLY2O4XIVW7WAQHON4DLX5R5V4O3MZWSWDKBNYZJYKHVBQ?pretty" \
      200 \
      true \
      '"apps-total-schema": {
      "num-byte-slice": 1,
      "num-uint": 1
    }'

    ###############
    # Asset Tests #
    ###############
    sql_test "[sql] asset create / destroy" $1 \
      "select deleted, created_at, closed_at, index from asset WHERE index=135" \
      "t|23|33|135"
    rest_test "[rest - asset]  asset create / destroy" \
      "/v2/assets/135?pretty" \
      404 \
      true \
      ''
    rest_test "[rest - asset]  asset create / destroy" \
      "/v2/assets/135?pretty&include-all=true" \
      200 \
      true \
      '"deleted": true' \
      '"created-at-round": 23' \
      '"destroyed-at-round": 33' \
      '"total": 0'
    rest_test "[rest - account]  asset create / destroy" \
      "/v2/accounts/MRPIAVGS2OCS6UO6KC6YZ3445Q2DCMDMRG6OVKZVEYIHLE6BINDCIJ6J7U?pretty" \
      200 \
      false \
      '"asset-id": 135'
    rest_test "[rest - account]  asset create / destroy" \
      "/v2/accounts/MRPIAVGS2OCS6UO6KC6YZ3445Q2DCMDMRG6OVKZVEYIHLE6BINDCIJ6J7U?pretty&include-all=true" \
      200 \
      true \
      '{
        "amount": 0,
        "asset-id": 135,
        "deleted": true,
        "is-frozen": false,
        "opted-in-at-round": 25,
        "opted-out-at-round": 31
      }'

    sql_test "[sql] asset create" $1 \
      "select deleted, created_at, closed_at, index from asset WHERE index=168" \
      "f|35||168"
    rest_test "[rest - asset] asset create" \
      "/v2/assets/168?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"created-at-round": 35' \
      '"total": 1337'
    rest_test "[rest - account] asset create" \
      "/v2/accounts/D2BFTG5GO2PUCLY2O4XIVW7WAQHON4DLX5R5V4O3MZWSWDKBNYZJYKHVBQ?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"created-at-round": 35' \
      '"total": 1337'

    rest_test "[rest - account asset holding] asset holding optin optout" \
      "/v2/accounts/MRPIAVGS2OCS6UO6KC6YZ3445Q2DCMDMRG6OVKZVEYIHLE6BINDCIJ6J7U?pretty" \
      200 \
      false \
      '"asset-id": 135'
    rest_test "[rest - account asset holding] asset holding optin optout" \
      "/v2/accounts/MRPIAVGS2OCS6UO6KC6YZ3445Q2DCMDMRG6OVKZVEYIHLE6BINDCIJ6J7U?pretty&include-all=true" \
      200 \
      true \
      '"asset-id": 135'

    ###########################
    # Application Local Tests #
    ###########################
    sql_test "[sql] app optin no closeout" $1 \
      "select deleted, created_at, closed_at, app from account_app WHERE addr=decode('rAMD0F85toNMRuxVEqtxTODehNMcEebqq49p/BZ9rRs=', 'base64') AND app=85" \
      "f|13||85"
    rest_test "[rest] app optin no closeout" \
      "/v2/accounts/VQBQHUC7HG3IGTCG5RKRFK3RJTQN5BGTDQI6N2VLR5U7YFT5VUNVAF57ZU?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"opted-in-at-round": 13' \
      '"deleted": false' \
      '"key": "Y1g="'

    sql_test "[sql] app multiple optins first saved (it is also closed)" $1 \
      "select deleted, created_at, closed_at, app from account_app WHERE addr=decode('Eze95btTASDFD/t5BDfgA2qvkSZtICa5pq1VSOUU0Y0=', 'base64') AND app=82" \
      "t|15|35|82"
    rest_test "[rest] app multiple optins first saved (it is also closed)" \
      "/v2/accounts/CM333ZN3KMASBRIP7N4QIN7AANVK7EJGNUQCNONGVVKURZIU2GG7XJIZ4Q?pretty" \
      200 \
      false \
      '"deleted": true' \
      '"opted-in-at-round": 15' \
      '"closed-out-at-round": 35'
    rest_test "[rest] app multiple optins first saved (it is also closed)" \
      "/v2/accounts/CM333ZN3KMASBRIP7N4QIN7AANVK7EJGNUQCNONGVVKURZIU2GG7XJIZ4Q?pretty&include-all=true" \
      200 \
      true \
      '"deleted": true' \
      '"opted-in-at-round": 15' \
      '"closed-out-at-round": 35'

    sql_test "[sql] app optin/optout/optin should leave last closed_at" $1 \
      "select deleted, created_at, closed_at, app from account_app WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND app=203" \
      "f|57|59|203"
    rest_test "[rest] app optin/optout/optin should leave last closed_at" \
      "/v2/accounts/MRPIAVGS2OCS6UO6KC6YZ3445Q2DCMDMRG6OVKZVEYIHLE6BINDCIJ6J7U?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"opted-in-at-round": 57' \
      '"closed-out-at-round": 59' \
      '"num-byte-slice": 1'

    #######################
    # Asset Holding Tests #
    #######################
    sql_test "[sql] asset optin" $1 \
      "select deleted, created_at, closed_at, assetid from account_asset WHERE addr=decode('MFkWBNGTXkuqhxtNVtRZYFN6jHUWeQQxqEn5cUp1DGs=', 'base64') AND assetid=27" \
      "f|13||27"
    rest_test "[rest - balances] asset optin" \
      "/v2/assets/27/balances?pretty&currency-less-than=100" \
      200 \
      true \
      '"deleted": false' \
      '"opted-in-at-round": 13'
    rest_test "[rest - account] asset optin" \
      "/v2/accounts/GBMRMBGRSNPEXKUHDNGVNVCZMBJXVDDVCZ4QIMNIJH4XCSTVBRVYWWVCZA?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"opted-in-at-round": 13'

    sql_test "[sql] asset optin / close-out" $1 \
      "select deleted, created_at, closed_at, assetid from account_asset WHERE addr=decode('E/p3R9m9X0c7eAv9DapnDcuNGC47kU0BxIVdSgHaFbk=', 'base64') AND assetid=36" \
      "t|16|25|36"
    rest_test "[rest] asset optin" \
      "/v2/assets/36/balances?pretty&currency-less-than=100" \
      200 \
      true \
      '"balances": []'
    rest_test "[rest] asset optin" \
      "/v2/assets/36/balances?pretty&currency-less-than=100&include-all=true" \
      200 \
      true \
      '"deleted": true' \
      '"opted-in-at-round": 16' \
      '"opted-out-at-round": 25'

    sql_test "[sql] asset optin / close-out / optin / close-out" $1 \
      "select deleted, created_at, closed_at, assetid from account_asset WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND assetid=135" \
      "t|25|31|135"
    rest_test "[rest] asset optin" \
      "/v2/assets/135/balances?pretty&currency-less-than=100" \
      200 \
      false \
      '"address": "MRPIAVGS2OCS6UO6KC6YZ3445Q2DCMDMRG6OVKZVEYIHLE6BINDCIJ6J7U"' \
      '"opted-in-at-round": 25' \
      '"opted-out-at-round": 31'
    rest_test "[rest] asset optin" \
      "/v2/assets/135/balances?pretty&currency-less-than=100&include-all=true" \
      200 \
      true \
      false \
      '"address": "MRPIAVGS2OCS6UO6KC6YZ3445Q2DCMDMRG6OVKZVEYIHLE6BINDCIJ6J7U"' \
      '"deleted": true' \
      '"opted-in-at-round": 25' \
      '"opted-out-at-round": 31'

    sql_test "[sql] asset optin / close-out / optin" $1 \
      "select deleted, created_at, closed_at, assetid from account_asset WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND assetid=168" \
      "f|37|39|168"
    rest_test "[rest] asset optin" \
      "/v2/assets/168/balances?pretty&currency-less-than=100" \
      200 \
      true \
      '"deleted": false' \
      '"opted-in-at-round": 37' \
      '"opted-out-at-round": 39'

    #################
    # Account Tests #
    #################
    sql_test "[sql] genesis account with no transactions" $1 \
      "select deleted, created_at, closed_at, microalgos from account WHERE addr = decode('4L294Wuqgwe0YXi236FDVI5RX3ayj4QL1QIloIyerC4=', 'base64')" \
      "f|0||5000000000000000"
    rest_test "[rest] genesis account with no transactions" \
      "/v2/accounts/4C633YLLVKBQPNDBPC3N7IKDKSHFCX3WWKHYIC6VAIS2BDE6VQXACGG3BQ?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"created-at-round": 0'

    sql_test "[sql] account created then never closed" $1 \
      "select deleted, created_at, closed_at, microalgos from account WHERE addr = decode('HoJZm6Z2n0EvGncuitv2BA7m8Gu/Y9rx22ZtKw1BbjI=', 'base64')" \
      "f|4||999999885998"
    rest_test "[rest] account created then never closed" \
      "/v2/accounts/D2BFTG5GO2PUCLY2O4XIVW7WAQHON4DLX5R5V4O3MZWSWDKBNYZJYKHVBQ?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"created-at-round": 4'

    sql_test "[sql] account create close create" $1 \
      "select deleted, created_at, closed_at, microalgos from account WHERE addr = decode('KbUa0wk9gB3BgAjQF0J9NqunWaFS+h4cdZdYgGfBes0=', 'base64')" \
      "f|17|19|100000"
    rest_test "[rest] account create close create" \
      "/v2/accounts/FG2RVUYJHWAB3QMABDIBOQT5G2V2OWNBKL5B4HDVS5MIAZ6BPLGR65YW3Y?pretty" \
      200 \
      true \
      '"deleted": false' \
      '"created-at-round": 17' \
      '"closed-at-round": 19'

    sql_test "[sql] account create close create close" $1 \
      "select deleted, created_at, closed_at, microalgos from account WHERE addr = decode('8rpfPsaRRIyMVAnrhHF+SHpq9za99C1NknhTLGm5Xkw=', 'base64')" \
      "t|9|15|0"
    rest_test "[rest] account create close create close" \
      "/v2/accounts/6K5F6PWGSFCIZDCUBHVYI4L6JB5GV5ZWXX2C2TMSPBJSY2NZLZGCF2NH5U?pretty" \
      404 \
      true \
      ''
    rest_test "[rest] account create close create close" \
      "/v2/accounts/6K5F6PWGSFCIZDCUBHVYI4L6JB5GV5ZWXX2C2TMSPBJSY2NZLZGCF2NH5U?pretty&include-all=true" \
      200 \
      true \
      '"deleted": true' \
      '"created-at-round": 9' \
      '"closed-at-round": 15'

      rest_test "[rest] b64 transaction fields are serialized" \
      "/v2/transactions/TV5RPJFA6YT2APADUOYKIEL3NFAXSVB5J4JO6TSG7BHK4Z5OJKSA?pretty" \
      200 \
      true \
      '"decimals": 0' \
      '"default-frozen": false' \
      '"name": "bogocoin"' \
      '"name-b64": "Ym9nb2NvaW4="' \
      '"reserve": "EQJSQTOITX64CPGA5VRKBKLEJR57YXVNLTW5DKZMESIPTEOLRDNWQIJCGU"' \
      '"total": 1000000000000' \
      '"unit-name": "bogo"' \
      '"unit-name-b64": "Ym9nbw=="' \
      '"created-asset-index": 36'

      rest_test "[rest] b64 asset fields are serialized" \
      "/v2/assets/36?pretty" \
      200 \
      true \
      '"decimals": 0' \
      '"default-frozen": false' \
      '"name": "bogocoin"' \
      '"name-b64": "Ym9nb2NvaW4="' \
      '"reserve": "EQJSQTOITX64CPGA5VRKBKLEJR57YXVNLTW5DKZMESIPTEOLRDNWQIJCGU"' \
      '"total": 1000000000000' \
      '"unit-name": "bogo"' \
      '"unit-name-b64": "Ym9nbw=="'

      rest_test "[rest] b64 account fields are serialized" \
      "/v2/accounts/EQJSQTOITX64CPGA5VRKBKLEJR57YXVNLTW5DKZMESIPTEOLRDNWQIJCGU?pretty" \
      200 \
      true \
      '"decimals": 0' \
      '"default-frozen": false' \
      '"name": "bogocoin"' \
      '"name-b64": "Ym9nb2NvaW4="' \
      '"reserve": "EQJSQTOITX64CPGA5VRKBKLEJR57YXVNLTW5DKZMESIPTEOLRDNWQIJCGU"' \
      '"total": 1000000000000' \
      '"unit-name": "bogo"' \
      '"unit-name-b64": "Ym9nbw=="'
}
