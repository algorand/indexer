package ledgerforevaluator_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/crypto/merklesignature"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	ledger_for_evaluator "github.com/algorand/indexer/idb/postgres/internal/ledger_for_evaluator"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
	pgutil "github.com/algorand/indexer/idb/postgres/internal/util"
	"github.com/algorand/indexer/util/test"
)

var readonlyRepeatableRead = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

func TestLedgerForEvaluatorLatestBlockHdr(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
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

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(2))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.LatestBlockHdr()
	require.NoError(t, err)

	assert.Equal(t, header, ret)
}

func TestLedgerForEvaluatorAccountTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	query := `INSERT INTO account
		(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, account_data)
		VALUES ($1, $2, $3, $4, false, 0, $5)`

	var voteID crypto.OneTimeSignatureVerifier
	voteID[0] = 2
	var selectionID crypto.VRFVerifier
	selectionID[0] = 3
	var stateProofID merklesignature.Verifier
	stateProofID[0] = 10
	accountDataFull := ledgercore.AccountData{
		AccountBaseData: ledgercore.AccountBaseData{
			Status:             basics.Online,
			MicroAlgos:         basics.MicroAlgos{Raw: 4},
			RewardsBase:        5,
			RewardedMicroAlgos: basics.MicroAlgos{Raw: 6},
			AuthAddr:           test.AccountA,
		},
		VotingData: ledgercore.VotingData{
			VoteID:          voteID,
			SelectionID:     selectionID,
			StateProofID:    stateProofID,
			VoteFirstValid:  basics.Round(7),
			VoteLastValid:   basics.Round(8),
			VoteKeyDilution: 9,
		},
	}

	accountDataWritten := encoding.TrimLcAccountData(accountDataFull)

	_, err := db.Exec(
		context.Background(),
		query, test.AccountB[:], accountDataFull.MicroAlgos.Raw, accountDataFull.RewardsBase,
		accountDataFull.RewardedMicroAlgos.Raw,
		encoding.EncodeTrimmedLcAccountData(accountDataWritten))
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err :=
		l.LookupWithoutRewards(map[basics.Address]struct{}{test.AccountB: {}})
	require.NoError(t, err)

	accountDataRet := ret[test.AccountB]

	require.NotNil(t, accountDataRet)
	assert.Equal(t, accountDataFull, *accountDataRet)
}

func insertDeletedAccount(db *pgxpool.Pool, address basics.Address) error {
	query := `INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted,
		created_at, account_data)
		VALUES ($1, 0, 0, 0, true, 0, 'null'::jsonb)`

	_, err := db.Exec(
		context.Background(), query, address[:])
	return err
}

func insertAccount(db *pgxpool.Pool, address basics.Address, data ledgercore.AccountData) error {
	query := `INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted,
		created_at, account_data)
		VALUES ($1, $2, $3, $4, false, 0, $5)`

	_, err := db.Exec(
		context.Background(), query,
		address[:], data.MicroAlgos.Raw, data.RewardsBase, data.RewardedMicroAlgos.Raw,
		encoding.EncodeTrimmedLcAccountData(data))
	return err
}

// TestLedgerForEvaluatorAccountTableBasicSingleAccount a table driven single account test.
func TestLedgerForEvaluatorAccountTableSingleAccount(t *testing.T) {
	tests := []struct {
		name    string
		deleted bool
		data    ledgercore.AccountData
		err     string
	}{
		{
			name: "small balance",
			data: ledgercore.AccountData{
				AccountBaseData: ledgercore.AccountBaseData{
					MicroAlgos: basics.MicroAlgos{Raw: 1},
				},
			},
		},
		{
			name: "max balance",
			data: ledgercore.AccountData{
				AccountBaseData: ledgercore.AccountBaseData{
					MicroAlgos: basics.MicroAlgos{Raw: math.MaxInt64},
				},
			},
		},
		{
			name: "over max balance",
			data: ledgercore.AccountData{
				AccountBaseData: ledgercore.AccountBaseData{
					MicroAlgos: basics.MicroAlgos{Raw: math.MaxUint64},
				},
			},
			err: fmt.Sprintf(
				"%d is greater than maximum value for Int8", uint64(math.MaxUint64)),
		},
		{
			name:    "deleted",
			deleted: true,
		},
	}

	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
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

			var err error
			if tc.deleted {
				err = insertDeletedAccount(db, addr)
			} else {
				err = insertAccount(db, addr, tc.data)
			}
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

			l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
			if checkError(err) {
				return
			}
			require.NoError(t, err)
			defer l.Close()

			ret, err := l.LookupWithoutRewards(map[basics.Address]struct{}{addr: {}})
			if checkError(err) {
				return
			}
			require.NoError(t, err)

			accountDataRet, ok := ret[addr]
			require.True(t, ok)

			// should be no result if deleted
			if tc.deleted {
				assert.Nil(t, accountDataRet)
			} else {
				assert.Equal(t, &tc.data, accountDataRet)
			}
		})
	}
}

