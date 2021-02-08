// Code generated from source reset.sql via go generate. DO NOT EDIT.

package postgres

const reset_sql = `TRUNCATE account;
TRUNCATE account_app;
TRUNCATE account_asset;
TRUNCATE app;
TRUNCATE asset;
TRUNCATE metastate;
UPDATE txn SET extra = NULL;

-- schema changes from various migrations, made re-run safe by 'IF NOT EXISTS'
ALTER TABLE account ADD COLUMN IF NOT EXISTS rewards_total bigint NOT NULL DEFAULT 0;
ALTER TABLE account ADD COLUMN IF NOT EXISTS deleted boolean DEFAULT NULL;
ALTER TABLE account ADD COLUMN IF NOT EXISTS created_at bigint DEFAULT NULL;
ALTER TABLE account ADD COLUMN IF NOT EXISTS closed_at bigint DEFAULT NULL;
ALTER TABLE app ADD COLUMN IF NOT EXISTS deleted boolean DEFAULT NULL;
ALTER TABLE app ADD COLUMN IF NOT EXISTS created_at bigint DEFAULT NULL;
ALTER TABLE app ADD COLUMN IF NOT EXISTS closed_at bigint DEFAULT NULL;
ALTER TABLE account_app ADD COLUMN IF NOT EXISTS deleted boolean DEFAULT NULL;
ALTER TABLE account_app ADD COLUMN IF NOT EXISTS created_at bigint DEFAULT NULL;
ALTER TABLE account_app ADD COLUMN IF NOT EXISTS closed_at bigint DEFAULT NULL;
ALTER TABLE account_asset ADD COLUMN IF NOT EXISTS deleted boolean DEFAULT NULL;
ALTER TABLE account_asset ADD COLUMN IF NOT EXISTS created_at bigint DEFAULT NULL;
ALTER TABLE account_asset ADD COLUMN IF NOT EXISTS closed_at bigint DEFAULT NULL;
ALTER TABLE asset ADD COLUMN IF NOT EXISTS deleted boolean DEFAULT NULL;
ALTER TABLE asset ADD COLUMN IF NOT EXISTS created_at bigint DEFAULT NULL;
ALTER TABLE asset ADD COLUMN IF NOT EXISTS closed_at bigint DEFAULT NULL;
`
