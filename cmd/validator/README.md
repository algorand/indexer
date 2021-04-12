# Validator

This tool is used to verify indexer and algod have the same account data.

# Usage

When running, validator looks for newline separated accounts from standard in. Progress is written to standard out, and any errors detected are written to stderr.

This can be used along with the [generate_accounts.sh](../../misc/generate_accounts.sh) to generate a stream of accounts.

For example here is how you would run the program with accounts generated from `generate_accounts.sh` and errors redirected to a file:
```
~$ ./generate_accounts.sh --pg_user postgres --pg_pass postgres --pg_url localhost --pg_db mainnet_database --pg_port 5432 | ./validator --algod-url http://localhost:4160 --algod-token token_here --indexer-url http://localhost:8980 --threads 4 --retries 5 2> errors.txt
```

## generate_accounts.sh

This is a simple wrapper to `psql` but can run in a couple different modes.

If the `--convert_addr` option is used the base64 encoded addresses will be converted into algorand formatted addresses. This feature isn't required for use with `validator`, which will perform the conversion itself if needed.

The other options are mostly self explanatory:
```
$ ./generate_accounts.sh -h
This script generates a stream of accounts and prints them to stdout.
If the convert_addr tool is provided accounts will also be decoded
from base64 to the algorand standard address format.

Requires 'psql' command to be available.

options:
  --convert_addr -> [optional] Path to the convert_addr utility.
  --pg_user      -> Postgres username.
  --pg_pass      -> Postgres password.
  --pg_url       -> Postgres url (without http).
  --pg_port      -> Postgres port.
  --pg_db        -> Postgres database.
  --query        -> [optional] Query to use for selecting accounts.
```

### --query

This option allows you to select exactly which accounts to return. This can be useful during a migration to test partial results. For example if accounts are processed alphabetically you can select the accounts with the following:
```
~$ ./convert_addr -addr J6RXYXUSCWX3JUFWT3WYBBXINHXVMETEGMVYDDHKUCVTMKTMBOUJZ532TQ
T6N8XpIVr7TQtp7tgIboae9WEmQzK4GM6qCrNipsC6g=
~$ ./generate_accounts.sh \
      --pg_user postgres\
      --pg_pass postgres\
      --pg_url postgres.com\
      --pg_port 5432\
      --pg_db indexer_database\
      --query "COPY (select encode(addr, 'base64') from account where addr<decode('T6N8XpIVr7TQtp7tgIboae9WEmQzK4GM6qCrNipsC6g=','base64')) TO stdout"
```

# Building

Run `go build` from this directory.
