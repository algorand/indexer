package ledgerforevaluator_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	ledger_for_evaluator "github.com/algorand/indexer/idb/postgres/internal/ledger_for_evaluator"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
	"github.com/algorand/indexer/util/test"
)

var readonlyRepeatableRead = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

func setupPostgres(t *testing.T) (*pgxpool.Pool, func()) {
	db, _, shutdownFunc := pgtest.SetupPostgres(t)

	_, err := db.Exec(context.Background(), schema.SetupPostgresSql)
	require.NoError(t, err)

	return db, shutdownFunc
}

func TestLedgerForEvaluatorBlockHdr(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO block_header (round, realtime, rewardslevel, header) " +
			"VALUES (2, 'epoch', 0, $1)"
	header := bookkeeping.BlockHeader{
		RewardsState: bookkeeping.RewardsState{
			FeeSink: test.FeeAddr,
		},
	}
	_, err := db.Exec(context.Background(), query, encoding.EncodeBlockHeader(header))
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, header.GenesisHash, transactions.SpecialAddresses{})
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.BlockHdr(basics.Round(2))
	require.NoError(t, err)

	assert.Equal(t, header, ret)
}

func TestLedgerForEvaluatorAccountTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

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

	_, err := db.Exec(
		context.Background(),
		query, test.AccountB[:], accountDataFull.MicroAlgos.Raw, accountDataFull.RewardsBase,
		accountDataFull.RewardedMicroAlgos.Raw,
		encoding.EncodeTrimmedAccountData(accountDataWritten))
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	checkFunc := func(preload bool) {
		l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
			tx, crypto.Digest{}, transactions.SpecialAddresses{})
		require.NoError(t, err)
		defer l.Close()

		if preload {
			err := l.PreloadAccounts(map[basics.Address]struct{}{test.AccountB: {}})
			require.NoError(t, err)
		}

		accountDataRet, round, err := l.LookupWithoutRewards(7, test.AccountB)
		require.NoError(t, err)

		assert.Equal(t, basics.Round(7), round)
		assert.Equal(t, accountDataFull, accountDataRet)
	}
	checkFunc(false)
	checkFunc(true)
}

func insertAccountData(db *pgxpool.Pool, account basics.Address, createdat uint64, deleted bool, data basics.AccountData) error {
	// This could be 'upsertAccountStmtName'
	query :=
		"INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted, " +
			"created_at, account_data) " +
			"VALUES ($1, $2, $3, $4, $5, $6, $7)"
	_, err := db.Exec(
		context.Background(), query,
		account[:], data.MicroAlgos.Raw, data.RewardsBase, data.RewardedMicroAlgos.Raw, deleted, createdat,
		encoding.EncodeTrimmedAccountData(data))
	return err
}

// TestLedgerForEvaluatorAccountTableBasicSingleAccount a table driven single account test.
func TestLedgerForEvaluatorAccountTableSingleAccount(t *testing.T) {
	tests := []struct {
		name      string
		createdAt uint64
		deleted   bool
		data      basics.AccountData
		err       string
	}{
		{
			name: "small balance",
			data: basics.AccountData{
				MicroAlgos: basics.MicroAlgos{Raw: 1},
			},
		},
		{
			name: "max balance",
			data: basics.AccountData{
				MicroAlgos: basics.MicroAlgos{Raw: math.MaxInt64},
			},
		},
		{
			name: "over max balance",
			data: basics.AccountData{
				MicroAlgos: basics.MicroAlgos{Raw: math.MaxUint64},
			},
			err: fmt.Sprintf("%d is greater than maximum value for Int8", uint64(math.MaxUint64)),
		},
		{
			name:    "deleted",
			deleted: true,
			data: basics.AccountData{
				MicroAlgos: basics.MicroAlgos{Raw: math.MaxInt64},
			},
		},
	}

	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	for i, testcase := range tests {
		tc := testcase
		var addr basics.Address
		addr[0] = byte(i + 1)
		t.Run(tc.name, func(t *testing.T) {
			// when returns true, exit test
			checkError := func(err error) bool {
				if err != nil && tc.err != "" {
					require.Contains(t, err.Error(), tc.err)
					return true
				}
				require.NoError(t, err)
				return false
			}
			// Add empty maps
			if tc.data.AssetParams == nil {
				tc.data.AssetParams = make(map[basics.AssetIndex]basics.AssetParams)
			}
			if tc.data.Assets == nil {
				tc.data.Assets = make(map[basics.AssetIndex]basics.AssetHolding)
			}
			if tc.data.AppLocalStates == nil {
				tc.data.AppLocalStates = make(map[basics.AppIndex]basics.AppLocalState)
			}
			if tc.data.AppParams == nil {
				tc.data.AppParams = make(map[basics.AppIndex]basics.AppParams)
			}

			err := insertAccountData(db, addr, tc.createdAt, tc.deleted, tc.data)
			if checkError(err) {
				return
			}

			require.NoError(t, err)

			tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
			if checkError(err) {
				return
			}
			require.NoError(t, err)
			defer tx.Rollback(context.Background())

			checkFunc := func(preload bool) {
				l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
					tx, crypto.Digest{}, transactions.SpecialAddresses{})
				if checkError(err) {
					return
				}
				require.NoError(t, err)
				defer l.Close()

				if preload {
					err := l.PreloadAccounts(map[basics.Address]struct{}{addr: {}})
					if checkError(err) {
						return
					}
					require.NoError(t, err)
				}

				accountDataRet, round, err := l.LookupWithoutRewards(7, addr)
				if checkError(err) {
					return
				}
				require.NoError(t, err)

				assert.Equal(t, basics.Round(7), round)
				// should be no result if deleted
				if tc.deleted {
					assert.Equal(t, basics.AccountData{}, accountDataRet)
				} else {
					assert.Equal(t, tc.data, accountDataRet)
				}
			}
			checkFunc(false)
			checkFunc(true)
		})
	}
}

