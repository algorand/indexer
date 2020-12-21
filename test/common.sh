#!/usr/bin/env bash

# The cleanup hook ensures these containers are removed when the script exits.
POSTGRES_CONTAINER=test-container

NET=localhost:8981
CURL_TEMPFILE=curl_out.txt
PIDFILE=testindexerpidfile
CONNECTION_STRING="host=localhost user=algorand password=algorand dbname=DB_NAME_HERE port=5434 sslmode=disable"

###################
## Print Helpers ##
###################
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

# $1 - database
# $2 - query
function base_query() {
  #export PGPASSWORD=algorand
  #psql -XA -h localhost -p 5434 -h localhost -U algorand $1 -c "$2"
  docker exec $POSTGRES_CONTAINER psql -XA -Ualgorand $1 -c "$2"
}

# SQL Test - query and veryify results
# $1 - test description.
# $2 - database
# $3 - query
# $4... - substring that should be in the response
function sql_test {
  local DESCRIPTION=$1
  shift
  local DATABASE=$1
  shift
  local QUERY=$1
  shift
  local SUBSTRING

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

  print_alert "Passed test: $DESCRIPTION"
}

# rest_test helper
function base_curl() {
  curl -o "$CURL_TEMPFILE" -w "%{http_code}" -q -s "$NET$1"
}