func TestLedgerForEvaluatorAccountTableDeleted(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted, " +
			"created_at, account_data) " +
			"VALUES ($1, 2, 3, 4, true, 0, $2)"

	accountData := ledgercore.AccountData{
		AccountBaseData: ledgercore.AccountBaseData{
			MicroAlgos: basics.MicroAlgos{Raw: 5},
		},
	}
	_, err := db.Exec(
		context.Background(), query, test.AccountB[:],
		encoding.EncodeTrimmedLcAccountData(accountData))
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err :=
		l.LookupWithoutRewards(map[basics.Address]struct{}{test.AccountB: {}})
	require.NoError(t, err)

	accountDataRet := ret[test.AccountB]
	assert.Nil(t, accountDataRet)
}

func TestLedgerForEvaluatorAccountTableMissingAccount(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err :=
		l.LookupWithoutRewards(map[basics.Address]struct{}{test.AccountB: {}})
	require.NoError(t, err)

	accountDataRet := ret[test.AccountB]
	assert.Nil(t, accountDataRet)
}

func insertDeletedAccountAsset(t *testing.T, db *pgxpool.Pool, addr basics.Address, assetid uint64) {
	query :=
		"INSERT INTO account_asset (addr, assetid, amount, frozen, deleted, created_at) " +
			"VALUES ($1, $2, 0, false, true, 0)"

	_, err := db.Exec(
		context.Background(), query, addr[:], assetid)
	require.NoError(t, err)
}

func insertAccountAsset(t *testing.T, db *pgxpool.Pool, addr basics.Address, assetid uint64, amount uint64, frozen bool) {
	query :=
		"INSERT INTO account_asset (addr, assetid, amount, frozen, deleted, created_at) " +
			"VALUES ($1, $2, $3, $4, false, 0)"

	_, err := db.Exec(
		context.Background(), query, addr[:], assetid, amount, frozen)
	require.NoError(t, err)
}

func TestLedgerForEvaluatorAccountAssetTable(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	insertAccountAsset(t, db, test.AccountA, 1, 2, false)
	insertAccountAsset(t, db, test.AccountA, 3, 4, true)
	insertDeletedAccountAsset(t, db, test.AccountA, 5)
	insertAccountAsset(t, db, test.AccountB, 5, 6, true)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.LookupResources(map[basics.Address]map[ledger.Creatable]struct{}{
		test.AccountA: {
			{Index: 1, Type: basics.AssetCreatable}: {},
			{Index: 3, Type: basics.AssetCreatable}: {},
			{Index: 5, Type: basics.AssetCreatable}: {},
			{Index: 8, Type: basics.AssetCreatable}: {},
		},
		test.AccountB: {
			{Index: 5, Type: basics.AssetCreatable}: {},
		},
	})
	require.NoError(t, err)

	expected := map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource{
		test.AccountA: {
			ledger.Creatable{Index: 1, Type: basics.AssetCreatable}: {
				AssetHolding: &basics.AssetHolding{
					Amount: 2,
					Frozen: false,
				},
			},
			ledger.Creatable{Index: 3, Type: basics.AssetCreatable}: {
				AssetHolding: &basics.AssetHolding{
					Amount: 4,
					Frozen: true,
				},
			},
			ledger.Creatable{Index: 5, Type: basics.AssetCreatable}: {},
			ledger.Creatable{Index: 8, Type: basics.AssetCreatable}: {},
		},
		test.AccountB: {
			ledger.Creatable{Index: 5, Type: basics.AssetCreatable}: {
				AssetHolding: &basics.AssetHolding{
					Amount: 6,
					Frozen: true,
				},
			},
		},
	}
	assert.Equal(t, expected, ret)
}

