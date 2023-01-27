package writer_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/avm-abi/apps"
	"github.com/algorand/indexer/helpers"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
	pgutil "github.com/algorand/indexer/idb/postgres/internal/util"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/types"
	"github.com/algorand/indexer/util"
	"github.com/algorand/indexer/util/test"

	crypto2 "github.com/algorand/go-algorand-sdk/v2/crypto"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

var serializable = pgx.TxOptions{IsoLevel: pgx.Serializable}

// makeTx is a helper to simplify calling TxWithRetry
func makeTx(db *pgxpool.Pool, f func(tx pgx.Tx) error) error {
	return pgutil.TxWithRetry(db, serializable, f, nil)
}

type txnRow struct {
	round    int
	intra    int
	typeenum idb.TxnTypeEnum
	asset    int
	txid     string
	txn      string
	extra    string
}

// txnQuery is a test helper for checking the txn table.
func txnQuery(db *pgxpool.Pool, query string) ([]txnRow, error) {
	var results []txnRow
	rows, err := db.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var result txnRow
		var txid []byte
		var txn []byte
		err = rows.Scan(
			&result.round, &result.intra, &result.typeenum, &result.asset, &txid,
			&txn, &result.extra)
		if err != nil {
			return nil, err
		}
		result.txid = string(txid)
		result.txn = string(txn)
		results = append(results, result)
	}
	return results, rows.Err()
}

type txnParticipationRow struct {
	addr  sdk.Address
	round int
	intra int
}

func txnParticipationQuery(db *pgxpool.Pool, query string) ([]txnParticipationRow, error) {
	var results []txnParticipationRow
	rows, err := db.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var result txnParticipationRow
		var addr []byte
		err = rows.Scan(&addr, &result.round, &result.intra)
		if err != nil {
			return nil, err
		}
		copy(result.addr[:], addr)
		results = append(results, result)
	}
	return results, rows.Err()
}

func TestWriterBlockHeaderTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(2)
	block.BlockHeader.TimeStamp = 333
	block.BlockHeader.RewardsLevel = 111111

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, ledgercore.StateDelta{})
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	row := db.QueryRow(context.Background(), "SELECT * FROM block_header")
	var round uint64
	var realtime time.Time
	var rewardslevel uint64
	var header []byte
	err = row.Scan(&round, &realtime, &rewardslevel, &header)
	require.NoError(t, err)

	assert.Equal(t, block.BlockHeader.Round, sdk.Round(round))
	{
		expected := time.Unix(block.BlockHeader.TimeStamp, 0).UTC()
		assert.True(t, expected.Equal(realtime))
	}
	assert.Equal(t, block.BlockHeader.RewardsLevel, rewardslevel)
	headerRead, err := encoding.DecodeBlockHeader(header)
	require.NoError(t, err)
	assert.Equal(t, block.BlockHeader, headerRead)
}

func TestWriterSpecialAccounts(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	block := test.MakeGenesisBlockV2()

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, ledgercore.StateDelta{})
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	j, err := pgutil.GetMetastate(
		context.Background(), db, nil, schema.SpecialAccountsMetastateKey)
	require.NoError(t, err)
	accounts, err := encoding.DecodeSpecialAddresses([]byte(j))
	require.NoError(t, err)

	expected := types.SpecialAddresses{
		FeeSink:     sdk.Address(test.FeeAddr),
		RewardsPool: sdk.Address(test.RewardAddr),
	}
	assert.Equal(t, expected, accounts)
}

func TestWriterTxnTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	block := sdk.Block{
		BlockHeader: sdk.BlockHeader{
			Round:       sdk.Round(2),
			TimeStamp:   333,
			GenesisID:   test.MakeGenesisV2().ID(),
			GenesisHash: test.MakeGenesisV2().Hash(),
			RewardsState: sdk.RewardsState{
				RewardsLevel: 111111,
			},
			TxnCounter: 9,
			UpgradeState: sdk.UpgradeState{
				CurrentProtocol: "future",
			},
		},
		Payset: make([]sdk.SignedTxnInBlock, 2),
	}

	stxnad0 := test.MakePaymentTxnV2(
		1000, 1, 0, 0, 0, 0, sdk.Address(test.AccountA), sdk.Address(test.AccountB), sdk.Address{},
		sdk.Address{})
	var err error
	block.Payset[0], err =
		util.EncodeSignedTxn(block.BlockHeader, stxnad0.SignedTxn, stxnad0.ApplyData)
	require.NoError(t, err)

	stxnad1 := test.MakeAssetConfigTxnV2(
		0, 100, 1, false, "ma", "myasset", "myasset.com", sdk.Address(test.AccountA))
	block.Payset[1], err =
		util.EncodeSignedTxn(block.BlockHeader, stxnad1.SignedTxn, stxnad1.ApplyData)
	require.NoError(t, err)

	f := func(tx pgx.Tx) error {
		return writer.AddTransactions(&block, block.Payset, tx)
	}
	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(context.Background(), "SELECT * FROM txn ORDER BY intra")
	require.NoError(t, err)
	defer rows.Close()

	var round uint64
	var intra uint64
	var typeenum uint
	var asset uint64
	var txid []byte
	var txn []byte
	var extra []byte

	require.True(t, rows.Next())
	err = rows.Scan(&round, &intra, &typeenum, &asset, &txid, &txn, &extra)
	require.NoError(t, err)
	assert.Equal(t, block.Round, sdk.Round(round))
	assert.Equal(t, uint64(0), intra)
	assert.Equal(t, idb.TypeEnumPay, idb.TxnTypeEnum(typeenum))
	assert.Equal(t, uint64(0), asset)
	assert.Equal(t, crypto2.TransactionIDString(stxnad0.Txn), string(txid))
	{
		stxn, err := encoding.DecodeSignedTxnWithAD(txn)
		require.NoError(t, err)
		assert.Equal(t, stxnad0, stxn)
	}
	assert.Equal(t, "{}", string(extra))

	require.True(t, rows.Next())
	err = rows.Scan(&round, &intra, &typeenum, &asset, &txid, &txn, &extra)
	require.NoError(t, err)
	assert.Equal(t, block.Round, sdk.Round(round))
	assert.Equal(t, uint64(1), intra)
	assert.Equal(t, idb.TypeEnumAssetConfig, idb.TxnTypeEnum(typeenum))
	assert.Equal(t, uint64(9), asset)
	assert.Equal(t, crypto2.TransactionIDString(stxnad1.Txn), string(txid))
	{
		stxn, err := encoding.DecodeSignedTxnWithAD(txn)
		require.NoError(t, err)
		assert.Equal(t, stxnad1, stxn)
	}
	assert.Equal(t, "{}", string(extra))

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Test that asset close amount is written even if it is missing in the apply data
// in the block (it is present in the "modified transactions").
func TestWriterTxnTableAssetCloseAmount(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	block := sdk.Block{
		BlockHeader: sdk.BlockHeader{
			GenesisID:   test.MakeGenesisV2().ID(),
			GenesisHash: test.MakeGenesisV2().Hash(),
			UpgradeState: sdk.UpgradeState{
				CurrentProtocol: "future",
			},
		},
		Payset: make(sdk.Payset, 1),
	}
	stxnad := test.MakeAssetTransferTxnV2(1, 2, sdk.Address(test.AccountA), sdk.Address(test.AccountB), sdk.Address(test.AccountC))
	var err error
	block.Payset[0], err = util.EncodeSignedTxn(block.BlockHeader, stxnad.SignedTxn, stxnad.ApplyData)
	require.NoError(t, err)

	payset := []sdk.SignedTxnInBlock{block.Payset[0]}
	payset[0].ApplyData.AssetClosingAmount = 3

	f := func(tx pgx.Tx) error {
		return writer.AddTransactions(&block, payset, tx)
	}
	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(
		context.Background(), "SELECT txn, extra FROM txn ORDER BY intra")
	require.NoError(t, err)
	defer rows.Close()

	var txn []byte
	var extra []byte
	require.True(t, rows.Next())
	err = rows.Scan(&txn, &extra)
	require.NoError(t, err)

	{
		ret, err := encoding.DecodeSignedTxnWithAD(txn)
		require.NoError(t, err)
		assert.Equal(t, stxnad, ret)
	}
	{
		expected := idb.TxnExtra{AssetCloseAmount: 3}

		actual, err := encoding.DecodeTxnExtra(extra)
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
	}

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

func TestWriterTxnParticipationTable(t *testing.T) {
	type testtype struct {
		name     string
		payset   sdk.Payset
		expected []txnParticipationRow
	}

	makeBlockFunc := func() sdk.Block {
		return sdk.Block{
			BlockHeader: sdk.BlockHeader{
				Round:       sdk.Round(2),
				GenesisID:   test.MakeGenesisV2().ID(),
				GenesisHash: test.MakeGenesisV2().Hash(),
				UpgradeState: sdk.UpgradeState{
					CurrentProtocol: "future",
				},
			},
		}
	}

	var tests []testtype
	{
		stxnad0 := test.MakePaymentTxnV2(
			1000, 1, 0, 0, 0, 0, sdk.Address(test.AccountA), sdk.Address(test.AccountB), sdk.Address{},
			sdk.Address{})
		stib0, err := util.EncodeSignedTxn(makeBlockFunc().BlockHeader, stxnad0.SignedTxn, stxnad0.ApplyData)
		require.NoError(t, err)

		stxnad1 := test.MakeAssetConfigTxnV2(
			0, 100, 1, false, "ma", "myasset", "myasset.com", sdk.Address(test.AccountC))
		stib1, err := util.EncodeSignedTxn(makeBlockFunc().BlockHeader, stxnad1.SignedTxn, stxnad1.ApplyData)
		require.NoError(t, err)

		testcase := testtype{
			name:   "basic",
			payset: []sdk.SignedTxnInBlock{stib0, stib1},
			expected: []txnParticipationRow{
				{
					addr:  sdk.Address(test.AccountA),
					round: 2,
					intra: 0,
				},
				{
					addr:  sdk.Address(test.AccountB),
					round: 2,
					intra: 0,
				},
				{
					addr:  sdk.Address(test.AccountC),
					round: 2,
					intra: 1,
				},
			},
		}
		tests = append(tests, testcase)
	}
	{
		stxnad := test.MakeCreateAppTxnV2(sdk.Address(test.AccountA))
		stxnad.Txn.ApplicationCallTxnFields.Accounts =
			[]sdk.Address{sdk.Address(test.AccountB), sdk.Address(test.AccountC)}
		stib, err := util.EncodeSignedTxn(makeBlockFunc().BlockHeader, stxnad.SignedTxn, stxnad.ApplyData)
		require.NoError(t, err)

		testcase := testtype{
			name:   "app_call_addresses",
			payset: []sdk.SignedTxnInBlock{stib},
			expected: []txnParticipationRow{
				{
					addr:  sdk.Address(test.AccountA),
					round: 2,
					intra: 0,
				},
				{
					addr:  sdk.Address(test.AccountB),
					round: 2,
					intra: 0,
				},
				{
					addr:  sdk.Address(test.AccountC),
					round: 2,
					intra: 0,
				},
			},
		}
		tests = append(tests, testcase)
	}

	for _, testcase := range tests {
		t.Run(testcase.name, func(t *testing.T) {
			db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
			defer shutdownFunc()

			block := makeBlockFunc()
			block.Payset = testcase.payset

			f := func(tx pgx.Tx) error {
				return writer.AddTransactionParticipation(&block, tx)
			}
			err := pgutil.TxWithRetry(db, serializable, f, nil)
			require.NoError(t, err)

			results, err := txnParticipationQuery(
				db, `SELECT * FROM txn_participation ORDER BY round, intra, addr`)
			assert.NoError(t, err)

			// Verify expected participation
			assert.Equal(t, testcase.expected, results)
		})
	}
}

// Create a new account and then delete it.
func TestWriterAccountTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var voteID crypto.OneTimeSignatureVerifier
	voteID[0] = 1

	var selectionID crypto.VRFVerifier
	selectionID[0] = 2

	var authAddr basics.Address
	authAddr[0] = 3

	var block sdk.Block
	block.BlockHeader.Round = 4

	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, ledgercore.AccountData{
		AccountBaseData: ledgercore.AccountBaseData{
			Status:             basics.Online,
			MicroAlgos:         basics.MicroAlgos{Raw: 5},
			RewardsBase:        6,
			RewardedMicroAlgos: basics.MicroAlgos{Raw: 7},
			AuthAddr:           authAddr,
		},
		VotingData: ledgercore.VotingData{
			VoteID:          voteID,
			SelectionID:     selectionID,
			VoteFirstValid:  7,
			VoteLastValid:   8,
			VoteKeyDilution: 9,
		},
	})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(context.Background(), "SELECT * FROM account")
	require.NoError(t, err)
	defer rows.Close()

	var addr []byte
	var microalgos uint64
	var rewardsbase uint64
	var rewardsTotal uint64
	var deleted bool
	var createdAt uint64
	var closedAt *uint64
	var keytype *string
	var accountData []byte

	require.True(t, rows.Next())
	err = rows.Scan(
		&addr, &microalgos, &rewardsbase, &rewardsTotal, &deleted, &createdAt, &closedAt,
		&keytype, &accountData)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	_, expectedAccountData := delta.Accts.GetByIdx(0)
	assert.Equal(t, expectedAccountData.MicroAlgos, basics.MicroAlgos{Raw: microalgos})
	assert.Equal(t, expectedAccountData.RewardsBase, rewardsbase)
	assert.Equal(
		t, expectedAccountData.RewardedMicroAlgos,
		basics.MicroAlgos{Raw: rewardsTotal})
	assert.False(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Nil(t, closedAt)
	assert.Nil(t, keytype)
	{
		accountDataRead, err := encoding.DecodeTrimmedLcAccountData(accountData)
		require.NoError(t, err)
		assert.Equal(t, encoding.TrimLcAccountData(expectedAccountData), accountDataRead)
	}

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete this account.
	block.BlockHeader.Round++
	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.Upsert(test.AccountA, ledgercore.AccountData{})

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM account")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(
		&addr, &microalgos, &rewardsbase, &rewardsTotal, &deleted, &createdAt, &closedAt,
		&keytype, &accountData)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, uint64(0), microalgos)
	assert.Equal(t, uint64(0), rewardsbase)
	assert.Equal(t, uint64(0), rewardsTotal)
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round)-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round), *closedAt)
	assert.Nil(t, keytype)
	assert.Equal(t, []byte("null"), accountData)
	{
		accountData, err := encoding.DecodeTrimmedLcAccountData(accountData)
		require.NoError(t, err)
		assert.Equal(t, ledgercore.AccountData{}, accountData)
	}

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate the scenario where an account is created and deleted in the same round.
func TestWriterAccountTableCreateDeleteSameRound(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = 4

	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, ledgercore.AccountData{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(context.Background(), "SELECT * FROM account")
	require.NoError(t, err)
	defer rows.Close()

	var addr []byte
	var microalgos uint64
	var rewardsbase uint64
	var rewardsTotal uint64
	var deleted bool
	var createdAt uint64
	var closedAt uint64
	var keytype *string
	var accountData []byte

	require.True(t, rows.Next())
	err = rows.Scan(
		&addr, &microalgos, &rewardsbase, &rewardsTotal, &deleted, &createdAt, &closedAt,
		&keytype, &accountData)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, uint64(0), microalgos)
	assert.Equal(t, uint64(0), rewardsbase)
	assert.Equal(t, uint64(0), rewardsTotal)
	assert.True(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Equal(t, block.Round, sdk.Round(closedAt))
	assert.Nil(t, keytype)
	assert.Equal(t, []byte("null"), accountData)
	{
		accountData, err := encoding.DecodeTrimmedLcAccountData(accountData)
		require.NoError(t, err)
		assert.Equal(t, ledgercore.AccountData{}, accountData)
	}

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

func TestWriterDeleteAccountDoesNotDeleteKeytype(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	block := sdk.Block{
		BlockHeader: sdk.BlockHeader{
			Round:       sdk.Round(4),
			GenesisID:   test.MakeGenesisV2().ID(),
			GenesisHash: test.MakeGenesisV2().Hash(),
			UpgradeState: sdk.UpgradeState{
				CurrentProtocol: "future",
			},
		},
		Payset: make(sdk.Payset, 1),
	}

	stxnad := test.MakePaymentTxnV2(
		1000, 1, 0, 0, 0, 0, sdk.Address(test.AccountA), sdk.Address(test.AccountB), sdk.Address{},
		sdk.Address{})
	stxnad.Sig[0] = 5 // set signature so that keytype for account is updated
	var err error
	block.Payset[0], err = util.EncodeSignedTxn(block.BlockHeader, stxnad.SignedTxn, stxnad.ApplyData)
	require.NoError(t, err)

	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, ledgercore.AccountData{
		AccountBaseData: ledgercore.AccountBaseData{
			MicroAlgos: basics.MicroAlgos{Raw: 5},
		},
	})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var keytype string

	row := db.QueryRow(context.Background(), "SELECT keytype FROM account")
	err = row.Scan(&keytype)
	require.NoError(t, err)
	assert.Equal(t, "sig", keytype)

	// Now delete this account.
	block.BlockHeader.Round = sdk.Round(5)
	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.Upsert(test.AccountA, ledgercore.AccountData{})

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	row = db.QueryRow(context.Background(), "SELECT keytype FROM account")
	err = row.Scan(&keytype)
	require.NoError(t, err)
	assert.Equal(t, "sig", keytype)
}

func TestWriterAccountAssetTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	assetID := basics.AssetIndex(3)
	assetHolding := basics.AssetHolding{
		Amount: 4,
		Frozen: true,
	}
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{},
		ledgercore.AssetHoldingDelta{Holding: &assetHolding})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var addr []byte
	var assetid uint64
	var amount uint64
	var frozen bool
	var deleted bool
	var createdAt uint64
	var closedAt *uint64

	rows, err := db.Query(context.Background(), "SELECT * FROM account_asset")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &assetid, &amount, &frozen, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, assetID, basics.AssetIndex(assetid))
	assert.Equal(t, assetHolding.Amount, amount)
	assert.Equal(t, assetHolding.Frozen, frozen)
	assert.False(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Nil(t, closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete the asset.
	block.BlockHeader.Round++

	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{},
		ledgercore.AssetHoldingDelta{Deleted: true})

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM account_asset")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &assetid, &amount, &frozen, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, assetID, basics.AssetIndex(assetid))
	assert.Equal(t, uint64(0), amount)
	assert.Equal(t, assetHolding.Frozen, frozen)
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round)-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an asset holding is added and deleted in the same round.
func TestWriterAccountAssetTableCreateDeleteSameRound(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	assetID := basics.AssetIndex(3)
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{},
		ledgercore.AssetHoldingDelta{Deleted: true})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var addr []byte
	var assetid uint64
	var amount uint64
	var frozen bool
	var deleted bool
	var createdAt uint64
	var closedAt uint64

	row := db.QueryRow(context.Background(), "SELECT * FROM account_asset")
	err = row.Scan(&addr, &assetid, &amount, &frozen, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, assetID, basics.AssetIndex(assetid))
	assert.Equal(t, uint64(0), amount)
	assert.False(t, frozen)
	assert.True(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Equal(t, block.Round, sdk.Round(closedAt))
}

