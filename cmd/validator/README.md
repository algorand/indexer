# Validator

This tool us used to compare accounts in indexer to accounts in algod. 

# Usage

When running, validator looks for newline separated accounts from standard in. Progress is written to standard out, and any errors detected are written to stderr.

This can be used along with the [generate_accounts.sh](../../misc/generate_accounts.sh) to generate a stream of accounts.

For example here is how you would run the program with accounts generated from `generate_accounts.sh` and errors redirected to a file:
```
~$ ./generate_accounts.sh --pg_user postgres --pg_pass postgres --pg_url localhost --pg_db mainnet_database --pg_port 5432 | ./validator --algod-url http://localhost:4160 --algod-token token_here --indexer-url http://localhost:8980 --threads 4 --retries 5 2> errors.txt
```

# Building

Run `go build` from this directory.