func insertDeletedAsset(t *testing.T, db *pgxpool.Pool, index uint64, creatorAddr basics.Address) {
	query := `INSERT INTO asset (index, creator_addr, params, deleted, created_at)
		VALUES ($1, $2, 'null'::jsonb, true, 0)`

	_, err := db.Exec(
		context.Background(), query, index, creatorAddr[:])
	require.NoError(t, err)
}

func insertAsset(t *testing.T, db *pgxpool.Pool, index uint64, creatorAddr basics.Address, params *basics.AssetParams) {
	query := `INSERT INTO asset (index, creator_addr, params, deleted, created_at)
		VALUES ($1, $2, $3, false, 0)`

	_, err := db.Exec(
		context.Background(), query, index, creatorAddr[:],
		encoding.EncodeAssetParams(*params))
	require.NoError(t, err)
}

func TestLedgerForEvaluatorAssetTable(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	insertAsset(t, db, 1, test.AccountA, &basics.AssetParams{Manager: test.AccountB})
	insertAsset(t, db, 2, test.AccountA, &basics.AssetParams{Total: 10})
	insertDeletedAsset(t, db, 3, test.AccountA)
	insertAsset(t, db, 4, test.AccountC, &basics.AssetParams{Total: 12})
	insertAsset(t, db, 5, test.AccountD, &basics.AssetParams{Total: 13})

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.LookupResources(map[basics.Address]map[ledger.Creatable]struct{}{
		test.AccountA: {
			{Index: 1, Type: basics.AssetCreatable}: {},
			{Index: 2, Type: basics.AssetCreatable}: {},
			{Index: 3, Type: basics.AssetCreatable}: {},
			{Index: 4, Type: basics.AssetCreatable}: {},
			{Index: 6, Type: basics.AssetCreatable}: {},
		},
		test.AccountD: {
			{Index: 5, Type: basics.AssetCreatable}: {},
		},
	})
	require.NoError(t, err)

	expected := map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource{
		test.AccountA: {
			ledger.Creatable{Index: 1, Type: basics.AssetCreatable}: {
				AssetParams: &basics.AssetParams{
					Manager: test.AccountB,
				},
			},
			ledger.Creatable{Index: 2, Type: basics.AssetCreatable}: {
				AssetParams: &basics.AssetParams{
					Total: 10,
				},
			},
			ledger.Creatable{Index: 3, Type: basics.AssetCreatable}: {},
			ledger.Creatable{Index: 4, Type: basics.AssetCreatable}: {},
			ledger.Creatable{Index: 6, Type: basics.AssetCreatable}: {},
		},
		test.AccountD: {
			ledger.Creatable{Index: 5, Type: basics.AssetCreatable}: {
				AssetParams: &basics.AssetParams{
					Total: 13,
				},
			},
		},
	}
	assert.Equal(t, expected, ret)
}

func insertDeletedApp(t *testing.T, db *pgxpool.Pool, index uint64, creator basics.Address) {
	query := `INSERT INTO app (index, creator, params, deleted, created_at)
		VALUES ($1, $2, 'null'::jsonb, true, 0)`

	_, err := db.Exec(context.Background(), query, index, creator[:])
	require.NoError(t, err)
}

func insertApp(t *testing.T, db *pgxpool.Pool, index uint64, creator basics.Address, params *basics.AppParams) {
	query := `INSERT INTO app (index, creator, params, deleted, created_at)
		VALUES ($1, $2, $3, false, 0)`

	_, err := db.Exec(
		context.Background(), query, index, creator[:], encoding.EncodeAppParams(*params))
	require.NoError(t, err)
}

