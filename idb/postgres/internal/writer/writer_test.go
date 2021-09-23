package writer_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/protocol"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	pgtest "github.com/algorand/indexer/idb/postgres/internal/testing"
	pgutil "github.com/algorand/indexer/idb/postgres/internal/util"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util/test"
)

var serializable = pgx.TxOptions{IsoLevel: pgx.Serializable}

func setupPostgres(t *testing.T) (*pgxpool.Pool, func()) {
	db, _, shutdownFunc := pgtest.SetupPostgres(t)

	_, err := db.Exec(context.Background(), schema.SetupPostgresSql)
	require.NoError(t, err)

	return db, shutdownFunc
}

// makeTx is a helper to simplify calling TxWithRetry
func makeTx(db *pgxpool.Pool, f func(tx pgx.Tx) error) error {
	return pgutil.TxWithRetry(db, serializable, f, nil)
}

type txnRow struct {
	round int
	intra int
	typeenum idb.TxnTypeEnum
	asset int
	txid string
	txnbytes []byte
	txn []byte
	extra string
}

// getTransactions is a test helper for checking the txn table.
func txnQuery(db *pgxpool.Pool, query string) ([]txnRow, error) {
	results := make([]txnRow, 0)
	rows, err := db.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var result txnRow
		var txid []byte
		err = rows.Scan(&result.round, &result.intra, &result.typeenum,
			&result.asset, &txid, &result.txnbytes, &result.txn,
			&result.extra)
		if err != nil {
			return nil, err
		}
		result.txid = string(txid)
		results = append(results, result)
	}
	return results, nil
}

func TestWriterBlockHeaderTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(2)
	block.BlockHeader.TimeStamp = 333
	block.BlockHeader.RewardsLevel = 111111

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit(context.Background())
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

	assert.Equal(t, block.BlockHeader.Round, basics.Round(round))
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
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	block := test.MakeGenesisBlock()

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	j, err := pgutil.GetMetastate(
		context.Background(), db, nil, schema.SpecialAccountsMetastateKey)
	require.NoError(t, err)
	accounts, err := encoding.DecodeSpecialAddresses([]byte(j))
	require.NoError(t, err)

	expected := transactions.SpecialAddresses{
		FeeSink:     test.FeeAddr,
		RewardsPool: test.RewardAddr,
	}
	assert.Equal(t, expected, accounts)
}

func TestWriterTxnTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	block := bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			Round:       basics.Round(2),
			TimeStamp:   333,
			GenesisID:   test.MakeGenesis().ID(),
			GenesisHash: test.GenesisHash,
			RewardsState: bookkeeping.RewardsState{
				RewardsLevel: 111111,
			},
			TxnCounter: 9,
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: test.Proto,
			},
		},
		Payset: make([]transactions.SignedTxnInBlock, 2),
	}

	stxnad0 := test.MakePaymentTxn(
		1000, 1, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{},
		basics.Address{})
	var err error
	block.Payset[0], err =
		block.BlockHeader.EncodeSignedTxn(stxnad0.SignedTxn, stxnad0.ApplyData)
	require.NoError(t, err)

	stxnad1 := test.MakeAssetConfigTxn(
		0, 100, 1, false, "ma", "myasset", "myasset.com", test.AccountA)
	block.Payset[1], err =
		block.BlockHeader.EncodeSignedTxn(stxnad1.SignedTxn, stxnad1.ApplyData)
	require.NoError(t, err)

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(context.Background(), "SELECT * FROM txn ORDER BY intra")
	require.NoError(t, err)

	var round uint64
	var intra uint64
	var typeenum uint
	var asset uint64
	var txid []byte
	var txnbytes []byte
	var txn []byte
	var extra []byte

	require.True(t, rows.Next())
	err = rows.Scan(&round, &intra, &typeenum, &asset, &txid, &txnbytes, &txn, &extra)
	require.NoError(t, err)
	assert.Equal(t, block.Round(), basics.Round(round))
	assert.Equal(t, uint64(0), intra)
	assert.Equal(t, idb.TypeEnumPay, idb.TxnTypeEnum(typeenum))
	assert.Equal(t, uint64(0), asset)
	assert.Equal(t, stxnad0.ID().String(), string(txid))
	assert.Equal(t, protocol.Encode(&stxnad0), txnbytes)
	{
		stxn, err := encoding.DecodeSignedTxnWithAD(txn)
		require.NoError(t, err)
		assert.Equal(t, stxnad0, stxn)
	}
	assert.Equal(t, "{}", string(extra))

	require.True(t, rows.Next())
	err = rows.Scan(&round, &intra, &typeenum, &asset, &txid, &txnbytes, &txn, &extra)
	require.NoError(t, err)
	assert.Equal(t, block.Round(), basics.Round(round))
	assert.Equal(t, uint64(1), intra)
	assert.Equal(t, idb.TypeEnumAssetConfig, idb.TxnTypeEnum(typeenum))
	assert.Equal(t, uint64(9), asset)
	assert.Equal(t, stxnad1.ID().String(), string(txid))
	assert.Equal(t, protocol.Encode(&stxnad1), txnbytes)
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
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	block := bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			GenesisID:   test.MakeGenesis().ID(),
			GenesisHash: test.GenesisHash,
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: test.Proto,
			},
		},
		Payset: make(transactions.Payset, 1),
	}
	stxnad := test.MakeAssetTransferTxn(1, 2, test.AccountA, test.AccountB, test.AccountC)
	var err error
	block.Payset[0], err = block.EncodeSignedTxn(stxnad.SignedTxn, stxnad.ApplyData)
	require.NoError(t, err)

	payset := []transactions.SignedTxnInBlock{block.Payset[0]}
	payset[0].ApplyData.AssetClosingAmount = 3

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(
		context.Background(), "SELECT txn, extra FROM txn ORDER BY intra")
	require.NoError(t, err)

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

		var actual idb.TxnExtra
		err := encoding.DecodeJSON(extra, &actual)
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
	}

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

func TestWriterTxnParticipationTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	block := bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			Round:       basics.Round(2),
			GenesisID:   test.MakeGenesis().ID(),
			GenesisHash: test.GenesisHash,
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: test.Proto,
			},
		},
		Payset: make(transactions.Payset, 2),
	}

	stxnad0 := test.MakePaymentTxn(
		1000, 1, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{},
		basics.Address{})
	var err error
	block.Payset[0], err = block.EncodeSignedTxn(stxnad0.SignedTxn, stxnad0.ApplyData)
	require.NoError(t, err)

	stxnad1 := test.MakeAssetConfigTxn(
		0, 100, 1, false, "ma", "myasset", "myasset.com", test.AccountC)
	block.Payset[1], err = block.EncodeSignedTxn(stxnad1.SignedTxn, stxnad1.ApplyData)
	require.NoError(t, err)

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(
		context.Background(), "SELECT * FROM txn_participation ORDER BY round, intra, addr")
	require.NoError(t, err)

	var addr []byte
	var round uint64
	var intra uint64

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &round, &intra)
	require.NoError(t, err)
	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, uint64(2), round)
	assert.Equal(t, uint64(0), intra)

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &round, &intra)
	require.NoError(t, err)
	assert.Equal(t, test.AccountB[:], addr)
	assert.Equal(t, uint64(2), round)
	assert.Equal(t, uint64(0), intra)

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &round, &intra)
	require.NoError(t, err)
	assert.Equal(t, test.AccountC[:], addr)
	assert.Equal(t, uint64(2), round)
	assert.Equal(t, uint64(1), intra)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Create a new account and then delete it.
func TestWriterAccountTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var voteID crypto.OneTimeSignatureVerifier
	voteID[0] = 1

	var selectionID crypto.VRFVerifier
	selectionID[0] = 2

	var authAddr basics.Address
	authAddr[0] = 3

	var block bookkeeping.Block
	block.BlockHeader.Round = 4

	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, basics.AccountData{
		Status:             basics.Online,
		MicroAlgos:         basics.MicroAlgos{Raw: 5},
		RewardsBase:        6,
		RewardedMicroAlgos: basics.MicroAlgos{Raw: 7},
		VoteID:             voteID,
		SelectionID:        selectionID,
		VoteFirstValid:     7,
		VoteLastValid:      8,
		VoteKeyDilution:    9,
		AuthAddr:           authAddr,
	})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(context.Background(), "SELECT * FROM account")
	require.NoError(t, err)

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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Nil(t, closedAt)
	assert.Nil(t, keytype)
	{
		accountDataRead, err := encoding.DecodeTrimmedAccountData(accountData)
		require.NoError(t, err)

		assert.Equal(t, expectedAccountData.Status, accountDataRead.Status)
		assert.Equal(t, expectedAccountData.VoteID, accountDataRead.VoteID)
		assert.Equal(t, expectedAccountData.SelectionID, accountDataRead.SelectionID)
		assert.Equal(t, expectedAccountData.VoteFirstValid, accountDataRead.VoteFirstValid)
		assert.Equal(t, expectedAccountData.VoteLastValid, accountDataRead.VoteLastValid)
		assert.Equal(t, expectedAccountData.VoteKeyDilution, accountDataRead.VoteKeyDilution)
		assert.Equal(t, expectedAccountData.AuthAddr, accountDataRead.AuthAddr)
	}

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete this account.
	block.BlockHeader.Round++
	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.Upsert(test.AccountA, basics.AccountData{})

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM account")
	require.NoError(t, err)

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
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)
	assert.Nil(t, keytype)
	assert.Nil(t, accountData)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate the scenario where an account is created and deleted in the same round.
