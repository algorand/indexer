#!/usr/bin/env bash
#
# Query an indexer postgres DB for accounts and dump them into stdout.
#
# This script is intended to be used with validate_accounting.sh

function help () {
  echo "This script generates a stream of accounts and prints them to stdout."
  echo ""
  echo "Requires 'psql' command to be available."
  echo ""
  echo "options:"
  echo "  --convert_addr  -> Path to the convert_addr utility."
  echo "  --pg_user  -> Postgres username."
  echo "  --pg_pass  -> Postgres password."
  echo "  --pg_url   -> Postgres url (without http)."
  echo "  --pg_port  -> Postgres port."
  echo "  --pg_db    -> Postgres database."
  echo "  --query    -> [optional] Query to use for selecting accounts."
}

#default selection query
SELECTION_QUERY="select encode(addr,'base64') from account where deleted is not null limit 1000"

START_TIME=$SECONDS
PGUSER=
PGPASSWORD=
PGHOST=
PGPORT=
PGDB=
TEST=

while (( "$#" )); do
  case "$1" in
    --convert_addr)
      shift
      CONVERT_ADDR="$1"
      ;;
    --pg_user)
      shift
      PGUSER="$1"
      ;;
    --pg_pass)
      shift
      PGPASSWORD="$1"
      ;;
    --pg_url)
      shift
      PGHOST="$1"
      ;;
    --pg_port)
      shift
      PGPORT="$1"
      ;;
    --pg_db)
      shift
      PGDB="$1"
      ;;
    --test)
      TEST=1
      ;;
    -h|--help)
      help
      exit
      ;;
  esac
  shift
done

if [ -z $CONVERT_ADDR ] || [ -z $PGUSER ] || [ -z $PGPASSWORD ] || [ -z $PGPORT ] || [ -z $PGHOST ] || [ -z $PGDB ]; then
  help
  exit
fi

# Required for psql
export PGPASSWORD

# $1 - the query to execute
# $2 - if not empty prints the command before executing it
function psql_query {
  if [ ! -z $2 ]; then
    echo "psql -t -h $PGHOST -d $PGDB -XA -p $PGPORT -U $PGUSER -c \"$1\""
  fi
  psql -t -h $PGHOST -d $PGDB -XA -p $PGPORT -U $PGUSER -c "$1"
}

#####################
# Start the script! #
#####################

if [ ! -z $TEST ]; then
  echo "psql configuration test:"
  psql_query "select * from metastate" 1
fi

while read -r line; do
  ACCT=$($CONVERT_ADDR -addr $line)
  echo $ACCT
done < <((psql_query "$SELECTION_QUERY")) # TODO: pass accounts in with a file?
