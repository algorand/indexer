#!/usr/bin/env bash
#
# This script requires the convertToAddress.go tool to be built and provided
# with the --convert_addr field

function help () {
  echo "This script compares account data in Indexer to data in algod."
  echo ""
  echo "options:"
  echo "  --pg_user       -> Postgres username."
  echo "  --pg_pass       -> Postgres password."
  echo "  --pg_url        -> Postgres url (without http)."
  echo "  --pg_port       -> Postgres port."
  echo "  --pg_db         -> Postgres database."
  echo "  --datadir       -> Full path to data directory."
  echo "  --algod         -> Algod url (with http), required if datadir is not available."
  echo "  --algod_token   -> Algod API token, required if datadir is not available."
  echo "  --indexer       -> Indexer url (with http)."
  echo "  --indexer_token -> Indexer API token. If not used, set to any non empty value."
  echo "  --retries       -> [optional, default=0] If an error is found this many retry attempts will be made."
}

MAX_ATTEMPTS=1
START_TIME=$SECONDS
ALGOD_TOKEN=
ALGOD_NET=
INDEXER_NET=
INDEXER_TOKEN=""
TEST=
CONVERT_ADDR=

while (( "$#" )); do
  case "$1" in
    --indexer)
      shift
      INDEXER_NET="$1"
      ;;
    --indexer_token)
      shift
      INDEXER_TOKEN="$1"
      ;;
    --algod)
      shift
      ALGOD_NET="$1"
      ;;
    --algod_token)
      shift
      ALGOD_TOKEN="$1"
      ;;
    --datadir)
      shift
      ALGOD_TOKEN="$(cat $1/algod.token)"
      ALGOD_NET="$(cat $1/algod.net)"
      ;;
    --retries)
      shift
      MAX_ATTEMPTS=$((MAX_ATTEMPTS + $1))
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

if [ -z $INDEXER_NET ] || [ -z $INDEXER_TOKEN ] || [ -z $ALGOD_NET ] || [ -z $ALGOD_TOKEN ]; then
  help
  exit
fi

function stats {
  ELAPSED=$(($SECONDS - $START_TIME))

  echo ""
  echo ""
  echo "Number of errors: [$ERROR_COUNT / $ACCOUNT_COUNT]"
  echo "Retry count: $RETRY_COUNT"
  printf 'Test run duration: %02dh:%02dm:%02ds\n' $(($ELAPSED/3600)) $(($ELAPSED%3600/60)) $(($ELAPSED%60))
}

function print_error_details {
  # TODO: Should we print the non-normalized accounts as well?
  printf "\n$s: Accounting mismatch for %s\n" "($ERROR_COUNT)" "$ACCT" >&2
  printf "\nIndexer JSON:\n$INDEXER_ACCT_NORMALIZED"                   >&2
  printf "\nALGOD JSON:\n\n$ALGOD_ACCT_NORMALIZED"                     >&2
}

# Fancy jq function to normalize the json coming out of indexer or algod.
# $1 - json data
function normalize_json {
  echo "$1" | jq -M\
    '
    # If there is a top level account field (indexer), use it.
    if .account then .account else . end
      |
    # Remove uninitialized accounts returned by algod
    select(.amount != 0)
      |
    # Remove any object marked deleted in indexer
    walk (
      if type == "object" then
        del( . | select(."deleted" == true))
      else
        .
      end)
      |
    # Remove fields which are not universally supported
    walk(
        if type == "object" then
          with_entries(
            select(
              # deleted is only in indexer
              .key != "deleted" and

              # at-round fields are only supported in indexer
              .key != "created-at-round" and
              .key != "deleted-at-round" and
              .key != "destroyed-at-round" and
              .key != "oopted-in-at-roundptin-at-round" and
              .key != "opted-in-at-round" and
              .key != "opted-out-at-round" and
              .key != "closeout-at-round" and
              .key != "closed-out-at-round" and
              .key != "closed-at-round" and

              # indexer does not attach creator to asset holdings
              .key != "creator" and

              # indexer puts a special sig-type field at the top level
              .key != "sig-type" and

              # indexer and algod are almost always off by 1 round
              .key != "round" and

              # algod seems to look this up on demand, indexer has a stale value
              .key != "reward-base" and

              # make sure empty fields are handled consistently
              .value != null and
              .value != "" and
              .value != [] and
              .value != {} and
              .value != 0)
                |
              # Indexer adds a space to NotParticipating
              if .key == "status" and .value == "Not Participating" then
                .value = "NotParticipating"
              else
                .
              end
            )
        elif type == "array" then
          map(select(. != null and . != {} and . != []))
        else
          .
        end)'
}

# $1 - account address
function update_account {
  INDEXER_ACCT=$(curl -s -q "$INDEXER_NET/v2/accounts/$1?pretty" -H "Authorization: Bearer $INDEXER_TOKEN")
  INDEXER_ACCT_NORMALIZED=$(normalize_json "$INDEXER_ACCT")

  ALGOD_ACCT=$(curl -s -q -H "Authorization: Bearer $ALGOD_TOKEN" "$ALGOD_NET/v2/accounts/$1?pretty")
  ALGOD_ACCT_NORMALIZED=$(normalize_json "$ALGOD_ACCT")
}

#####################
# Start the script! #
#####################

# Make sure stats are reported after ctrl-C, this script will probably never end.
trap stats EXIT

# Print connection tests if enabled.
if [ ! -z $TEST ]; then
  echo -e "\nindexer configuration test:"
  echo -e "~$ "'curl -s -q "$INDEXER_NET/health?pretty" -H "Authorization: Bearer $INDEXER_TOKEN"'
  curl -s -q "$INDEXER_NET/health?pretty" -H "Authorization: Bearer $INDEXER_TOKEN"
  echo -e "\nalgod configuration test:"
  echo -e "~$"' curl -s -q "$ALGOD_NET/v2/status?pretty" -H "Authorization: Bearer $ALGOD_TOKEN"'
  curl -s -q "$ALGOD_NET/v2/status?pretty" -H "Authorization: Bearer $ALGOD_TOKEN"
  echo ""
fi

# Things to accumulate for final report.
ACCOUNT_COUNT=0
ERROR_COUNT=0
RETRY_COUNT=0

# Loop through all accounts from stdin
# cat with no input copies stdin -> stdout.
cat | while read -r ACCT; do
  # print progress
  if [ $(($ACCOUNT_COUNT%50)) -eq 0 ]; then
      printf "\n%-8d : " "$ACCOUNT_COUNT"
  fi

  # get normalized account details, optional retry..
  n=0
  while true; do
    update_account $ACCT
    # break out on success
    [ "$INDEXER_ACCT_NORMALIZED" == "$ALGOD_ACCT_NORMALIZED" ] && break
    ((n++))
    ((RETRY_COUNT++))
    # break out on max attempts
    [ "$n" -ge $MAX_ATTEMPTS ] && break
    sleep 1
  done

  # print errors
  if [ "$INDEXER_ACCT_NORMALIZED" != "$ALGOD_ACCT_NORMALIZED" ] ; then
    ((ERROR_COUNT++))
    print_error_details
  fi
  ((ACCOUNT_COUNT++))

  # print progress
  if [ "$n" -ne 0 ]; then
    printf "X"
  else
    printf "."
  fi
done