func TestWriterAccountTableCreateDeleteSameRound(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = 4

	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, basics.AccountData{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err := db.Query(context.Background(), "SELECT * FROM account")
	require.NoError(t, err)

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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
	assert.Nil(t, keytype)
	assert.Nil(t, accountData)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

func TestWriterDeleteAccountDoesNotDeleteKeytype(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	block := bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			Round:       basics.Round(4),
			GenesisID:   test.MakeGenesis().ID(),
			GenesisHash: test.GenesisHash,
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: test.Proto,
			},
		},
		Payset: make(transactions.Payset, 1),
	}

	stxnad := test.MakePaymentTxn(
		1000, 1, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{},
		basics.Address{})
	stxnad.Sig[0] = 5 // set signature so that keytype for account is updated
	var err error
	block.Payset[0], err = block.EncodeSignedTxn(stxnad.SignedTxn, stxnad.ApplyData)
	require.NoError(t, err)

	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 5},
	})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var keytype string

	row := db.QueryRow(context.Background(), "SELECT keytype FROM account")
	err = row.Scan(&keytype)
	require.NoError(t, err)
	assert.Equal(t, "sig", keytype)

	// Now delete this account.
	block.BlockHeader.Round = basics.Round(5)
	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.Upsert(test.AccountA, basics.AccountData{})

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	row = db.QueryRow(context.Background(), "SELECT keytype FROM account")
	err = row.Scan(&keytype)
	require.NoError(t, err)
	assert.Equal(t, "sig", keytype)
}

func TestWriterAccountAssetTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	assetHolding := basics.AssetHolding{
		Amount: 4,
		Frozen: true,
	}
	accountData := basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 5},
		Assets: map[basics.AssetIndex]basics.AssetHolding{
			assetID: assetHolding,
		},
	}
	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, accountData)

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
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

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &assetid, &amount, &frozen, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, assetID, basics.AssetIndex(assetid))
	assert.Equal(t, assetHolding.Amount, amount)
	assert.Equal(t, assetHolding.Frozen, frozen)
	assert.False(t, deleted)
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Nil(t, closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete the asset.
	block.BlockHeader.Round++

	delta.ModifiedAssetHoldings = map[ledgercore.AccountAsset]bool{
		{Address: test.AccountA, Asset: assetID}: false,
	}
	accountData.Assets = nil
	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.Upsert(test.AccountA, accountData)

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM account_asset")
	require.NoError(t, err)

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &assetid, &amount, &frozen, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, assetID, basics.AssetIndex(assetid))
	assert.Equal(t, uint64(0), amount)
	assert.Equal(t, assetHolding.Frozen, frozen)
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an asset holding is added and deleted in the same round.
func TestWriterAccountAssetTableCreateDeleteSameRound(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	delta := ledgercore.StateDelta{
		ModifiedAssetHoldings: map[ledgercore.AccountAsset]bool{
			{Address: test.AccountA, Asset: assetID}: false,
		},
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
}

func TestWriterAccountAssetTableLargeAmount(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	assetHolding := basics.AssetHolding{
		Amount: math.MaxUint64,
	}
	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 5},
		Assets: map[basics.AssetIndex]basics.AssetHolding{
			assetID: assetHolding,
		},
	})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}
	err := pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	var amount uint64

	rows, err := db.Query(context.Background(), "SELECT amount FROM account_asset")
	require.NoError(t, err)

	require.True(t, rows.Next())
	err = rows.Scan(&amount)
	require.NoError(t, err)
	assert.Equal(t, assetHolding.Amount, amount)
}

func TestWriterAssetTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	assetParams := basics.AssetParams{
		Total:   99999,
		Manager: test.AccountB,
	}
	accountData := basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 5},
		AssetParams: map[basics.AssetIndex]basics.AssetParams{
			assetID: assetParams,
		},
	}
	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, accountData)

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
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

	require.True(t, rows.Next())
	err = rows.Scan(&index, &creatorAddr, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, assetID, basics.AssetIndex(index))
	assert.Equal(t, test.AccountA[:], creatorAddr)
	{
		paramsRead, err := encoding.DecodeAssetParams(params)
		require.NoError(t, err)
		assert.Equal(t, assetParams, paramsRead)
	}
	assert.False(t, deleted)
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Nil(t, closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete the asset.
	block.BlockHeader.Round++

	delta.Creatables = map[basics.CreatableIndex]ledgercore.ModifiedCreatable{
		basics.CreatableIndex(assetID): {
			Ctype:   basics.AssetCreatable,
			Created: false,
			Creator: test.AccountA,
		},
	}
	accountData.AssetParams = nil
	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.Upsert(test.AccountA, accountData)

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM asset")
	require.NoError(t, err)

	require.True(t, rows.Next())
	err = rows.Scan(&index, &creatorAddr, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, assetID, basics.AssetIndex(index))
	assert.Equal(t, test.AccountA[:], creatorAddr)
	{
		paramsRead, err := encoding.DecodeAssetParams(params)
		require.NoError(t, err)
		assert.Equal(t, basics.AssetParams{}, paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an asset is added and deleted in the same round.
func TestWriterAssetTableCreateDeleteSameRound(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	delta := ledgercore.StateDelta{
		Creatables: map[basics.CreatableIndex]ledgercore.ModifiedCreatable{
			basics.CreatableIndex(assetID): {
				Ctype:   basics.AssetCreatable,
				Created: false,
				Creator: test.AccountA,
			},
		},
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
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
	{
		paramsRead, err := encoding.DecodeAssetParams(params)
		require.NoError(t, err)
		assert.Equal(t, basics.AssetParams{}, paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
}

func TestWriterAppTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	appID := basics.AppIndex(3)
	appParams := basics.AppParams{
		ApprovalProgram: []byte{3, 4, 5},
		GlobalState: map[string]basics.TealValue{
			string([]byte{0xff}): { // try a non-utf8 key
				Type: 3,
			},
		},
	}
	accountData := basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 5},
		AppParams: map[basics.AppIndex]basics.AppParams{
			appID: appParams,
		},
	}
	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, accountData)

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Nil(t, closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete the app.
	block.BlockHeader.Round++

	delta.Creatables = map[basics.CreatableIndex]ledgercore.ModifiedCreatable{
		basics.CreatableIndex(appID): {
			Ctype:   basics.AppCreatable,
			Created: false,
			Creator: test.AccountA,
		},
	}
	accountData.AppParams = nil
	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.Upsert(test.AccountA, accountData)

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM app")
	require.NoError(t, err)

	require.True(t, rows.Next())
	err = rows.Scan(&index, &creator, &params, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, appID, basics.AppIndex(index))
	assert.Equal(t, test.AccountA[:], creator)
	{
		paramsRead, err := encoding.DecodeAppParams(params)
		require.NoError(t, err)
		assert.Equal(t, basics.AppParams{}, paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an app is added and deleted in the same round.
func TestWriterAppTableCreateDeleteSameRound(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	appID := basics.AppIndex(3)
	delta := ledgercore.StateDelta{
		Creatables: map[basics.CreatableIndex]ledgercore.ModifiedCreatable{
			basics.CreatableIndex(appID): {
				Ctype:   basics.AppCreatable,
				Created: false,
				Creator: test.AccountA,
			},
		},
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
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
	{
		paramsRead, err := encoding.DecodeAppParams(params)
		require.NoError(t, err)
		assert.Equal(t, basics.AppParams{}, paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
}

func TestWriterAccountAppTableBasic(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	appID := basics.AppIndex(3)
	appLocalState := basics.AppLocalState{
		KeyValue: map[string]basics.TealValue{
			string([]byte{0xff}): { // try a non-utf8 key
				Type: 4,
			},
		},
	}
	accountData := basics.AccountData{
		MicroAlgos: basics.MicroAlgos{Raw: 5},
		AppLocalStates: map[basics.AppIndex]basics.AppLocalState{
			appID: appLocalState,
		},
	}
	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, accountData)

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Nil(t, closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete the app.
	block.BlockHeader.Round++

	delta.ModifiedAppLocalStates = map[ledgercore.AccountApp]bool{
		{Address: test.AccountA, App: appID}: false,
	}
	accountData.AppLocalStates = nil
	delta.Accts = ledgercore.AccountDeltas{}
	delta.Accts.Upsert(test.AccountA, accountData)

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	rows, err = db.Query(context.Background(), "SELECT * FROM account_app")
	require.NoError(t, err)

	require.True(t, rows.Next())
	err = rows.Scan(&addr, &app, &localstate, &deleted, &createdAt, &closedAt)
	require.NoError(t, err)

	assert.Equal(t, test.AccountA[:], addr)
	assert.Equal(t, appID, basics.AppIndex(app))
	{
		appLocalStateRead, err := encoding.DecodeAppLocalState(localstate)
		require.NoError(t, err)
		assert.Equal(t, basics.AppLocalState{}, appLocalStateRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an account app is added and deleted in the same round.
func TestWriterAccountAppTableCreateDeleteSameRound(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	appID := basics.AppIndex(3)
	delta := ledgercore.StateDelta{
		ModifiedAppLocalStates: map[ledgercore.AccountApp]bool{
			{Address: test.AccountA, App: appID}: false,
		},
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit(context.Background())
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
	{
		appLocalStateRead, err := encoding.DecodeAppLocalState(localstate)
		require.NoError(t, err)
		assert.Equal(t, basics.AppLocalState{}, appLocalStateRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
}

// Check that adding same block twice does not result in an error.
func TestWriterAddBlockTwice(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	block := bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			Round:       basics.Round(2),
			TimeStamp:   333,
			GenesisID:   test.MakeGenesis().ID(),
			GenesisHash: test.GenesisHash,
			RewardsState: bookkeeping.RewardsState{
				RewardsLevel: 111111,
			},
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: test.Proto,
			},
		},
		Payset: make(transactions.Payset, 2),
	}

	stxnad0 := test.MakePaymentTxn(
		1000, 1, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{},
		basics.Address{})
	var err error
	block.Payset[0], err = block.EncodeSignedTxn(stxnad0.SignedTxn, stxnad0.ApplyData)
	require.NoError(t, err)

	stxnad1 := test.MakeAssetConfigTxn(
		0, 100, 1, false, "ma", "myasset", "myasset.com", test.AccountA)
	block.Payset[1], err = block.EncodeSignedTxn(stxnad1.SignedTxn, stxnad1.ApplyData)
	require.NoError(t, err)

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit(context.Background())
	}

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)
}

func TestWriterAddBlockInnerTxnsAssetCreate(t *testing.T) {
	db, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()

	// App call with inner txns, should be intra 0, 1, 2
	appCall := test.MakeAppCallWithInner(test.AccountA, test.AccountB, test.AccountC)

	// Asset create call, should have intra = 3
	assetCreate := test.MakeAssetConfigTxn(
		0, 100, 1, false, "ma", "myasset", "myasset.com", test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &appCall, &assetCreate)
	require.NoError(t, err)

	err = makeTx(db, func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(&block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit(context.Background())
	})

	txns, err := txnQuery(db, "SELECT * FROM txn ORDER BY intra")
	require.NoError(t, err)
	require.Len(t, txns, 2)

	// Verify that intra is correctly assigned
	require.Equal(t, 0, txns[0].intra, "Intra should be assigned 0 - 3.")
	require.Equal(t, 3, txns[1].intra, "Intra should be assigned 0 - 3.")

	// Verify correct order of transaction types.
	require.Equal(t, idb.TypeEnumApplication,   txns[0].typeenum)
	require.Equal(t, idb.TypeEnumAssetConfig,   txns[1].typeenum)

	// Verify correct App and Asset IDs
	require.Equal(t, 1, txns[0].asset, "intra == 0 -> ApplicationID = 1")
	require.Equal(t, 4, txns[1].asset, "intra == 3 -> ApplicationID = 4")
}