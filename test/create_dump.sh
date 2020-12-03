#!/usr/bin/env bash
#
# Automate setting up and exporting a postgres dump.

source common.sh

if [ "$#" -ne 1 ]; then
  echo "Must provide 1 argument: path to e2edata archive."
  exit
fi

if ! ask "Have you built the older version of 'algorand-indexer'?"; then
  echo "Come back when you're serious."
  exit
fi

trap cleanup EXIT

start_postgres
start_indexer_with_blocks createdestroy "$1"

sleep infinity
