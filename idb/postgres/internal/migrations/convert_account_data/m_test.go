package convertaccountdata_test

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/algorand/indexer/v3/idb/postgres/internal/encoding"
	cad "github.com/algorand/indexer/v3/idb/postgres/internal/migrations/convert_account_data"
	pgtest "github.com/algorand/indexer/v3/idb/postgres/internal/testing"
	pgutil "github.com/algorand/indexer/v3/idb/postgres/internal/util"
)

func makeAddress(i int) sdk.Address {
	var address sdk.Address
	address[0] = byte(i)
	return address
}

func insertAccount(t *testing.T, db *pgxpool.Pool, address sdk.Address, trimmedAccountData *sdk.AccountData) {
	query := `INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted,
		created_at, account_data) VALUES ($1, 0, 0, 0, false, 0, $2)`
	_, err := db.Exec(
		context.Background(), query, address[:],
		encoding.EncodeTrimmedLcAccountData(*trimmedAccountData))
	require.NoError(t, err)
}

func insertDeletedAccount(t *testing.T, db *pgxpool.Pool, address sdk.Address) {
	query := `INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, deleted,
		created_at, account_data) VALUES ($1, 0, 0, 0, true, 0, 'null'::jsonb)`
	_, err := db.Exec(context.Background(), query, address[:])
	require.NoError(t, err)
}

func checkAccount(t *testing.T, db *pgxpool.Pool, address sdk.Address, accountData *sdk.AccountData) {
	query := "SELECT account_data FROM account WHERE addr = $1"
	row := db.QueryRow(context.Background(), query, address[:])

	var buf []byte
	err := row.Scan(&buf)
	require.NoError(t, err)

	ret, err := encoding.DecodeTrimmedLcAccountData(buf)
	require.NoError(t, err)

	assert.Equal(t, accountData, &ret)
}

func checkDeletedAccount(t *testing.T, db *pgxpool.Pool, address sdk.Address) {
	query := "SELECT account_data FROM account WHERE addr = $1"
	row := db.QueryRow(context.Background(), query, address[:])

	var buf []byte
	err := row.Scan(&buf)
	require.NoError(t, err)

	assert.Equal(t, []byte("null"), buf)
}

func insertAccountAsset(t *testing.T, db *pgxpool.Pool, address sdk.Address, assetid uint64, deleted bool) {
	query := `INSERT INTO account_asset (addr, assetid, amount, frozen, deleted,
		created_at) VALUES ($1, $2, 0, false, $3, 0)`
	_, err := db.Exec(context.Background(), query, address[:], assetid, deleted)
	require.NoError(t, err)
}

func insertAsset(t *testing.T, db *pgxpool.Pool, assetid uint64, address sdk.Address, deleted bool) {
	query := `INSERT INTO asset (index, creator_addr, params, deleted, created_at)
		VALUES ($1, $2, 'null'::jsonb, $3, 0)`
	_, err := db.Exec(context.Background(), query, assetid, address[:], deleted)
	require.NoError(t, err)
}

func insertApp(t *testing.T, db *pgxpool.Pool, appid uint64, address sdk.Address, deleted bool) {
	query := `INSERT INTO app (index, creator, params, deleted, created_at)
		VALUES ($1, $2, 'null'::jsonb, $3, 0)`
	_, err := db.Exec(context.Background(), query, appid, address[:], deleted)
	require.NoError(t, err)
}

func insertAccountApp(t *testing.T, db *pgxpool.Pool, address sdk.Address, appid uint64, deleted bool) {
	query := `INSERT INTO account_app (addr, app, localstate, deleted, created_at)
		VALUES ($1, $2, 'null'::jsonb, $3, 0)`
	_, err := db.Exec(context.Background(), query, address[:], appid, deleted)
	require.NoError(t, err)
}

func TestBasic(t *testing.T) {
	for _, i := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
			defer shutdownFunc()

			insertAccount(t, db, makeAddress(1), &sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: 1}})
			insertDeletedAccount(t, db, makeAddress(2))
			insertAccount(t, db, makeAddress(3), &sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: 3}})

			f := func(tx pgx.Tx) error {
				return cad.RunMigration(tx, 1)
			}
			err := pgutil.TxWithRetry(db, pgx.TxOptions{IsoLevel: pgx.Serializable}, f, nil)
			require.NoError(t, err)

			checkAccount(
				t, db, makeAddress(1),
				&sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: 1}})
			checkDeletedAccount(t, db, makeAddress(2))
			checkAccount(
				t, db, makeAddress(3),
				&sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: 3}})
		})
	}
}

