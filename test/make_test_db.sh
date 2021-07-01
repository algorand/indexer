#!/bin/bash

# Run this script with PGPASSWORD env variable set. Prior to running, stop the
# indexer writer to get a consistent result.

set -x

HOST=localhost
PORT=5432
USER=postgres
SOURCE_DB=m2
TARGET_DB=test
OUTPUT_FILE=test.sql

# Create the target database.
createdb -h $HOST -p $PORT -U $USER $TARGET_DB

# Copy the schema.
pg_dump -h $HOST -p $PORT -U $USER -d $SOURCE_DB -s | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB

# Copy some block_header rows.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM block_header WHERE round % 100000 = 0) TO STDOUT;" | dd status=progress | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY block_header FROM STDIN;"

# Copy the block_header for the max accounted round.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM block_header WHERE round = CAST((SELECT v->>'next_account_round' FROM metastate WHERE k='state') AS bigint) - 1) TO STDOUT;" | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY block_header FROM STDIN;"

# Copy some txn rows.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM txn WHERE round % 100000 = 0) TO STDOUT;" | dd status=progress | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY txn FROM STDIN;"

# Copy some txn_participation rows.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM txn_participation WHERE round % 100000 = 0) TO STDOUT;" | dd status=progress | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY txn_participation FROM STDIN;"

# Copy some account rows.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM account WHERE get_byte(addr, 0) = 0) TO STDOUT;" | dd status=progress | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY account FROM STDIN;"

# Copy some account_asset rows.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM account_asset WHERE get_byte(addr, 0) = 0) TO STDOUT;" | dd status=progress | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY account_asset FROM STDIN;"

# Copy some asset rows.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM asset WHERE get_byte(creator_addr, 0) = 0) TO STDOUT;" | dd status=progress | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY asset FROM STDIN;"

# Copy the metastate table.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM metastate) TO STDOUT;" | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY metastate FROM STDIN;"

# Copy some app rows.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM app WHERE get_byte(creator, 0) < 32) TO STDOUT;" | dd status=progress | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY app FROM STDIN;"

# Copy some account_app rows.
psql -h $HOST -p $PORT -U $USER -d $SOURCE_DB -c "COPY(SELECT * FROM account_app WHERE get_byte(addr, 0) = 0) TO STDOUT;" | dd status=progress | psql -h $HOST -p $PORT -U $USER -d $TARGET_DB -c "COPY account_app FROM STDIN;"

# Dump target database content to a file.
pg_dump -h $HOST -p $PORT -U $USER --column-inserts --no-acl $TARGET_DB > $OUTPUT_FILE

# Drop the target database.
dropdb -h $HOST -p $PORT -U $USER $TARGET_DB