func TestLedgerForEvaluatorAppTable(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	params1 := basics.AppParams{
		GlobalState: map[string]basics.TealValue{
			string([]byte{0xff}): {}, // try a non-utf8 string
		},
	}
	insertApp(t, db, 1, test.AccountA, &params1)

	params2 := basics.AppParams{
		ApprovalProgram: []byte{1, 2, 3, 10},
	}
	insertApp(t, db, 2, test.AccountA, &params2)

	insertDeletedApp(t, db, 3, test.AccountA)

	params4 := basics.AppParams{
		ApprovalProgram: []byte{1, 2, 3, 12},
	}
	insertApp(t, db, 4, test.AccountB, &params4)

	params5 := basics.AppParams{
		ApprovalProgram: []byte{1, 2, 3, 13},
	}
	insertApp(t, db, 5, test.AccountC, &params5)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.LookupResources(map[basics.Address]map[ledger.Creatable]struct{}{
		test.AccountA: {
			{Index: 1, Type: basics.AppCreatable}: {},
			{Index: 2, Type: basics.AppCreatable}: {},
			{Index: 3, Type: basics.AppCreatable}: {},
			{Index: 4, Type: basics.AppCreatable}: {},
			{Index: 6, Type: basics.AppCreatable}: {},
		},
		test.AccountC: {
			{Index: 5, Type: basics.AppCreatable}: {},
		},
	})
	require.NoError(t, err)

	expected := map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource{
		test.AccountA: {
			ledger.Creatable{Index: 1, Type: basics.AppCreatable}: {
				AppParams: &params1,
			},
			ledger.Creatable{Index: 2, Type: basics.AppCreatable}: {
				AppParams: &params2,
			},
			ledger.Creatable{Index: 3, Type: basics.AppCreatable}: {},
			ledger.Creatable{Index: 4, Type: basics.AppCreatable}: {},
			ledger.Creatable{Index: 6, Type: basics.AppCreatable}: {},
		},
		test.AccountC: {
			ledger.Creatable{Index: 5, Type: basics.AppCreatable}: {
				AppParams: &params5,
			},
		},
	}
	assert.Equal(t, expected, ret)
}

func insertDeletedAccountApp(t *testing.T, db *pgxpool.Pool, addr basics.Address, app uint64) {
	query := `INSERT INTO account_app (addr, app, localstate, deleted, created_at)
		VALUES ($1, $2, 'null'::jsonb, true, 0)`

	_, err := db.Exec(
		context.Background(), query, addr[:], app)
	require.NoError(t, err)
}

func insertAccountApp(t *testing.T, db *pgxpool.Pool, addr basics.Address, app uint64, localstate *basics.AppLocalState) {
	query := `INSERT INTO account_app (addr, app, localstate, deleted, created_at)
		VALUES ($1, $2, $3, false, 0)`

	_, err := db.Exec(
		context.Background(), query, addr[:], app,
		encoding.EncodeAppLocalState(*localstate))
	require.NoError(t, err)
}

func TestLedgerForEvaluatorAccountAppTable(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	stateA1 := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			string([]byte{0xff}): {}, // try a non-utf8 string
		},
	}
	insertAccountApp(t, db, test.AccountA, 1, &stateA1)

	stateA2 := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			"abc": {},
		},
	}
	insertAccountApp(t, db, test.AccountA, 2, &stateA2)

	insertDeletedAccountApp(t, db, test.AccountA, 3)

	stateB3 := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			"abf": {},
		},
	}
	insertAccountApp(t, db, test.AccountB, 3, &stateB3)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.LookupResources(map[basics.Address]map[ledger.Creatable]struct{}{
		test.AccountA: {
			{Index: 1, Type: basics.AppCreatable}: {},
			{Index: 2, Type: basics.AppCreatable}: {},
			{Index: 3, Type: basics.AppCreatable}: {},
			{Index: 4, Type: basics.AppCreatable}: {},
		},
		test.AccountB: {
			{Index: 3, Type: basics.AppCreatable}: {},
		},
	})
	require.NoError(t, err)

	expected := map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource{
		test.AccountA: {
			ledger.Creatable{Index: 1, Type: basics.AppCreatable}: {
				AppLocalState: &stateA1,
			},
			ledger.Creatable{Index: 2, Type: basics.AppCreatable}: {
				AppLocalState: &stateA2,
			},
			ledger.Creatable{Index: 3, Type: basics.AppCreatable}: {},
			ledger.Creatable{Index: 4, Type: basics.AppCreatable}: {},
		},
		test.AccountB: {
			ledger.Creatable{Index: 3, Type: basics.AppCreatable}: {
				AppLocalState: &stateB3,
			},
		},
	}
	assert.Equal(t, expected, ret)
}