func TestAccountAssetCount(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	insertAccount(t, db, makeAddress(1), &sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: 1}})
	for i := uint64(2); i < 10; i++ {
		insertAccountAsset(t, db, makeAddress(1), i, i%2 == 0)
	}

	f := func(tx pgx.Tx) error {
		return cad.RunMigration(tx, 1)
	}
	err := pgutil.TxWithRetry(db, pgx.TxOptions{IsoLevel: pgx.Serializable}, f, nil)
	require.NoError(t, err)

	expected := sdk.AccountData{
		AccountBaseData: sdk.AccountBaseData{
			TotalAssets: 4,
		},
		VotingData: sdk.VotingData{
			VoteKeyDilution: 1,
		},
	}
	checkAccount(t, db, makeAddress(1), &expected)
}

func TestAssetCount(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	insertAccount(t, db, makeAddress(1), &sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: 1}})
	for i := uint64(2); i < 10; i++ {
		insertAsset(t, db, i, makeAddress(1), i%2 == 0)
	}

	f := func(tx pgx.Tx) error {
		return cad.RunMigration(tx, 1)
	}
	err := pgutil.TxWithRetry(db, pgx.TxOptions{IsoLevel: pgx.Serializable}, f, nil)
	require.NoError(t, err)

	expected := sdk.AccountData{
		AccountBaseData: sdk.AccountBaseData{
			TotalAssetParams: 4,
		},
		VotingData: sdk.VotingData{
			VoteKeyDilution: 1,
		},
	}
	checkAccount(t, db, makeAddress(1), &expected)
}

func TestAppCount(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	insertAccount(t, db, makeAddress(1), &sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: 1}})
	for i := uint64(2); i < 10; i++ {
		insertApp(t, db, i, makeAddress(1), i%2 == 0)
	}

	f := func(tx pgx.Tx) error {
		return cad.RunMigration(tx, 1)
	}
	err := pgutil.TxWithRetry(db, pgx.TxOptions{IsoLevel: pgx.Serializable}, f, nil)
	require.NoError(t, err)

	expected := sdk.AccountData{
		AccountBaseData: sdk.AccountBaseData{
			TotalAppParams: 4,
		},
		VotingData: sdk.VotingData{
			VoteKeyDilution: 1,
		},
	}
	checkAccount(t, db, makeAddress(1), &expected)
}

func TestAccountAppCount(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	insertAccount(t, db, makeAddress(1), &sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: 1}})
	for i := uint64(2); i < 10; i++ {
		insertAccountApp(t, db, makeAddress(1), i, i%2 == 0)
	}

	f := func(tx pgx.Tx) error {
		return cad.RunMigration(tx, 1)
	}
	err := pgutil.TxWithRetry(db, pgx.TxOptions{IsoLevel: pgx.Serializable}, f, nil)
	require.NoError(t, err)

	expected := sdk.AccountData{
		AccountBaseData: sdk.AccountBaseData{
			TotalAppLocalStates: 4,
		},
		VotingData: sdk.VotingData{
			VoteKeyDilution: 1,
		},
	}
	checkAccount(t, db, makeAddress(1), &expected)
}

func TestAllResourcesMultipleAccounts(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	numAccounts := 14

	for i := 0; i < numAccounts; i++ {
		insertAccount(t, db, makeAddress(i), &sdk.AccountData{VotingData: sdk.VotingData{VoteKeyDilution: uint64(i)}})
		for j := uint64(20); j < 30; j++ {
			insertAccountAsset(t, db, makeAddress(i), j, j%2 == 0)
		}
		for j := uint64(30); j < 50; j++ {
			insertAsset(t, db, uint64(i)*1000+j, makeAddress(i), j%2 == 0)
		}
		for j := uint64(50); j < 80; j++ {
			insertApp(t, db, uint64(i)*1000+j, makeAddress(i), j%2 == 0)
		}
		for j := uint64(80); j < 120; j++ {
			insertAccountApp(t, db, makeAddress(i), j, j%2 == 0)
		}
	}

	f := func(tx pgx.Tx) error {
		return cad.RunMigration(tx, 5)
	}
	err := pgutil.TxWithRetry(db, pgx.TxOptions{IsoLevel: pgx.Serializable}, f, nil)
	require.NoError(t, err)

	for i := 0; i < numAccounts; i++ {
		expected := sdk.AccountData{
			AccountBaseData: sdk.AccountBaseData{
				TotalAssets:         5,
				TotalAssetParams:    10,
				TotalAppParams:      15,
				TotalAppLocalStates: 20,
			},
			VotingData: sdk.VotingData{
				VoteKeyDilution: uint64(i),
			},
		}
		checkAccount(t, db, makeAddress(i), &expected)
	}
}
