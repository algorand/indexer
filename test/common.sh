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
# $4 - substring that should be in the response
function query_and_verify {
  local DESCRIPTION=$1
  local DATABASE=$2
  local QUERY=$3
  local SUBSTRING=$4

  set +e
  RES=$(base_query $DATABASE "$QUERY")
  if [[ $? != 0 ]]; then
    echo "ERROR from psql: $RESULT"
    fail_and_exit "$DESCRIPTION" "$QUERY" "psql had a non-zero exit code."
  fi
  set -e

  if [[ "$RES" != *"$SUBSTRING"* ]]; then
    fail_and_exit "$DESCRIPTION" "$QUERY" "unexpected response. should contain '$SUBSTRING', actual: '$RES'"
  fi

  print_alert "Passed test: $DESCRIPTION"
}

# call_and_verify helper
function base_call() {
  curl -o "$CURL_TEMPFILE" -w "%{http_code}" -q -s "$NET$1"
}

# CURL Test - query and veryify results
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
    fail_and_exit "$1" "$2" "curl had a non-zero exit code."
  fi
  set -e

  RES=$(cat "$CURL_TEMPFILE")
  if [[ "$CODE" != "$3" ]]; then
    fail_and_exit "$1" "$2" "unexpected HTTP status code expected $3 (actual $CODE): $RES"
  fi
  if [[ "$RES" != *"$4"* ]]; then
    fail_and_exit "$1" "$2" "unexpected response. should contain '$4', actual: $RES"
  fi

  print_alert "Passed test: $1"
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
    --pidfile $PIDFILE > /dev/null 2>&1 &
}


# $1 - postgres dbname
# $2 - e2edata tar.bz2 archive
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

  start_indexer $1
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
    call_and_verify 'Ensure migration updated specific account rewards.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI' 200 '"rewards":80000539878'
    call_and_verify 'Ensure migration updated specific account rewards @ round = 810.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=810' 200 '"rewards":80000539878'
    call_and_verify 'Ensure migration updated specific account rewards @ round = 800.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=800' 200 '"rewards":68000335902'
    call_and_verify 'Ensure migration updated specific account rewards @ round = 500.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=500' 200 '"rewards":28000055972'
    call_and_verify 'Ensure migration updated specific account rewards @ round = 100.' '/v2/accounts/FZPGVIFCMHCE2HC2LEDD7IZQLKZVHRV5PENSD26Y2AOS3OWCYMKTY33UXI?round=100' 200 '"rewards":7999999996'
}

# $1 - the DB to query
function create_delete_tests() {
    #####################
    # Application Tests #
    #####################
    query_and_verify "app create (app-id=203)" $1 \
      "select created_at, closed_at, index from app WHERE index = 203" \
      "55||203"
    query_and_verify "app create & delete (app-id=82)" $1 \
      "select created_at, closed_at, index from app WHERE index = 82" \
      "13|37|82"

    ###############
    # Asset Tests #
    ###############
    query_and_verify "asset create / destroy" $1 \
      "select created_at, closed_at, index from asset WHERE index=135" \
      "23|33|135"
    query_and_verify "asset create" $1 \
      "select created_at, closed_at, index from asset WHERE index=168" \
      "35||168"

    ###########################
    # Application Local Tests #
    ###########################
    query_and_verify "app optin no closeout" $1 \
      "select created_at, closed_at, app from account_app WHERE addr=decode('rAMD0F85toNMRuxVEqtxTODehNMcEebqq49p/BZ9rRs=', 'base64') AND app=85" \
      "13||85"
    query_and_verify "app multiple optins first saved" $1 \
      "select created_at, closed_at, app from account_app WHERE addr=decode('Eze95btTASDFD/t5BDfgA2qvkSZtICa5pq1VSOUU0Y0=', 'base64') AND app=82" \
      "15|35|82"
    query_and_verify "app optin/optout/optin should clear closed_at" $1 \
      "select created_at, closed_at, app from account_app WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND app=203" \
      "57||203"

    #######################
    # Asset Holding Tests #
    #######################
    query_and_verify "asset optin" $1 \
      "select created_at, closed_at, assetid from account_asset WHERE addr=decode('MFkWBNGTXkuqhxtNVtRZYFN6jHUWeQQxqEn5cUp1DGs=', 'base64') AND assetid=27" \
      "13||27"
    query_and_verify "asset optin / close-out" $1 \
      "select created_at, closed_at, assetid from account_asset WHERE addr=decode('E/p3R9m9X0c7eAv9DapnDcuNGC47kU0BxIVdSgHaFbk=', 'base64') AND assetid=36" \
      "16|25|36"
    query_and_verify "asset optin / close-out / optin / close-out" $1 \
      "select created_at, closed_at, assetid from account_asset WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND assetid=135" \
      "25|31|135"
    query_and_verify "asset optin / close-out / optin" $1 \
      "select created_at, closed_at, assetid from account_asset WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND assetid=168" \
      "37||168"

    #################
    # Account Tests #
    #################
    query_and_verify "genesis account with no transactions" $1 \
      "select created_at, closed_at, microalgos from account WHERE addr = decode('4L294Wuqgwe0YXi236FDVI5RX3ayj4QL1QIloIyerC4=', 'base64')" \
      "0||5000000000000000"
    query_and_verify "account created then never closed" $1 \
      "select created_at, closed_at, microalgos from account WHERE addr = decode('HoJZm6Z2n0EvGncuitv2BA7m8Gu/Y9rx22ZtKw1BbjI=', 'base64')" \
      "4||999999885998"
    query_and_verify "account create close create" $1 \
      "select created_at, closed_at, microalgos from account WHERE addr = decode('KbUa0wk9gB3BgAjQF0J9NqunWaFS+h4cdZdYgGfBes0=', 'base64')" \
      "17||100000"
    query_and_verify "account create close create close" $1 \
      "select created_at, closed_at, microalgos from account WHERE addr = decode('8rpfPsaRRIyMVAnrhHF+SHpq9za99C1NknhTLGm5Xkw=', 'base64')" \
      "9|15|0"
}