func TestLedgerForEvaluatorAccountTableDeleted(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted, " +
			"created_at, account_data) " +
			"VALUES ($1, 2, 3, 4, true, 0, $2)"

	accountData := basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 5},
	}
	_, err := db.Exec(
		context.Background(), query, test.AccountB[:],
		encoding.EncodeTrimmedAccountData(accountData))
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	checkFunc := func(preload bool) {
		l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
			tx, crypto.Digest{}, transactions.SpecialAddresses{})
		require.NoError(t, err)
		defer l.Close()

		if preload {
			err := l.PreloadAccounts(map[basics.Address]struct{}{test.AccountB: {}})
			require.NoError(t, err)
		}

		accountDataRet, round, err := l.LookupWithoutRewards(7, test.AccountB)
		require.NoError(t, err)

		assert.Equal(t, basics.Round(7), round)
		assert.Equal(t, basics.AccountData{}, accountDataRet)
	}
	checkFunc(false)
	checkFunc(true)
}

func TestLedgerForEvaluatorAccountTableMissingAccount(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	checkFunc := func(preload bool) {
		l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
			tx, crypto.Digest{}, transactions.SpecialAddresses{})
		require.NoError(t, err)
		defer l.Close()

		if preload {
			err := l.PreloadAccounts(map[basics.Address]struct{}{test.AccountB: {}})
			require.NoError(t, err)
		}

		accountDataRet, round, err := l.LookupWithoutRewards(7, test.AccountB)
		require.NoError(t, err)

		assert.Equal(t, basics.Round(7), round)
		assert.Equal(t, basics.AccountData{}, accountDataRet)
	}
	checkFunc(false)
	checkFunc(true)
}

func TestLedgerForEvaluatorAccountTableNullAccountData(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

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
	_, err := db.Exec(
		context.Background(), query, test.AccountA[:], accountDataFull.MicroAlgos.Raw)
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
	require.NoError(t, err)
	defer l.Close()

	accountDataRet, _, err := l.LookupWithoutRewards(0, test.AccountA)
	require.NoError(t, err)

	assert.Equal(t, accountDataFull, accountDataRet)
}

