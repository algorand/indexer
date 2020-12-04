#!/usr/bin/env bash
#
# Automate setting up and exporting a postgres dump.

source common.sh

if [ "$#" -ne 2 ]; then
  echo "Must provide 2 argument: <path to e2edata archive> <dumpfile target>"
  exit
fi

if ! ask "Have you built the version of 'algorand-indexer' you want?"; then
  echo "Come back when you're serious."
  exit
fi

trap cleanup EXIT

start_postgres
start_indexer_with_blocks test "$1"
docker exec test-container pg_dump -d test -Ualgorand > "$2"
