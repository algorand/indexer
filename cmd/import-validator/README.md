# Import validation tool

The import validator tool imports blocks into indexer database and algod's
sqlite database in lockstep and checks that the modified accounts are the same
in the two databases.
It lets us detect the first round where an accounting discrepancy occurs
and it prints out the difference before crashing.

There is a small limitation, however.
The set of modified accounts is computed using the sqlite database.
Thus, if indexer's accounting were to modify a superset of those accounts,
this tool would not detect it.
This, however, should be unlikely.

# Using the tool

Running the tool is similar to running `algorand-indexer` with the exception
that one has to specify the path to the sqlite (go-algorand) database.

Create a new postgres (indexer) database.
```
createdb db0
```

Create a directory for the sqlite (go-algorand) database.
This will essentially serve as a non-archival algod database.
```
mkdir ledger
```

Run the tool.
```
./import-validator --algod-net localhost:8080 --algod-token ALGOD_TOKEN --algod-ledger ledger --postgres "user=ubuntu host=localhost database=db0 sslmode=disable"
```

The tool can be safely stopped and started again.
Also, the provided indexer database can be created by indexer itself, and the
provided ledger sqlite database can be created by algod.
Since the import validator essentially exercises indexer's and go-algorand's
database code, this should work.
However, the sqlite database must be at the same round as the postgres database
or one round behind; otherwise, the import validator will fail to start.

# Performance

Reading and writing to/from the sqlite database is negligible compared to
importing blocks into the postgres database.
However, the tool has to read the modified accounts after importing each block.
Thus, we can expect the import validator to be 1.5 times slower than indexer.
