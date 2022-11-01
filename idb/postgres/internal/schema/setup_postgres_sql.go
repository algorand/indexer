// Code generated from source setup_postgres.sql via go generate. DO NOT EDIT.

package schema

const SetupPostgresSql = `-- This file is setup_postgres.sql which gets compiled into go source using a go:generate statement in postgres.go
--
-- TODO? replace all 'addr bytea' with 'addr_id bigint' and a mapping table? makes addrs an 8 byte int that fits in a register instead of a 32 byte string

CREATE TABLE IF NOT EXISTS block_header (
  round bigint PRIMARY KEY,
  realtime timestamp without time zone NOT NULL,
  rewardslevel bigint NOT NULL,
  header jsonb NOT NULL
);

-- For looking round by timestamp. We could replace this with a round-to-timestamp algorithm, it should be extremely
-- efficient since there is such a high correlation between round and time.
CREATE INDEX IF NOT EXISTS block_header_time ON block_header (realtime);

CREATE TABLE IF NOT EXISTS txn (
  round bigint NOT NULL,
  intra integer NOT NULL,
  typeenum smallint NOT NULL,
  asset bigint NOT NULL, -- 0=Algos, otherwise AssetIndex
  txid bytea, -- base32 of [32]byte hash, or NULL for inner transactions.
  txn jsonb NOT NULL, -- json encoding of signed txn with apply data; inner txns exclude nested inner txns
  extra jsonb NOT NULL,
  PRIMARY KEY ( round, intra )
);

-- For transaction lookup
CREATE INDEX IF NOT EXISTS txn_by_tixid ON txn ( txid );

-- Optional, to make txn queries by asset fast:
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS txn_asset ON txn (asset, round, intra);

CREATE TABLE IF NOT EXISTS txn_participation (
  addr bytea NOT NULL,
  round bigint NOT NULL,
  intra integer NOT NULL
);

-- For query account transactions
CREATE UNIQUE INDEX IF NOT EXISTS txn_participation_i ON txn_participation ( addr, round DESC, intra DESC );

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

-- For looking up app box storage
CREATE TABLE IF NOT EXISTS app_box (
  app bigint NOT NULL,
  name bytea NOT NULL,
  value bytea NOT NULL, -- upon creation 'value' is 0x000...000 with length being the box'es size
  PRIMARY KEY (app, name)
);
`
