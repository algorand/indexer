#!/usr/bin/env bash
#
# Query an indexer postgres DB for boxes and dump them into stdout.
#

function help () {
  echo "This script generates a stream of boxes and prints them to stdout."
  echo ""
  echo "Requires 'psql' command to be available."
  echo ""
  echo "options:"
  echo "  --pg_user      -> Postgres username."
  echo "  --pg_pass      -> Postgres password."
  echo "  --pg_url       -> Postgres url (without http)."
  echo "  --pg_port      -> Postgres port."
  echo "  --pg_db        -> Postgres database."
}

#default selection queries
SELECTION_QUERY="select app, encode(name,'base64') from app_box limit 1000"
SELECTION_QUERY_COPY="COPY (select 'box'||','||app||','||encode(name,'base64') from app_box) TO stdout"

START_TIME=$SECONDS
PGUSER=
PGPASSWORD=
PGHOST=
PGPORT=
PGDB=
TEST=

while (( "$#" )); do
  case "$1" in
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
    *)
      echo "Unknown argument '$1'"
      echo ""
      help
      exit
  esac
  shift
done

if [ -z $PGUSER ] || [ -z $PGPASSWORD ] || [ -z $PGPORT ] || [ -z $PGHOST ] || [ -z $PGDB ]; then
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

psql_query "$SELECTION_QUERY_COPY"
