package postgres

import (
	"context"
	"testing"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	ledger_for_evaluator "github.com/algorand/indexer/idb/postgres/internal/ledger_for_evaluator"
	"github.com/algorand/indexer/util/test"
)

func TestLedgerForEvaluatorBlockHdr(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO block_header (round, realtime, rewardslevel, header) " +
			"VALUES (2, 'epoch', 0, $1)"
	header := bookkeeping.BlockHeader{
		RewardsState: bookkeeping.RewardsState{
			FeeSink: test.FeeAddr,
		},
	}
	_, err = db.db.Exec(query, encoding.EncodeBlockHeader(header))
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, header.GenesisHash, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.BlockHdr(basics.Round(2))
	require.NoError(t, err)

	assert.Equal(t, header, ret)
}

func TestLedgerForEvaluatorAccountTableBasic(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted, " +
			"created_at, account_data) " +
			"VALUES ($1, $2, $3, $4, false, 0, $5)"

	var voteID crypto.OneTimeSignatureVerifier
	voteID[0] = 2
	var selectionID crypto.VRFVerifier
	selectionID[0] = 3
	accountDataWritten := basics.AccountData{
		Status:          basics.Online,
		VoteID:          voteID,
		SelectionID:     selectionID,
		VoteFirstValid:  basics.Round(4),
		VoteLastValid:   basics.Round(5),
		VoteKeyDilution: 6,
		AuthAddr:        test.AccountA,
	}

	accountDataFull := accountDataWritten
	accountDataFull.MicroAlgos = basics.MicroAlgos{Raw: 2}
	accountDataFull.RewardsBase = 3
	accountDataFull.RewardedMicroAlgos = basics.MicroAlgos{Raw: 4}
	accountDataFull.AssetParams = make(map[basics.AssetIndex]basics.AssetParams)
	accountDataFull.Assets = make(map[basics.AssetIndex]basics.AssetHolding)
	accountDataFull.AppLocalStates = make(map[basics.AppIndex]basics.AppLocalState)
	accountDataFull.AppParams = make(map[basics.AppIndex]basics.AppParams)

	_, err = db.db.Exec(
		query, test.AccountB[:], accountDataFull.MicroAlgos.Raw, accountDataFull.RewardsBase,
		accountDataFull.RewardedMicroAlgos.Raw,
		encoding.EncodeAccountData(accountDataWritten))
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	accountDataRet, round, err := l.LookupWithoutRewards(7, test.AccountB)
	require.NoError(t, err)

	assert.Equal(t, basics.Round(7), round)
	assert.Equal(t, accountDataFull, accountDataRet)
}

func TestLedgerForEvaluatorAccountTableDeleted(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted, " +
			"created_at, account_data) " +
			"VALUES ($1, 2, 3, 4, true, 0, $2)"

	accountData := basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 5},
	}
	_, err = db.db.Exec(
		query, test.AccountB[:], encoding.EncodeAccountData(accountData))
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	accountDataRet, round, err := l.LookupWithoutRewards(7, test.AccountB)
	require.NoError(t, err)

	assert.Equal(t, basics.Round(7), round)
	assert.Equal(t, basics.AccountData{}, accountDataRet)
}

func TestLedgerForEvaluatorAccountTableNullAccountData(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, $2, 0, 0, false, 0)"

	accountDataFull := basics.AccountData{
		MicroAlgos:     basics.MicroAlgos{Raw: 2},
		AssetParams:    make(map[basics.AssetIndex]basics.AssetParams),
		Assets:         make(map[basics.AssetIndex]basics.AssetHolding),
		AppLocalStates: make(map[basics.AppIndex]basics.AppLocalState),
		AppParams:      make(map[basics.AppIndex]basics.AppParams),
	}
	_, err = db.db.Exec(query, test.AccountA[:], accountDataFull.MicroAlgos.Raw)
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	accountDataRet, _, err := l.LookupWithoutRewards(0, test.AccountA)
	require.NoError(t, err)

	assert.Equal(t, accountDataFull, accountDataRet)
}