func TestLedgerForEvaluatorFetchAllResourceTypes(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	insertAccountAsset(t, db, test.AccountA, 1, 2, true)
	insertAsset(t, db, 1, test.AccountA, &basics.AssetParams{Total: 3})
	insertAccountApp(
		t, db, test.AccountA, 4,
		&basics.AppLocalState{Schema: basics.StateSchema{NumUint: 5}})
	insertApp(t, db, 4, test.AccountA, &basics.AppParams{ExtraProgramPages: 6})

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.LookupResources(map[basics.Address]map[ledger.Creatable]struct{}{
		test.AccountA: {
			{Index: 1, Type: basics.AssetCreatable}: {},
			{Index: 4, Type: basics.AppCreatable}:   {},
		},
	})
	require.NoError(t, err)

	expected := map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource{
		test.AccountA: {
			ledger.Creatable{Index: 1, Type: basics.AssetCreatable}: {
				AssetHolding: &basics.AssetHolding{
					Amount: 2,
					Frozen: true,
				},
				AssetParams: &basics.AssetParams{
					Total: 3,
				},
			},
			ledger.Creatable{Index: 4, Type: basics.AppCreatable}: {
				AppLocalState: &basics.AppLocalState{
					Schema: basics.StateSchema{
						NumUint: 5,
					},
				},
				AppParams: &basics.AppParams{
					ExtraProgramPages: 6,
				},
			},
		},
	}
	assert.Equal(t, expected, ret)
}

// Tests that queuing and reading from a batch is in the same order.
func TestLedgerForEvaluatorLookupMultipleAccounts(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	addAccountQuery := `INSERT INTO account
		(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, account_data)
		VALUES ($1, 0, 0, 0, false, 0, 'null'::jsonb)`

	addresses := []basics.Address{
		test.AccountA, test.AccountB, test.AccountC, test.AccountD, test.AccountE}

	for _, address := range addresses {
		_, err := db.Exec(context.Background(), addAccountQuery, address[:])
		require.NoError(t, err)
	}

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	addressesMap := make(map[basics.Address]struct{})
	for _, address := range addresses {
		addressesMap[address] = struct{}{}
	}
	addressesMap[test.FeeAddr] = struct{}{}
	addressesMap[test.RewardAddr] = struct{}{}

	ret, err := l.LookupWithoutRewards(addressesMap)
	require.NoError(t, err)

	for _, address := range addresses {
		accountData, _ := ret[address]
		require.NotNil(t, accountData)
	}
}

func TestLedgerForEvaluatorAssetCreatorBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', false, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.GetAssetCreator(
		map[basics.AssetIndex]struct{}{basics.AssetIndex(2): {}})
	require.NoError(t, err)

	foundAddress, ok := ret[basics.AssetIndex(2)]
	require.True(t, ok)

	expected := ledger.FoundAddress{
		Address: test.AccountA,
		Exists:  true,
	}
	assert.Equal(t, expected, foundAddress)
}

func TestLedgerForEvaluatorAssetCreatorDeleted(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', true, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.GetAssetCreator(
		map[basics.AssetIndex]struct{}{basics.AssetIndex(2): {}})
	require.NoError(t, err)

	foundAddress, ok := ret[basics.AssetIndex(2)]
	require.True(t, ok)

	assert.False(t, foundAddress.Exists)
}

