
CREATE TABLE public.block_header (
    round INT8 NOT NULL,
    realtime TIMESTAMP NOT NULL,
    rewardslevel INT8 NOT NULL,
    header JSONB NOT NULL,
    CONSTRAINT block_header_pkey PRIMARY KEY (round ASC),
    INDEX block_header_time (realtime ASC)
);

CREATE TABLE public.txn (
    round INT8 NOT NULL,
    intra INT8 NOT NULL,
    typeenum INT2 NOT NULL,
    asset INT8 NOT NULL,
    txid BYTES NULL,
    txn JSONB NOT NULL,
    extra JSONB NOT NULL,
    note3 BYTES NULL AS (substring(decode((txn->'txn':::STRING)->>'note':::STRING, 'base64':::STRING), 1:::INT8, 3:::INT8)) STORED,
    CONSTRAINT txn_pkey PRIMARY KEY (round ASC, intra ASC),
    INDEX ndly_txn_asset_extra (asset ASC, round ASC, intra ASC) STORING (txn, extra) WHERE asset > 0:::INT8,
    INDEX ndly_txn_txid (txid ASC) STORING (asset, txn, extra),
    INDEX ndly_txn_note3 (note3 ASC) WHERE (note3 IS NOT NULL) AND (note3 != '\x000000':::BYTES)
);

CREATE TABLE public.account (
    addr BYTES NOT NULL,
    microalgos INT8 NOT NULL,
    rewardsbase INT8 NOT NULL,
    rewards_total INT8 NOT NULL,
    deleted BOOL NOT NULL,
    created_at INT8 NOT NULL,
    closed_at INT8 NULL,
    keytype VARCHAR(8) NULL,
    account_data JSONB NOT NULL,
    CONSTRAINT account_pkey PRIMARY KEY (addr ASC)
);

CREATE TABLE public.account_asset (
    addr BYTES NOT NULL,
    assetid INT8 NOT NULL,
    amount DECIMAL(20) NOT NULL,
    frozen BOOL NOT NULL,
    deleted BOOL NOT NULL,
    created_at INT8 NOT NULL,
    closed_at INT8 NULL,
    CONSTRAINT account_asset_pkey PRIMARY KEY (addr ASC, assetid ASC),
    INDEX account_asset_by_addr_partial (addr ASC) WHERE NOT deleted,
    INDEX account_asset_asset (assetid ASC, addr ASC),
    INDEX ndly_account_asset_holder (assetid ASC, amount DESC) WHERE amount > 0:::DECIMAL,
    INDEX ndly_account_asset_optedin (assetid ASC, amount DESC) STORING (frozen, deleted, created_at, closed_at) WHERE NOT deleted
);

CREATE TABLE public.asset (
    id INT8 NOT NULL,
    creator_addr BYTES NOT NULL,
    params JSONB NOT NULL,
    deleted BOOL NOT NULL,
    created_at INT8 NOT NULL,
    closed_at INT8 NULL,
    CONSTRAINT asset_pkey PRIMARY KEY (id ASC),
    INDEX ndly_asset_by_creator_addr_deleted (creator_addr ASC, deleted ASC) STORING (params, created_at, closed_at),
    INVERTED INDEX ndly_asset_params_an ((params->>'an':::STRING) gin_trgm_ops),
    INVERTED INDEX ndly_asset_params_un ((params->>'un':::STRING) gin_trgm_ops)
);

CREATE TABLE public.metastate (
    k STRING NOT NULL,
    v JSONB NULL,
    CONSTRAINT metastate_pkey PRIMARY KEY (k ASC)
);

CREATE TABLE public.app (
    id INT8 NOT NULL,
    creator BYTES NOT NULL,
    params JSONB NOT NULL,
    deleted BOOL NOT NULL,
    created_at INT8 NOT NULL,
    closed_at INT8 NULL,
    CONSTRAINT app_pkey PRIMARY KEY (id ASC),
    INDEX app_by_creator_deleted (creator ASC, deleted ASC)
);

CREATE TABLE public.account_app (
    addr BYTES NOT NULL,
    app INT8 NOT NULL,
    localstate JSONB NOT NULL,
    deleted BOOL NOT NULL,
    created_at INT8 NOT NULL,
    closed_at INT8 NULL,
    CONSTRAINT account_app_pkey PRIMARY KEY (addr ASC, app ASC),
    INDEX account_app_by_addr_partial (addr ASC) WHERE NOT deleted,
    INDEX ndly_account_app_app (app ASC, addr ASC)
);

CREATE TABLE public.app_box (
    app INT8 NOT NULL,
    name BYTES NOT NULL,
    value BYTES NOT NULL,
    CONSTRAINT app_box_pkey PRIMARY KEY (app ASC, name ASC)
);

CREATE TABLE public.txn_participation (
    addr BYTES NOT NULL,
    round INT8 NOT NULL,
    intra INT8 NOT NULL,
    CONSTRAINT txn_participation_pkey PRIMARY KEY (addr ASC, round DESC, intra DESC),
    INDEX ndly_txn_participation_rnd (round ASC)
);