func TestLedgerForEvaluatorAccountAssetTable(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	_, err = db.db.Exec(query, test.AccountA[:])
	require.NoError(t, err)

	query =
		"INSERT INTO account_asset (addr, assetid, amount, frozen, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, $5, 0)"
	_, err = db.db.Exec(query, test.AccountA[:], 1, 2, false, false)
	require.NoError(t, err)
	_, err = db.db.Exec(query, test.AccountA[:], 3, 4, true, false)
	require.NoError(t, err)
	_, err = db.db.Exec(query, test.AccountA[:], 5, 6, true, true) // deleted
	require.NoError(t, err)
	_, err = db.db.Exec(query, test.AccountB[:], 5, 6, true, false) // wrong account
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	accountDataRet, _, err := l.LookupWithoutRewards(0, test.AccountA)
	require.NoError(t, err)

	accountDataExpected := basics.AccountData{
		AssetParams: make(map[basics.AssetIndex]basics.AssetParams),
		Assets: map[basics.AssetIndex]basics.AssetHolding{
			1: {
				Amount: 2,
				Frozen: false,
			},
			3: {
				Amount: 4,
				Frozen: true,
			},
		},
		AppLocalStates: make(map[basics.AppIndex]basics.AppLocalState),
		AppParams:      make(map[basics.AppIndex]basics.AppParams),
	}
	assert.Equal(t, accountDataExpected, accountDataRet)
}

func TestLedgerForEvaluatorAssetTable(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	_, err = db.db.Exec(query, test.AccountA[:])
	require.NoError(t, err)

	query =
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, 0)"

	_, err = db.db.Exec(
		query, 1, test.AccountA[:],
		encoding.EncodeAssetParams(basics.AssetParams{Manager: test.AccountB}),
		false)
	require.NoError(t, err)

	_, err = db.db.Exec(
		query, 2, test.AccountA[:],
		encoding.EncodeAssetParams(basics.AssetParams{Manager: test.AccountC}),
		false)
	require.NoError(t, err)

	_, err = db.db.Exec(query, 3, test.AccountA[:], "{}", true) // deleted
	require.NoError(t, err)

	_, err = db.db.Exec(query, 4, test.AccountD[:], "{}", false) // wrong account
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	accountDataRet, _, err := l.LookupWithoutRewards(0, test.AccountA)
	require.NoError(t, err)

	accountDataExpected := basics.AccountData{
		AssetParams: map[basics.AssetIndex]basics.AssetParams{
			1: {
				Manager: test.AccountB,
			},
			2: {
				Manager: test.AccountC,
			},
		},
		Assets:         make(map[basics.AssetIndex]basics.AssetHolding),
		AppLocalStates: make(map[basics.AppIndex]basics.AppLocalState),
		AppParams:      make(map[basics.AppIndex]basics.AppParams),
	}
	assert.Equal(t, accountDataExpected, accountDataRet)
}

func TestLedgerForEvaluatorAppTable(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	_, err = db.db.Exec(query, test.AccountA[:])
	require.NoError(t, err)

	query =
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, 0)"

	params1 := basics.AppParams{
		GlobalState: map[string]basics.TealValue{
			string([]byte{0xff}): {}, // try a non-utf8 string
		},
	}
	_, err = db.db.Exec(
		query, 1, test.AccountA[:], encoding.EncodeAppParams(params1), false)
	require.NoError(t, err)

	params2 := basics.AppParams{
		ApprovalProgram: []byte{1, 2, 3},
	}
	_, err = db.db.Exec(
		query, 2, test.AccountA[:], encoding.EncodeAppParams(params2), false)
	require.NoError(t, err)

	_, err = db.db.Exec(query, 3, test.AccountA[:], "{}", true) // deteled
	require.NoError(t, err)

	_, err = db.db.Exec(query, 4, test.AccountB[:], "{}", false) // wrong account
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	accountDataRet, _, err := l.LookupWithoutRewards(0, test.AccountA)
	require.NoError(t, err)

	accountDataExpected := basics.AccountData{
		AssetParams:    make(map[basics.AssetIndex]basics.AssetParams),
		Assets:         make(map[basics.AssetIndex]basics.AssetHolding),
		AppLocalStates: make(map[basics.AppIndex]basics.AppLocalState),
		AppParams: map[basics.AppIndex]basics.AppParams{
			1: params1,
			2: params2,
		},
	}
	assert.Equal(t, accountDataExpected, accountDataRet)
}

