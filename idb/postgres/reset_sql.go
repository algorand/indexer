// Code generated from source reset.sql via go generate. DO NOT EDIT.

package postgres

const reset_sql = `DROP TABLE IF EXISTS account;
DROP TABLE IF EXISTS account_app;
DROP TABLE IF EXISTS account_asset;
DROP TABLE IF EXISTS app;
DROP TABLE IF EXISTS asset;
DROP TABLE IF EXISTS metastate;
UPDATE txn SET extra = NULL WHERE extra IS NOT NULL;
`
