// Code generated from source reset.sql via go generate. DO NOT EDIT.

package postgres

const reset_sql = `TRUNCATE account;
TRUNCATE account_app;
TRUNCATE account_asset;
TRUNCATE app;
TRUNCATE asset;
TRUNCATE metastate;
UPDATE txn SET extra = NULL WHERE extra IS NOT NULL;
`