func TestLedgerForEvaluatorAccountAppTable(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	_, err = db.db.Exec(query, test.AccountA[:])
	require.NoError(t, err)

	query =
		"INSERT INTO account_app (addr, app, localstate, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, 0)"

	params1 := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			string([]byte{0xff}): {}, // try a non-utf8 string
		},
	}
	_, err = db.db.Exec(
		query, test.AccountA[:], 1, encoding.EncodeAppLocalState(params1), false)
	require.NoError(t, err)

	params2 := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			"abc": {},
		},
	}
	_, err = db.db.Exec(
		query, test.AccountA[:], 2, encoding.EncodeAppLocalState(params2), false)
	require.NoError(t, err)

	_, err = db.db.Exec(query, test.AccountA[:], 3, "{}", true) // deteled
	require.NoError(t, err)

	_, err = db.db.Exec(query, test.AccountB[:], 4, "{}", false) // wrong account
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	accountDataRet, _, err := l.LookupWithoutRewards(0, test.AccountA)
	require.NoError(t, err)

	accountDataExpected := basics.AccountData{
		AssetParams: make(map[basics.AssetIndex]basics.AssetParams),
		Assets:      make(map[basics.AssetIndex]basics.AssetHolding),
		AppLocalStates: map[basics.AppIndex]basics.AppLocalState{
			1: params1,
			2: params2,
		},
		AppParams: make(map[basics.AppIndex]basics.AppParams),
	}
	assert.Equal(t, accountDataExpected, accountDataRet)
}

func TestLedgerForEvaluatorAssetCreatorBasic(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', false, 0)"
	_, err = db.db.Exec(query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	address, ok, err := l.GetCreatorForRound(
		basics.Round(0), basics.CreatableIndex(2), basics.AssetCreatable)
	require.NoError(t, err)

	assert.True(t, ok)
	assert.Equal(t, test.AccountA, address)
}

func TestLedgerForEvaluatorAssetCreatorDeleted(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', true, 0)"
	_, err = db.db.Exec(query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	_, ok, err := l.GetCreatorForRound(
		basics.Round(0), basics.CreatableIndex(2), basics.AssetCreatable)
	require.NoError(t, err)

	assert.False(t, ok)
}

func TestLedgerForEvaluatorAppCreatorBasic(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', false, 0)"
	_, err = db.db.Exec(query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	address, ok, err := l.GetCreatorForRound(
		basics.Round(0), basics.CreatableIndex(2), basics.AppCreatable)
	require.NoError(t, err)

	assert.True(t, ok)
	assert.Equal(t, test.AccountA, address)
}

func TestLedgerForEvaluatorAppCreatorDeleted(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	query :=
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', true, 0)"
	_, err = db.db.Exec(query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	_, ok, err := l.GetCreatorForRound(
		basics.Round(0), basics.CreatableIndex(2), basics.AppCreatable)
	require.NoError(t, err)

	assert.False(t, ok)
}

func TestLedgerForEvaluatorGenesisHash(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	tx, err := db.db.BeginTx(context.Background(), &readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback()

	var genesisHash crypto.Digest
	genesisHash[0] = 3

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, genesisHash, basics.Address{})
	require.NoError(t, err)
	defer l.Close()

	genesisHashRet := l.GenesisHash()
	assert.Equal(t, genesisHash, genesisHashRet)
}
