#!/usr/bin/env sh
set -e

echo "Importing test data."
./cmd/algorand-indexer/algorand-indexer import \
  -P "host=indexer-db port=5432 user=algorand password=harness dbname=$DATABASE_NAME sslmode=disable" \
  --genesis "/tmp/algod/genesis.json" \
  /tmp/blocktars/*

echo "Starting indexer in read-only mode."
./cmd/algorand-indexer/algorand-indexer daemon \
  --no-algod \
  -P "host=indexer-db port=5432 user=algorand password=harness dbname=$DATABASE_NAME sslmode=disable"