func TestLedgerForEvaluatorAccountAssetTable(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	query =
		"INSERT INTO account_asset (addr, assetid, amount, frozen, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, $5, 0)"
	_, err = db.Exec(context.Background(), query, test.AccountA[:], 1, 2, false, false)
	require.NoError(t, err)
	_, err = db.Exec(context.Background(), query, test.AccountA[:], 3, 4, true, false)
	require.NoError(t, err)
	_, err = db.Exec(context.Background(), query, test.AccountA[:], 5, 6, true, true) // deleted
	require.NoError(t, err)
	_, err = db.Exec(context.Background(), query, test.AccountB[:], 5, 6, true, false) // wrong account
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
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
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	query =
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, 0)"

	_, err = db.Exec(
		context.Background(), query, 1, test.AccountA[:],
		encoding.EncodeAssetParams(basics.AssetParams{Manager: test.AccountB}),
		false)
	require.NoError(t, err)

	_, err = db.Exec(
		context.Background(), query, 2, test.AccountA[:],
		encoding.EncodeAssetParams(basics.AssetParams{Manager: test.AccountC}),
		false)
	require.NoError(t, err)

	_, err = db.Exec(context.Background(), query, 3, test.AccountA[:], "{}", true) // deleted
	require.NoError(t, err)

	_, err = db.Exec(context.Background(), query, 4, test.AccountD[:], "{}", false) // wrong account
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
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
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	query =
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, 0)"

	params1 := basics.AppParams{
		GlobalState: map[string]basics.TealValue{
			string([]byte{0xff}): {}, // try a non-utf8 string
		},
	}
	_, err = db.Exec(
		context.Background(), query, 1, test.AccountA[:],
		encoding.EncodeAppParams(params1), false)
	require.NoError(t, err)

	params2 := basics.AppParams{
		ApprovalProgram: []byte{1, 2, 3},
	}
	_, err = db.Exec(
		context.Background(), query, 2, test.AccountA[:],
		encoding.EncodeAppParams(params2), false)
	require.NoError(t, err)

	_, err = db.Exec(
		context.Background(), query, 3, test.AccountA[:], "{}", true) // deteled
	require.NoError(t, err)

	_, err = db.Exec(
		context.Background(), query, 4, test.AccountB[:], "{}", false) // wrong account
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
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
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	query =
		"INSERT INTO account_app (addr, app, localstate, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, 0)"

	params1 := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			string([]byte{0xff}): {}, // try a non-utf8 string
		},
	}
	_, err = db.Exec(
		context.Background(), query, test.AccountA[:], 1,
		encoding.EncodeAppLocalState(params1), false)
	require.NoError(t, err)

	params2 := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			"abc": {},
		},
	}
	_, err = db.Exec(
		context.Background(), query, test.AccountA[:], 2,
		encoding.EncodeAppLocalState(params2), false)
	require.NoError(t, err)

	_, err = db.Exec(
		context.Background(), query, test.AccountA[:], 3, "{}", true) // deteled
	require.NoError(t, err)

	_, err = db.Exec(
		context.Background(), query, test.AccountB[:], 4, "{}", false) // wrong account
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
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

// Tests that queuing and reading from a batch when using PreloadAccounts()
// is in the same order.
func TestLedgerForEvaluatorLookupMultipleAccounts(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	addAccountQuery :=
		"INSERT INTO account " +
			"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at) " +
			"VALUES ($1, 0, 0, 0, false, 0)"
	addAccountAssetQuery :=
		"INSERT INTO account_asset (addr, assetid, amount, frozen, deleted, created_at) " +
			"VALUES ($1, $2, 0, false, false, 0)"
	addAssetQuery :=
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES ($1, $2, '{}', false, 0)"
	addAppQuery :=
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES ($1, $2, '{}', false, 0)"
	addAccountAppQuery :=
		"INSERT INTO account_app (addr, app, localstate, deleted, created_at) " +
			"VALUES ($1, $2, '{}', false, 0)"

	addresses := []basics.Address{
		test.AccountA, test.AccountB, test.AccountC, test.AccountD, test.AccountE}
	seq := []int{4, 9, 3, 6, 5, 1}

	for i, address := range addresses {
		_, err := db.Exec(context.Background(), addAccountQuery, address[:])
		require.NoError(t, err)

		// Insert all types of creatables. Note that no creatable id is ever repeated.
		for j := range seq {
			_, err = db.Exec(
				context.Background(), addAccountAssetQuery, address[:], i+10*j+100)
			require.NoError(t, err)

			_, err = db.Exec(
				context.Background(), addAssetQuery, i+10*j+200, address[:])
			require.NoError(t, err)

			_, err = db.Exec(
				context.Background(), addAppQuery, i+10*j+300, address[:])
			require.NoError(t, err)

			_, err = db.Exec(
				context.Background(), addAccountAppQuery, address[:], i+10*j+400)
			require.NoError(t, err)
		}
	}

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	specialAddresses := transactions.SpecialAddresses{
		FeeSink:     test.FeeAddr,
		RewardsPool: test.RewardAddr,
	}
	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, specialAddresses)
	require.NoError(t, err)
	defer l.Close()

	// Preload accounts so that batching is actually used.
	{
		addressesMap := make(map[basics.Address]struct{})
		for _, address := range addresses {
			addressesMap[address] = struct{}{}
		}
		// Add special accounts.
		addressesMap[test.FeeAddr] = struct{}{}
		addressesMap[test.RewardAddr] = struct{}{}

		err := l.PreloadAccounts(addressesMap)
		require.NoError(t, err)
	}

	for i, address := range addresses {
		accountData, _, err := l.LookupWithoutRewards(0, address)
		require.NoError(t, err)

		assert.Equal(t, len(seq), len(accountData.Assets))
		assert.Equal(t, len(seq), len(accountData.AssetParams))
		assert.Equal(t, len(seq), len(accountData.AppParams))
		assert.Equal(t, len(seq), len(accountData.AppLocalStates))

		for j := range seq {
			_, ok := accountData.Assets[basics.AssetIndex(i+10*j+100)]
			assert.True(t, ok)

			_, ok = accountData.AssetParams[basics.AssetIndex(i+10*j+200)]
			assert.True(t, ok)

			_, ok = accountData.AppParams[basics.AppIndex(i+10*j+300)]
			assert.True(t, ok)

			_, ok = accountData.AppLocalStates[basics.AppIndex(i+10*j+400)]
			assert.True(t, ok)
		}
	}

	// Read special accounts.
	for _, address := range []basics.Address{test.FeeAddr, test.RewardAddr} {
		accountData, _, err := l.LookupWithoutRewards(0, address)
		require.NoError(t, err)

		expected := basics.MicroAlgos{Raw: 1000 * 1000 * 1000 * 1000 * 1000}
		assert.Equal(t, expected, accountData.MicroAlgos)
	}
}