# CURL Test - query and veryify results
# $1 - test description.
# $2 - query
# $3 - expected status code
# $4... - substring that should be in the response
function rest_test {
  local DESCRIPTION=$1
  shift
  local QUERY=$1
  shift
  local EXPECTED_CODE=$1
  shift
  local SUBSTRING

  set +e
  local CODE=$(base_curl "$QUERY")
  if [[ $? != 0 ]]; then
    echo "ERROR"
    cat $CURL_TEMPFILE
    fail_and_exit "$DESCRIPTION" "$QUERY" "curl had a non-zero exit code."
  fi
  set -e

  RES=$(cat "$CURL_TEMPFILE")
  if [[ "$CODE" != "$EXPECTED_CODE" ]]; then
    fail_and_exit "$DESCRIPTION" "$QUERY" "unexpected HTTP status code expected $EXPECTED_CODE (actual $CODE): $RES"
  fi

  # Check results
  for SUBSTRING in "$@"; do
    if [[ "$RES" != *"$SUBSTRING"* ]]; then
      fail_and_exit "$DESCRIPTION" "$QUERY" "unexpected response. should contain '$SUBSTRING', actual: $RES"
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

# $1 - postgres dbname
# $2 - if set, halts execution
function start_indexer() {
  if [ ! -z $2 ]; then
    echo "daemon -S $NET -P \"${CONNECTION_STRING/DB_NAME_HERE/$1}\""
    sleep infinity
  fi

  ALGORAND_DATA= ../cmd/algorand-indexer/algorand-indexer daemon \
    -S $NET \
    -P "${CONNECTION_STRING/DB_NAME_HERE/$1}" \
    --pidfile $PIDFILE 2>&1 > /dev/null &
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

  echo "Start args 'import -P \"${CONNECTION_STRING/DB_NAME_HERE/$1}\" --genesis \"$TEMPDIR/algod/genesis.json\" $TEMPDIR/blocktars/*"
  #sleep infinity
  ALGORAND_DATA= ../cmd/algorand-indexer/algorand-indexer import \
    -P "${CONNECTION_STRING/DB_NAME_HERE/$1}" \
    --genesis "$TEMPDIR/algod/genesis.json" \
    $TEMPDIR/blocktars/*

  rm -rf $TEMPDIR

  start_indexer $1 $3
}

# Query indexer for 20 seconds waiting for migration to complete.
# Exit with error if still not ready.
function wait_for_ready() {
  local n=0

  set +e
  local READY
  until [ "$n" -ge 20 ] || [ ! -z $READY ]
  do
    curl -q -s "$NET/health" | grep '"is-migrating":false' > /dev/null 2>&1 && READY=1
    n=$((n+1))
    sleep 1
  done
  set -e

  if [ -z $READY ]; then
    echo "Error: timed out waiting for db to become available."
    curl "$NET/health"
    exit 1
  fi
}

# Kill indexer using the PIDFILE
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
    rest_test 'Ensure migration updated specific account rewards.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI' 200 '"rewards":80000539878'
    # Rewards / Rewind is now disabled
    #rest_test 'Ensure migration updated specific account rewards @ round = 810.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=810' 200 '"rewards":80000539878'
    #rest_test 'Ensure migration updated specific account rewards @ round = 800.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=800' 200 '"rewards":68000335902'
    #rest_test 'Ensure migration updated specific account rewards @ round = 500.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=500' 200 '"rewards":28000055972'
    #rest_test 'Ensure migration updated specific account rewards @ round = 100.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=100' 200 '"rewards":7999999996'

    # One disabled test...
    rest_test 'Ensure migration updated specific account rewards @ round = 810.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=810' 200 '"rewards":0'
}

# $1 - the DB to query
function create_delete_tests() {
    #####################
    # Application Tests #
    #####################
    sql_test "[sql] app create (app-id=203)" $1 \
      "select created_at, closed_at, index from app WHERE index = 203" \
      "55||203"
    rest_test "[rest] app create (app-id=203)" \
      "/v2/applications/203?pretty" \
      200 \
      '"created-at-round": 55'
    sql_test "[sql] app create & delete (app-id=82)" $1 \
      "select created_at, closed_at, index from app WHERE index = 82" \
      "13|37|82"
    rest_test "[rest] app create & delete (app-id=82)" \
      "/v2/applications/82?pretty" \
      200 \
      '"created-at-round": 13' \
      '"destroyed-at-round": 37'

    ###############
    # Asset Tests #
    ###############
    sql_test "[sql] asset create / destroy" $1 \
      "select created_at, closed_at, index from asset WHERE index=135" \
      "23|33|135"
    sql_test "[sql] asset create" $1 \
      "select created_at, closed_at, index from asset WHERE index=168" \
      "35||168"

    ###########################
    # Application Local Tests #
    ###########################
    sql_test "[sql] app optin no closeout" $1 \
      "select created_at, closed_at, app from account_app WHERE addr=decode('rAMD0F85toNMRuxVEqtxTODehNMcEebqq49p/BZ9rRs=', 'base64') AND app=85" \
      "13||85"
    sql_test "[sql] app multiple optins first saved" $1 \
      "select created_at, closed_at, app from account_app WHERE addr=decode('Eze95btTASDFD/t5BDfgA2qvkSZtICa5pq1VSOUU0Y0=', 'base64') AND app=82" \
      "15|35|82"
    sql_test "[sql] app optin/optout/optin should clear closed_at" $1 \
      "select created_at, closed_at, app from account_app WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND app=203" \
      "57|59|203"

    #######################
    # Asset Holding Tests #
    #######################
    sql_test "[sql] asset optin" $1 \
      "select created_at, closed_at, assetid from account_asset WHERE addr=decode('MFkWBNGTXkuqhxtNVtRZYFN6jHUWeQQxqEn5cUp1DGs=', 'base64') AND assetid=27" \
      "13||27"
    sql_test "[sql] asset optin / close-out" $1 \
      "select created_at, closed_at, assetid from account_asset WHERE addr=decode('E/p3R9m9X0c7eAv9DapnDcuNGC47kU0BxIVdSgHaFbk=', 'base64') AND assetid=36" \
      "16|25|36"
    sql_test "[sql] asset optin / close-out / optin / close-out" $1 \
      "select created_at, closed_at, assetid from account_asset WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND assetid=135" \
      "25|31|135"
    sql_test "[sql] asset optin / close-out / optin" $1 \
      "select created_at, closed_at, assetid from account_asset WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND assetid=168" \
      "37|39|168"

    #################
    # Account Tests #
    #################
    sql_test "[sql] genesis account with no transactions" $1 \
      "select created_at, closed_at, microalgos from account WHERE addr = decode('4L294Wuqgwe0YXi236FDVI5RX3ayj4QL1QIloIyerC4=', 'base64')" \
      "0||5000000000000000"
    rest_test "[rest] genesis account with no transactions" \
      "/v2/accounts/4C633YLLVKBQPNDBPC3N7IKDKSHFCX3WWKHYIC6VAIS2BDE6VQXACGG3BQ?pretty" \
      200 \
      '"created-at-round": 0'

    sql_test "[sql] account created then never closed" $1 \
      "select created_at, closed_at, microalgos from account WHERE addr = decode('HoJZm6Z2n0EvGncuitv2BA7m8Gu/Y9rx22ZtKw1BbjI=', 'base64')" \
      "4||999999885998"
    rest_test "[rest] account created then never closed" \
      "/v2/accounts/D2BFTG5GO2PUCLY2O4XIVW7WAQHON4DLX5R5V4O3MZWSWDKBNYZJYKHVBQ?pretty" \
      200 \
      '"created-at-round": 4'

    sql_test "[sql] account create close create" $1 \
      "select created_at, closed_at, microalgos from account WHERE addr = decode('KbUa0wk9gB3BgAjQF0J9NqunWaFS+h4cdZdYgGfBes0=', 'base64')" \
      "17|19|100000"
    rest_test "[rest] account create close create" \
      "/v2/accounts/FG2RVUYJHWAB3QMABDIBOQT5G2V2OWNBKL5B4HDVS5MIAZ6BPLGR65YW3Y?pretty" \
      200 \
      '"created-at-round": 17' \
      '"closeout-at-round": 19'

    sql_test "[sql] account create close create close" $1 \
      "select created_at, closed_at, microalgos from account WHERE addr = decode('8rpfPsaRRIyMVAnrhHF+SHpq9za99C1NknhTLGm5Xkw=', 'base64')" \
      "9|15|0"
    rest_test "[rest] account create close create close" \
      "/v2/accounts/6K5F6PWGSFCIZDCUBHVYI4L6JB5GV5ZWXX2C2TMSPBJSY2NZLZGCF2NH5U?pretty" \
      200 \
      '"created-at-round": 9' \
      '"closeout-at-round": 15'

    rest_test "[rest] account with created and closed applications" \
      "/v2/accounts/XNMIHFHAZ2GE3XUKISNMOYKNFDOJXBJMVHRSXVVVIK3LNMT22ET2TA4N4I?pretty" \
      200 \
      '"created-at-round": 39' \
      '"created-at-round": 13' \
      '"destroyed-at-round": 37'

    rest_test "[rest] account with created / closed / created app local states" \
      "/v2/accounts/MRPIAVGS2OCS6UO6KC6YZ3445Q2DCMDMRG6OVKZVEYIHLE6BINDCIJ6J7U?pretty" \
      200 \
      '"optin-at-round": 57' \
      '"closeout-at-round": 59' \
      '"optin-at-round": 45' \
      '"closeout-at-round": 51'

}
