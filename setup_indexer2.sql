CREATE TABLE IF NOT EXISTS block_header (
round bigint PRIMARY KEY,
realtime timestamp without time zone NOT NULL,
header bytea NOT NULL
);
CREATE INDEX IF NOT EXISTS block_header_time ON block_header (realtime);

CREATE TABLE IF NOT EXISTS txn (
round bigint NOT NULL,
intra smallint NOT NULL,
typeenum smallint NOT NULL,
asset bigint NOT NULL, -- TODO? 0=Algos, otherwise AssetIndex
txnbytes bytea NOT NULL,
txn jsonb NOT NULL,
PRIMARY KEY ( round, intra )
);

CREATE TABLE IF NOT EXISTS txn_participation (
addr bytea NOT NULL,
round bigint NOT NULL,
intra smallint NOT NULL
);
CREATE INDEX IF NOT EXISTS txn_participation_i ON txn_participation ( addr, round DESC, intra DESC );

-- bookeeping for local file import
CREATE TABLE IF NOT EXISTS imported (path text);

-- like ledger/accountdb.go
CREATE TABLE IF NOT EXISTS accounttotals (
  id string primary key,
  online integer,
  onlinerewardunits integer,
  offline integer,
  offlinerewardunits integer,
  notparticipating integer,
  notparticipatingrewardunits integer,
  rewardslevel integer);

-- expand data.basics.AccountData
CREATE TABLE IF NOT EXISTS accountbase (
  addr bytea primary key,
  microalgos bigint NOT NULL,
  account_data jsonb NOT NULL -- data.basics.AccountData except AssetParams and Assets
);

-- data.basics.AccountData Assets[asset id] AssetHolding{}
CREATE TABLE IF NOT EXISTS account_asset (
  addr bytea NOT NULL, -- [32]byte
  assetid bigint NOT NULL,
  amount bigint NOT NULL,
  frozen boolean NOT NULL,
  PRIMARY KEY (addr, assetid)
);

-- data.basics.AccountData AssetParams[index] AssetParams{}
CREATE TABLE IF NOT EXISTS asset (
  index bigint PRIMARY KEY,
  creator_addr bytea NOT NULL,
  params jsonb NOT NULL -- data.basics.AssetParams -- TODO index some fields?
);
-- TODO: index on creator_addr