func TestWriterAccountAssetTableLargeAmount(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	assetID := basics.AssetIndex(3)
	assetHolding := basics.AssetHolding{
		Amount: math.MaxUint64,
	}
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{},
		ledgercore.AssetHoldingDelta{Holding: &assetHolding})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var amount uint64

	row := db.QueryRow(context.Background(), "SELECT amount FROM account_asset")
	err = row.Scan(&amount)
	require.NoError(t, err)
	assert.Equal(t, assetHolding.Amount, amount)
}

func TestWriterAssetTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	assetID := basics.AssetIndex(3)
	assetParams := basics.AssetParams{
		Total:   99999,
		Manager: test.AccountB,
	}
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{Params: &assetParams},
		ledgercore.AssetHoldingDelta{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var index uint64
	var creatorAddr []byte
	var params []byte
	var deleted bool
	var createdAt uint64
	var closedAt *uint64

	rows, err := db.Query(context.Background(), "SELECT * FROM asset")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(&index, &creatorAddr, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, assetID, basics.AssetIndex(index))
	assert.Equal(t, test.AccountA[:], creatorAddr)
	{
		paramsRead, err := encoding.DecodeAssetParams(params)
		require.NoError(t, err)
		assert.Equal(t, helpers.ConvertParams(assetParams), paramsRead)
	}
	assert.False(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Nil(t, closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete the asset.
	block.BlockHeader.Round++

	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{Deleted: true},
		ledgercore.AssetHoldingDelta{})

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM asset")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(&index, &creatorAddr, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, assetID, basics.AssetIndex(index))
	assert.Equal(t, test.AccountA[:], creatorAddr)
	assert.Equal(t, []byte("null"), params)
	{
		paramsRead, err := encoding.DecodeAssetParams(params)
		require.NoError(t, err)
		assert.Equal(t, helpers.ConvertParams(basics.AssetParams{}), paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round)-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an asset is added and deleted in the same round.
func TestWriterAssetTableCreateDeleteSameRound(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	assetID := basics.AssetIndex(3)
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{Deleted: true},
		ledgercore.AssetHoldingDelta{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var index uint64
	var creatorAddr []byte
	var params []byte
	var deleted bool
	var createdAt uint64
	var closedAt uint64

	row := db.QueryRow(context.Background(), "SELECT * FROM asset")
	err = row.Scan(&index, &creatorAddr, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, assetID, basics.AssetIndex(index))
	assert.Equal(t, test.AccountA[:], creatorAddr)
	assert.Equal(t, []byte("null"), params)
	{
		paramsRead, err := encoding.DecodeAssetParams(params)
		require.NoError(t, err)
		assert.Equal(t, helpers.ConvertParams(basics.AssetParams{}), paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Equal(t, block.Round, sdk.Round(closedAt))
}

func TestWriterAppTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	appID := basics.AppIndex(3)
	appParams := basics.AppParams{
		ApprovalProgram: []byte{3, 4, 5},
		GlobalState: map[string]basics.TealValue{
			string([]byte{0xff}): { // try a non-utf8 key
				Type: 3,
			},
		},
	}
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{Params: &appParams},
		ledgercore.AppLocalStateDelta{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var index uint64
	var creator []byte
	var params []byte
	var deleted bool
	var createdAt uint64
	var closedAt *uint64

	rows, err := db.Query(context.Background(), "SELECT * FROM app")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(&index, &creator, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, appID, basics.AppIndex(index))
	assert.Equal(t, test.AccountA[:], creator)
	{
		paramsRead, err := encoding.DecodeAppParams(params)
		require.NoError(t, err)
		assert.Equal(t, appParams, paramsRead)
	}
	assert.False(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Nil(t, closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete the app.
	block.BlockHeader.Round++

	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{Deleted: true},
		ledgercore.AppLocalStateDelta{})

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM app")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(&index, &creator, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, appID, basics.AppIndex(index))
	assert.Equal(t, test.AccountA[:], creator)
	assert.Equal(t, []byte("null"), params)
	{
		paramsRead, err := encoding.DecodeAppParams(params)
		require.NoError(t, err)
		assert.Equal(t, basics.AppParams{}, paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round)-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an app is added and deleted in the same round.
func TestWriterAppTableCreateDeleteSameRound(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	appID := basics.AppIndex(3)
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{Deleted: true},
		ledgercore.AppLocalStateDelta{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var index uint64
	var creator []byte
	var params []byte
	var deleted bool
	var createdAt uint64
	var closedAt uint64

	row := db.QueryRow(context.Background(), "SELECT * FROM app")
	require.NoError(t, err)
	err = row.Scan(&index, &creator, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, appID, basics.AppIndex(index))
	assert.Equal(t, test.AccountA[:], creator)
	assert.Equal(t, []byte("null"), params)
	{
		paramsRead, err := encoding.DecodeAppParams(params)
		require.NoError(t, err)
		assert.Equal(t, basics.AppParams{}, paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Equal(t, block.Round, sdk.Round(closedAt))
}

func TestWriterAccountAppTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	appID := basics.AppIndex(3)
	appLocalState := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			string([]byte{0xff}): { // try a non-utf8 key
				Type: 4,
			},
		},
	}
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{},
		ledgercore.AppLocalStateDelta{LocalState: &appLocalState})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var addr []byte
	var app uint64
	var localstate []byte
	var deleted bool
	var createdAt uint64
	var closedAt *uint64

	rows, err := db.Query(context.Background(), "SELECT * FROM account_app")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &app, &localstate, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, appID, basics.AppIndex(app))
	{
		appLocalStateRead, err := encoding.DecodeAppLocalState(localstate)
		require.NoError(t, err)
		assert.Equal(t, appLocalState, appLocalStateRead)
	}
	assert.False(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Nil(t, closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete the app.
	block.BlockHeader.Round++

	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{},
		ledgercore.AppLocalStateDelta{Deleted: true})

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM account_app")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &app, &localstate, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, appID, basics.AppIndex(app))
	assert.Equal(t, []byte("null"), localstate)
	{
		appLocalStateRead, err := encoding.DecodeAppLocalState(localstate)
		require.NoError(t, err)
		assert.Equal(t, basics.AppLocalState{}, appLocalStateRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round)-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an account app is added and deleted in the same round.
func TestWriterAccountAppTableCreateDeleteSameRound(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)

	appID := basics.AppIndex(3)
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{},
		ledgercore.AppLocalStateDelta{Deleted: true})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var addr []byte
	var app uint64
	var localstate []byte
	var deleted bool
	var createdAt uint64
	var closedAt uint64

	row := db.QueryRow(context.Background(), "SELECT * FROM account_app")
	err = row.Scan(&addr, &app, &localstate, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, appID, basics.AppIndex(app))
	assert.Equal(t, []byte("null"), localstate)
	{
		appLocalStateRead, err := encoding.DecodeAppLocalState(localstate)
		require.NoError(t, err)
		assert.Equal(t, basics.AppLocalState{}, appLocalStateRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, block.Round, sdk.Round(createdAt))
	assert.Equal(t, block.Round, sdk.Round(closedAt))
}

func TestAddBlockInvalidInnerAsset(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	callWithBadInner := test.MakeCreateAppTxnV2(sdk.Address(test.AccountA))
	callWithBadInner.ApplyData.EvalDelta.InnerTxns = []sdk.SignedTxnWithAD{
		{
			ApplyData: sdk.ApplyData{
				// This is the invalid inner asset. It should not be zero.
				ConfigAsset: 0,
			},
			SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{
					Type: sdk.AssetConfigTx,
					Header: sdk.Header{
						Sender: sdk.Address(test.AccountB),
					},
					AssetConfigTxnFields: sdk.AssetConfigTxnFields{
						ConfigAsset: 0,
					},
				},
			},
		},
	}

	genesisBlock := test.MakeGenesisBlockV2()
	block, err := test.MakeBlockForTxnsV2(genesisBlock.BlockHeader, &callWithBadInner)
	require.NoError(t, err)

	err = makeTx(db, func(tx pgx.Tx) error {
		return writer.AddTransactions(&block, block.Payset, tx)
	})
	require.Contains(t, err.Error(), "Missing ConfigAsset for transaction: ")
}

func TestWriterAddBlockInnerTxnsAssetCreate(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	// App call with inner txns, should be intra 0, 1, 2, 3, 4
	var appAddr sdk.Address
	appAddr[1] = 99
	appCall := test.MakeAppCallWithInnerTxnV2(sdk.Address(test.AccountA), appAddr, sdk.Address(test.AccountB), appAddr, sdk.Address(test.AccountC))

	// Asset create call, should have intra = 5
	assetCreate := test.MakeAssetConfigTxnV2(
		0, 100, 1, false, "ma", "myasset", "myasset.com", sdk.Address(test.AccountD))

	genesisBlock := test.MakeGenesisBlockV2()
	block, err := test.MakeBlockForTxnsV2(genesisBlock.BlockHeader, &appCall, &assetCreate)
	require.NoError(t, err)

	err = makeTx(db, func(tx pgx.Tx) error {
		err := writer.AddTransactions(&block, block.Payset, tx)
		if err != nil {
			return err
		}
		return writer.AddTransactionParticipation(&block, tx)
	})
	require.NoError(t, err)

	txns, err := txnQuery(db, "SELECT * FROM txn ORDER BY intra")
	require.NoError(t, err)
	require.Len(t, txns, 6)

	// Verify that intra is correctly assigned
	for i, tx := range txns {
		require.Equal(t, i, tx.intra, "Intra should be assigned 0 - 4.")
	}

	// Verify correct order of transaction types.
	require.Equal(t, idb.TypeEnumApplication, txns[0].typeenum)
	require.Equal(t, idb.TypeEnumPay, txns[1].typeenum)
	require.Equal(t, idb.TypeEnumApplication, txns[2].typeenum)
	require.Equal(t, idb.TypeEnumAssetTransfer, txns[3].typeenum)
	require.Equal(t, idb.TypeEnumApplication, txns[4].typeenum)
	require.Equal(t, idb.TypeEnumAssetConfig, txns[5].typeenum)

	// Verify special properties of inner transactions.
	expectedExtra := fmt.Sprintf(`{"root-txid": "%s", "root-intra": "%d"}`, txns[0].txid, 0)

	// Inner pay 1
	require.Equal(t, "", txns[1].txid)
	require.Equal(t, expectedExtra, txns[1].extra)

	// Inner pay 2
	require.Equal(t, "", txns[2].txid)
	require.Equal(t, expectedExtra, txns[2].extra)
	require.NotContains(t, txns[2].txn, "itx", "The inner transactions should be pruned.")

	// Inner xfer
	require.Equal(t, "", txns[3].txid)
	require.Equal(t, expectedExtra, txns[3].extra)
	require.NotContains(t, txns[3].txn, "itx", "The inner transactions should be pruned.")

	// Verify correct App and Asset IDs
	require.Equal(t, 1, txns[0].asset, "intra == 0 -> ApplicationID = 1")
	require.Equal(t, 789, txns[4].asset, "intra == 4 -> ApplicationID = 789")
	require.Equal(t, 6, txns[5].asset, "intra == 5 -> AssetID = 6")

	// Verify txn participation
	txnPart, err := txnParticipationQuery(db, `SELECT * FROM txn_participation ORDER BY round, intra, addr`)
	require.NoError(t, err)

	expectedParticipation := []txnParticipationRow{
		// Top-level appl transaction + inner transactions
		{
			addr:  appAddr,
			round: 1,
			intra: 0,
		},
		{
			addr:  sdk.Address(test.AccountA),
			round: 1,
			intra: 0,
		},
		{
			addr:  sdk.Address(test.AccountB),
			round: 1,
			intra: 0,
		},
		{
			addr:  sdk.Address(test.AccountC),
			round: 1,
			intra: 0,
		},
		// Inner pay transaction 1
		{
			addr:  appAddr,
			round: 1,
			intra: 1,
		},
		{
			addr:  sdk.Address(test.AccountB),
			round: 1,
			intra: 1,
		},
		// Inner pay transaction 2
		{
			addr:  appAddr,
			round: 1,
			intra: 2,
		},
		// Inner xfer transaction
		{
			addr:  appAddr,
			round: 1,
			intra: 3,
		},
		{
			addr:  sdk.Address(test.AccountC),
			round: 1,
			intra: 3,
		},
		// Inner appl transaction
		{
			addr:  appAddr,
			round: 1,
			intra: 4,
		},
		// acfg after appl
		{
			addr:  sdk.Address(test.AccountD),
			round: 1,
			intra: 5,
		},
	}

	require.Len(t, txnPart, len(expectedParticipation))
	for i := 0; i < len(txnPart); i++ {
		require.Equal(t, expectedParticipation[i], txnPart[i])
	}
}

func TestWriterAddBlock0(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	block := test.MakeGenesisBlockV2()

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, ledgercore.StateDelta{})
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	// Test that the block header was written correctly.
	{
		row := db.QueryRow(context.Background(), "SELECT * FROM block_header")
		var round uint64
		var realtime time.Time
		var rewardslevel uint64
		var header []byte
		err = row.Scan(&round, &realtime, &rewardslevel, &header)
		require.NoError(t, err)

		assert.Equal(t, block.BlockHeader.Round, sdk.Round(round))
		{
			expected := time.Unix(block.BlockHeader.TimeStamp, 0).UTC()
			assert.True(t, expected.Equal(realtime))
		}
		assert.Equal(t, block.BlockHeader.RewardsLevel, rewardslevel)
		headerRead, err := encoding.DecodeBlockHeader(header)
		require.NoError(t, err)
		assert.Equal(t, block.BlockHeader, headerRead)
	}

	// Test that the special addresses were written to the metastate.
	{
		j, err := pgutil.GetMetastate(
			context.Background(), db, nil, schema.SpecialAccountsMetastateKey)
		require.NoError(t, err)
		accounts, err := encoding.DecodeSpecialAddresses([]byte(j))
		require.NoError(t, err)

		expected := types.SpecialAddresses{
			FeeSink:     sdk.Address(test.FeeAddr),
			RewardsPool: sdk.Address(test.RewardAddr),
		}
		assert.Equal(t, expected, accounts)
	}
}
func getNameAndAccountPointer(t *testing.T, value ledgercore.KvValueDelta, fullKey string, accts map[basics.Address]*ledgercore.AccountData) (basics.Address, string, *ledgercore.AccountData) {
	require.NotNil(t, value, "cannot handle a nil value for box stats modification")
	appIdxUint, name, err := apps.SplitBoxKey(fullKey)
	appIdx := basics.AppIndex(appIdxUint)
	account := appIdx.Address()
	require.NoError(t, err)
	acctData, ok := accts[account]
	if !ok {
		acctData = &ledgercore.AccountData{
			AccountBaseData: ledgercore.AccountBaseData{},
		}
		accts[account] = acctData
	}
	return account, name, acctData
}

func addBoxInfoToStats(t *testing.T, fullKey string, value ledgercore.KvValueDelta,
	accts map[basics.Address]*ledgercore.AccountData, boxTotals map[basics.Address]basics.AccountData) {
	addr, name, acctData := getNameAndAccountPointer(t, value, fullKey, accts)

	acctData.TotalBoxes++
	acctData.TotalBoxBytes += uint64(len(name) + len(value.Data))

	boxTotals[addr] = basics.AccountData{
		TotalBoxes:    acctData.TotalBoxes,
		TotalBoxBytes: acctData.TotalBoxBytes,
	}
}

func subtractBoxInfoToStats(t *testing.T, fullKey string, value ledgercore.KvValueDelta,
	accts map[basics.Address]*ledgercore.AccountData, boxTotals map[basics.Address]basics.AccountData) {
	addr, name, acctData := getNameAndAccountPointer(t, value, fullKey, accts)

	prevBoxBytes := uint64(len(name) + len(value.Data))
	require.GreaterOrEqual(t, acctData.TotalBoxes, uint64(0))
	require.GreaterOrEqual(t, acctData.TotalBoxBytes, prevBoxBytes)

	acctData.TotalBoxes--
	acctData.TotalBoxBytes -= prevBoxBytes

	boxTotals[addr] = basics.AccountData{
		TotalBoxes:    acctData.TotalBoxes,
		TotalBoxBytes: acctData.TotalBoxBytes,
	}
}

// buildAccountDeltasFromKvsAndMods simulates keeping track of the evolution of the box statistics
func buildAccountDeltasFromKvsAndMods(t *testing.T, kvOriginals, kvMods map[string]ledgercore.KvValueDelta) (
	ledgercore.StateDelta, map[string]ledgercore.KvValueDelta, map[basics.Address]basics.AccountData) {
	kvUpdated := map[string]ledgercore.KvValueDelta{}
	boxTotals := map[basics.Address]basics.AccountData{}
	accts := map[basics.Address]*ledgercore.AccountData{}
	/*
		1. fill the accts and kvUpdated using kvOriginals
		2. for each (fullKey, value) in kvMod:
			* (A) if the key is not present in kvOriginals just add the info as in #1
			* (B) else (fullKey present):
			    * (i)  if the value is nil
					==> remove the box info from the stats and kvUpdated with assertions
				* (ii) else (value is NOT nil):
					==> reset kvUpdated and assert that the box hasn't changed shapes
	*/

	/* 1. */
	for fullKey, value := range kvOriginals {
		addBoxInfoToStats(t, fullKey, value, accts, boxTotals)
		kvUpdated[fullKey] = value
	}

	/* 2. */
	for fullKey, value := range kvMods {
		prevValue, ok := kvOriginals[fullKey]
		if !ok {
			/* 2A. */
			addBoxInfoToStats(t, fullKey, value, accts, boxTotals)
			kvUpdated[fullKey] = value
			continue
		}
		/* 2B. */
		if value.Data == nil {
			/* 2Bi. */
			subtractBoxInfoToStats(t, fullKey, prevValue, accts, boxTotals)
			delete(kvUpdated, fullKey)
			continue
		}
		/* 2Bii. */
		require.Contains(t, kvUpdated, fullKey)
		kvUpdated[fullKey] = value
	}

	var delta ledgercore.StateDelta
	for acct, acctData := range accts {
		delta.Accts.Upsert(acct, *acctData)
	}
	return delta, kvUpdated, boxTotals
}

// Simulate a scenario where app boxes are created, mutated and deleted in consecutive rounds.
func TestWriterAppBoxTableInsertMutateDelete(t *testing.T) {
	/* Simulation details:
	Box 1: inserted and then untouched
	Box 2: inserted and mutated
	Box 3: inserted and deleted
	Box 4: inserted, mutated and deleted
	Box 5: inserted, deleted and re-inserted
	Box 6: inserted after Box 2 is set
	*/

	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block sdk.Block
	block.BlockHeader.Round = sdk.Round(1)
	delta := ledgercore.StateDelta{}

	addNewBlock := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, delta)
		require.NoError(t, err)

		w.Close()
		return nil
	}

	appID := basics.AppIndex(3)
	notPresent := "NOT PRESENT"

	// ---- ROUND 1: create 5 boxes  ---- //
	n1, v1 := "box1", "inserted"
	n2, v2 := "box2", "inserted"
	n3, v3 := "box3", "inserted"
	n4, v4 := "box4", "inserted"
	n5, v5 := "box5", "inserted"

	k1 := apps.MakeBoxKey(uint64(appID), n1)
	k2 := apps.MakeBoxKey(uint64(appID), n2)
	k3 := apps.MakeBoxKey(uint64(appID), n3)
	k4 := apps.MakeBoxKey(uint64(appID), n4)
	k5 := apps.MakeBoxKey(uint64(appID), n5)

	delta.KvMods = map[string]ledgercore.KvValueDelta{}
	delta.KvMods[k1] = ledgercore.KvValueDelta{Data: []byte(v1)}
	delta.KvMods[k2] = ledgercore.KvValueDelta{Data: []byte(v2)}
	delta.KvMods[k3] = ledgercore.KvValueDelta{Data: []byte(v3)}
	delta.KvMods[k4] = ledgercore.KvValueDelta{Data: []byte(v4)}
	delta.KvMods[k5] = ledgercore.KvValueDelta{Data: []byte(v5)}

	delta2, newKvMods, accts := buildAccountDeltasFromKvsAndMods(t, map[string]ledgercore.KvValueDelta{}, delta.KvMods)
	delta.Accts = delta2.Accts

	err := pgutil.TxWithRetry(db, serializable, addNewBlock, nil)
	require.NoError(t, err)

	validateRow := func(expectedName string, expectedValue string) {
		appBoxSQL := `SELECT app, name, value FROM app_box WHERE app = $1 AND name = $2`
		var app basics.AppIndex
		var name, value []byte

		row := db.QueryRow(context.Background(), appBoxSQL, appID, []byte(expectedName))
		err := row.Scan(&app, &name, &value)

		if expectedValue == notPresent {
			require.ErrorContains(t, err, "no rows in result set")
			return
		}

		require.NoError(t, err)
		require.Equal(t, appID, app)
		require.Equal(t, expectedName, string(name))
		require.Equal(t, expectedValue, string(value))
	}

	validateTotals := func() {
		acctDataSQL := "SELECT account_data FROM account WHERE addr = $1"
		for addr, acctInfo := range accts {
			row := db.QueryRow(context.Background(), acctDataSQL, addr[:])

			var buf []byte
			err := row.Scan(&buf)
			require.NoError(t, err)

			ret, err := encoding.DecodeTrimmedLcAccountData(buf)
			require.NoError(t, err)
			require.Equal(t, acctInfo.TotalBoxes, ret.TotalBoxes)
			require.Equal(t, acctInfo.TotalBoxBytes, ret.TotalBoxBytes)
		}
	}

	validateRow(n1, v1)
	validateRow(n2, v2)
	validateRow(n3, v3)
	validateRow(n4, v4)
	validateRow(n5, v5)

	validateTotals()

	// ---- ROUND 2: mutate 2, delete 3, mutate 4, delete 5, create 6  ---- //
	oldV2 := v2
	v2 = "mutated"
	// v3 is "deleted"
	oldV4 := v4
	v4 = "mutated"
	// v5 is "deleted"
	n6, v6 := "box6", "inserted"

	k6 := apps.MakeBoxKey(uint64(appID), n6)

	delta.KvMods = map[string]ledgercore.KvValueDelta{}
	delta.KvMods[k2] = ledgercore.KvValueDelta{Data: []byte(v2), OldData: []byte(oldV2)}
	delta.KvMods[k3] = ledgercore.KvValueDelta{Data: nil}
	delta.KvMods[k4] = ledgercore.KvValueDelta{Data: []byte(v4), OldData: []byte(oldV4)}
	delta.KvMods[k5] = ledgercore.KvValueDelta{Data: nil}
	delta.KvMods[k6] = ledgercore.KvValueDelta{Data: []byte(v6)}

	delta2, newKvMods, accts = buildAccountDeltasFromKvsAndMods(t, newKvMods, delta.KvMods)
	delta.Accts = delta2.Accts

	err = pgutil.TxWithRetry(db, serializable, addNewBlock, nil)
	require.NoError(t, err)

	validateRow(n1, v1) // untouched
	validateRow(n2, v2) // new v2
	validateRow(n3, notPresent)
	validateRow(n4, v4) // new v4
	validateRow(n5, notPresent)
	validateRow(n6, v6)

	validateTotals()

	// ---- ROUND 3: delete 4, insert 5  ---- //

	// v4 is "deleted"
	v5 = "re-inserted"

	delta.KvMods = map[string]ledgercore.KvValueDelta{}
	delta.KvMods[k4] = ledgercore.KvValueDelta{Data: nil}
	delta.KvMods[k5] = ledgercore.KvValueDelta{Data: []byte(v5)}

	delta2, newKvMods, accts = buildAccountDeltasFromKvsAndMods(t, newKvMods, delta.KvMods)
	delta.Accts = delta2.Accts

	err = pgutil.TxWithRetry(db, serializable, addNewBlock, nil)
	require.NoError(t, err)

	validateRow(n1, v1)         // untouched
	validateRow(n2, v2)         // untouched
	validateRow(n3, notPresent) // still deleted
	validateRow(n4, notPresent) // deleted
	validateRow(n5, v5)         // re-inserted
	validateRow(n6, v6)         // untouched

	validateTotals()

	/*** FOURTH ROUND  - NOOP ***/
	delta.KvMods = map[string]ledgercore.KvValueDelta{}
	delta2, _, accts = buildAccountDeltasFromKvsAndMods(t, newKvMods, delta.KvMods)
	delta.Accts = delta2.Accts

	err = pgutil.TxWithRetry(db, serializable, addNewBlock, nil)
	require.NoError(t, err)

	validateRow(n1, v1)         // untouched
	validateRow(n2, v2)         // untouched
	validateRow(n3, notPresent) // still deleted
	validateRow(n4, notPresent) // still deleted
	validateRow(n5, v5)         // untouched
	validateRow(n6, v6)         // untouched

	validateTotals()
}
