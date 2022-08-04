-- This file is setup_postgres.sql which gets compiled into go source using a go:generate statement in postgres.go
--
-- TODO? replace all 'addr bytea' with 'addr_id bigint' and a mapping table? makes addrs an 8 byte int that fits in a register instead of a 32 byte string

-- Optional, to make txn queries by asset fast:
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS txn_asset ON txn (asset, round, intra);

-- expand data.basics.AccountData
CREATE TABLE IF NOT EXISTS account (
  addr bytea primary key,
  microalgos bigint NOT NULL, -- okay because less than 2^54 Algos
  rewardsbase bigint NOT NULL,
  rewards_total bigint NOT NULL,
  deleted bool NOT NULL, -- whether or not it is currently deleted
  created_at bigint NOT NULL, -- round that the account is first used
  closed_at bigint, -- round that the account was last closed
  keytype varchar(8), -- "sig", "msig", "lsig", or NULL if unknown
  account_data jsonb NOT NULL -- trimmed ledgercore.AccountData that excludes the fields above; SQL 'NOT NULL' is held though the json string will be "null" iff account is deleted
);

-- data.basics.AccountData Assets[asset id] AssetHolding{}
CREATE TABLE IF NOT EXISTS account_asset (
  addr bytea NOT NULL, -- [32]byte
  assetid bigint NOT NULL,
  amount numeric(20) NOT NULL, -- need the full 18446744073709551615
  frozen boolean NOT NULL,
  deleted bool NOT NULL, -- whether or not it is currently deleted
  created_at bigint NOT NULL, -- round that the asset was added to an account
  closed_at bigint, -- round that the asset was last removed from the account
  PRIMARY KEY (addr, assetid)
);

-- For lookup up existing assets by account
CREATE INDEX IF NOT EXISTS account_asset_by_addr_partial ON account_asset(addr) WHERE NOT deleted;

-- Optional, to make queries of all asset balances fast /v2/assets/<assetid>/balances
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS account_asset_asset ON account_asset (assetid, addr ASC);

-- data.basics.AccountData AssetParams[index] AssetParams{}
CREATE TABLE IF NOT EXISTS asset (
  index bigint PRIMARY KEY,
  creator_addr bytea NOT NULL,
  params jsonb NOT NULL, -- data.basics.AssetParams; json string "null" iff asset is deleted
  deleted bool NOT NULL, -- whether or not it is currently deleted
  created_at bigint NOT NULL, -- round that the asset was created
  closed_at bigint -- round that the asset was closed; cannot be recreated because the index is unique
);

-- For account lookup
CREATE INDEX IF NOT EXISTS asset_by_creator_addr_deleted ON asset(creator_addr, deleted);

-- Includes indexer import state, migration state, special accounts (fee sink and
-- rewards pool) and account totals.
CREATE TABLE IF NOT EXISTS metastate (
  k text primary key,
  v jsonb
);

-- per app global state
-- roughly go-algorand/data/basics/userBalance.go AppParams
CREATE TABLE IF NOT EXISTS app (
  index bigint PRIMARY KEY,
  creator bytea NOT NULL, -- account address
  params jsonb NOT NULL, -- json string "null" iff app is deleted
  deleted bool NOT NULL, -- whether or not it is currently deleted
  created_at bigint NOT NULL, -- round that the asset was created
  closed_at bigint -- round that the app was deleted; cannot be recreated because the index is unique
);

-- For account lookup
CREATE INDEX IF NOT EXISTS app_by_creator_deleted ON app(creator, deleted);

-- per-account app local state
CREATE TABLE IF NOT EXISTS account_app (
  addr bytea,
  app bigint,
  localstate jsonb NOT NULL, -- json string "null" iff deleted from the account
  deleted bool NOT NULL, -- whether or not it is currently deleted
  created_at bigint NOT NULL, -- round that the app was added to an account
  closed_at bigint, -- round that the account_app was last removed from the account
  PRIMARY KEY (addr, app)
);

-- For looking up existing app local states by account
CREATE INDEX IF NOT EXISTS account_app_by_addr_partial ON account_app(addr) WHERE NOT deleted;
