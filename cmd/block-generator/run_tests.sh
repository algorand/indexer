#!/usr/bin/env bash

CONNECTION_STRING=""
INDEXER_BINARY=""
REPORT_DIR=""
DURATION="1h"
LOG_LEVEL="error"

help() {
  echo "Usage:"
  echo " -v|--verbose    enable verbose script output."
  echo " -c|--connection-string"
  echo "                 PostgreSQL connection string."
  echo " -i|--indexer    path to indexer binary."
  echo " -r|--report-dir directory where the report should be written."
  echo " -d|--duration   test duration."
  echo " -l|--level      log level to pass to Indexer."
  exit
}

while :; do
  case "${1-}" in
  -h | --help) help ;;
  -v | --verbose) set -x ;;
  -c | --connection-string)
    CONNECTION_STRING="${2-}"
    shift
    ;;
  -i | --indexer)
    INDEXER_BINARY="${2-}"
    shift
    ;;
  -r | --report-dir)
    REPORT_DIR="${2-}"
    shift
    ;;
  -d | --duration)
    DURATION="${2-}"
    shift
    ;;
  -l | --level)
    LOG_LEVEL="${2-}"
    shift
    ;;
  -?*) echo "Unknown option: $1" && exit 1;;
  *) break ;;
  esac
  shift
done

args=("$@")

if [ -z "$CONNECTION_STRING" ]; then
  echo "Missing required connection string parameter (-c / --connection-string)."
  exit 1
fi

if [ -z "$INDEXER_BINARY" ]; then
  echo "Missing required indexer binary parameter (-i / --indexer)."
  exit 1
fi

echo "Running with binary: $INDEXER_BINARY"
echo "Report directory: $REPORT_DIR"
echo "Duration: $DURATION"
echo "Log Level: $LOG_LEVEL"

"$INDEXER_BINARY" \
         util block-generator runner \
  -i "$INDEXER_BINARY" \
  -s /home/ubuntu/scenarios/ \
  -d "$DURATION" \
  -c "$CONNECTION_STRING" \
  --report-directory "$REPORT_DIR" \
  --log-level "$LOG_LEVEL" \
  --reset