func TestLedgerForEvaluatorAssetCreatorBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', false, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
	require.NoError(t, err)
	defer l.Close()

	address, ok, err := l.GetCreatorForRound(
		basics.Round(0), basics.CreatableIndex(2), basics.AssetCreatable)
	require.NoError(t, err)

	assert.True(t, ok)
	assert.Equal(t, test.AccountA, address)
}

func TestLedgerForEvaluatorAssetCreatorDeleted(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', true, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
	require.NoError(t, err)
	defer l.Close()

	_, ok, err := l.GetCreatorForRound(
		basics.Round(0), basics.CreatableIndex(2), basics.AssetCreatable)
	require.NoError(t, err)

	assert.False(t, ok)
}

func TestLedgerForEvaluatorAppCreatorBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', false, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
	require.NoError(t, err)
	defer l.Close()

	address, ok, err := l.GetCreatorForRound(
		basics.Round(0), basics.CreatableIndex(2), basics.AppCreatable)
	require.NoError(t, err)

	assert.True(t, ok)
	assert.Equal(t, test.AccountA, address)
}

func TestLedgerForEvaluatorAppCreatorDeleted(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', true, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, crypto.Digest{}, transactions.SpecialAddresses{})
	require.NoError(t, err)
	defer l.Close()

	_, ok, err := l.GetCreatorForRound(
		basics.Round(0), basics.CreatableIndex(2), basics.AppCreatable)
	require.NoError(t, err)

	assert.False(t, ok)
}

func TestLedgerForEvaluatorSpecialAddresses(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	specialAddresses := transactions.SpecialAddresses{
		FeeSink:     test.FeeAddr,
		RewardsPool: test.RewardAddr,
	}
	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, test.GenesisHash, specialAddresses)
	require.NoError(t, err)
	defer l.Close()

	amount := basics.MicroAlgos{Raw: 1000 * 1000 * 1000 * 1000 * 1000}

	accountData, round, err := l.LookupWithoutRewards(basics.Round(5), test.FeeAddr)
	require.NoError(t, err)
	assert.Equal(t, amount, accountData.MicroAlgos)
	assert.Equal(t, basics.Round(5), round)

	accountData, round, err = l.LookupWithoutRewards(basics.Round(5), test.RewardAddr)
	require.NoError(t, err)
	assert.Equal(t, amount, accountData.MicroAlgos)
	assert.Equal(t, basics.Round(5), round)
}

func TestLedgerForEvaluatorGenesisHash(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeLedgerForEvaluator(
		tx, test.GenesisHash, transactions.SpecialAddresses{})
	require.NoError(t, err)
	defer l.Close()

	genesisHash := l.GenesisHash()
	assert.Equal(t, test.GenesisHash, genesisHash)
}