func TestLedgerForEvaluatorAssetCreatorMultiple(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	creatorsMap := map[basics.AssetIndex]basics.Address{
		1: test.AccountA,
		2: test.AccountB,
		3: test.AccountC,
		4: test.AccountD,
		5: test.AccountE,
	}

	query :=
		"INSERT INTO asset (index, creator_addr, params, deleted, created_at) " +
			"VALUES ($1, $2, '{}', false, 0)"
	for index, address := range creatorsMap {
		_, err := db.Exec(context.Background(), query, index, address[:])
		require.NoError(t, err)
	}

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	indices := map[basics.AssetIndex]struct{}{
		1: {}, 2: {}, 3: {}, 4: {}, 5: {}, 6: {}, 7: {}, 8: {}}
	ret, err := l.GetAssetCreator(indices)
	require.NoError(t, err)

	assert.Equal(t, len(indices), len(ret))
	for i := 1; i <= 5; i++ {
		index := basics.AssetIndex(i)

		foundAddress, ok := ret[index]
		require.True(t, ok)

		expected := ledger.FoundAddress{
			Address: creatorsMap[index],
			Exists:  true,
		}
		assert.Equal(t, expected, foundAddress)
	}
	for i := 6; i <= 8; i++ {
		index := basics.AssetIndex(i)

		foundAddress, ok := ret[index]
		require.True(t, ok)

		assert.False(t, foundAddress.Exists)
	}
}

func TestLedgerForEvaluatorAppCreatorBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', false, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.GetAppCreator(
		map[basics.AppIndex]struct{}{basics.AppIndex(2): {}})
	require.NoError(t, err)

	foundAddress, ok := ret[basics.AppIndex(2)]
	require.True(t, ok)

	expected := ledger.FoundAddress{
		Address: test.AccountA,
		Exists:  true,
	}
	assert.Equal(t, expected, foundAddress)
}

func TestLedgerForEvaluatorAppCreatorDeleted(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	query :=
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES (2, $1, '{}', true, 0)"
	_, err := db.Exec(context.Background(), query, test.AccountA[:])
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	ret, err := l.GetAppCreator(
		map[basics.AppIndex]struct{}{basics.AppIndex(2): {}})
	require.NoError(t, err)

	foundAddress, ok := ret[basics.AppIndex(2)]
	require.True(t, ok)

	assert.False(t, foundAddress.Exists)
}

func TestLedgerForEvaluatorAppCreatorMultiple(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	creatorsMap := map[basics.AppIndex]basics.Address{
		1: test.AccountA,
		2: test.AccountB,
		3: test.AccountC,
		4: test.AccountD,
		5: test.AccountE,
	}

	query :=
		"INSERT INTO app (index, creator, params, deleted, created_at) " +
			"VALUES ($1, $2, '{}', false, 0)"
	for index, address := range creatorsMap {
		_, err := db.Exec(context.Background(), query, index, address[:])
		require.NoError(t, err)
	}

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	indices := map[basics.AppIndex]struct{}{
		1: {}, 2: {}, 3: {}, 4: {}, 5: {}, 6: {}, 7: {}, 8: {}}
	ret, err := l.GetAppCreator(indices)
	require.NoError(t, err)

	assert.Equal(t, len(indices), len(ret))
	for i := 1; i <= 5; i++ {
		index := basics.AppIndex(i)

		foundAddress, ok := ret[index]
		require.True(t, ok)

		expected := ledger.FoundAddress{
			Address: creatorsMap[index],
			Exists:  true,
		}
		assert.Equal(t, expected, foundAddress)
	}
	for i := 6; i <= 8; i++ {
		index := basics.AppIndex(i)

		foundAddress, ok := ret[index]
		require.True(t, ok)

		assert.False(t, foundAddress.Exists)
	}
}

func TestLedgerForEvaluatorAccountTotals(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	accountTotals := ledgercore.AccountTotals{
		Online: ledgercore.AlgoCount{
			Money: basics.MicroAlgos{Raw: 33},
		},
	}
	err := pgutil.SetMetastate(
		db, nil, schema.AccountTotals, string(encoding.EncodeAccountTotals(&accountTotals)))
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), readonlyRepeatableRead)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())

	l, err := ledger_for_evaluator.MakeDeprecatedLedgerForEvaluator(tx, basics.Round(0))
	require.NoError(t, err)
	defer l.Close()

	accountTotalsRead, err := l.LatestTotals()
	require.NoError(t, err)

	assert.Equal(t, accountTotals, accountTotalsRead)
}

// func TestLedgerForEvaluatorAppBox(t *testing.T) {
// 	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
// 	defer shutdownFunc()

// }
