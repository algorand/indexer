package postgres

import (
	"context"
	"database/sql"
	"encoding/base32"
	"math"
	"testing"
	"time"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/writer"
	"github.com/algorand/indexer/util/test"
)

func TestWriterBlockHeaderTableBasic(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(2)
	block.BlockHeader.TimeStamp = 333
	block.BlockHeader.RewardsLevel = 111111

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	row := db.db.QueryRow("SELECT * FROM block_header")
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

func TestWriterTxnTableBasic(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(2)
	block.BlockHeader.TimeStamp = 333
	block.BlockHeader.RewardsLevel = 111111
	block.BlockHeader.TxnCounter = 9
	block.Payset = make([]transactions.SignedTxnInBlock, 2)
	block.Payset[0] = transactions.SignedTxnInBlock{
		SignedTxnWithAD: test.MakePaymentTxn(
			1000, 1, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{},
			basics.Address{}),
	}
	block.Payset[1] = transactions.SignedTxnInBlock{
		SignedTxnWithAD: test.MakeCreateAssetTxn(
			100, 1, false, "ma", "myasset", "myasset.com", test.AccountA),
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err := db.db.Query("SELECT * FROM txn ORDER BY intra")
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
	{
		id := block.Payset[0].SignedTxnWithAD.ID()
		expected := []byte(
			base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:]))
		assert.Equal(t, expected, txid)
	}
	assert.Equal(t, protocol.Encode(&block.Payset[0].SignedTxnWithAD), txnbytes)
	{
		stxn, err := encoding.DecodeSignedTxnWithAD(txn)
		require.NoError(t, err)
		assert.Equal(t, block.Payset[0].SignedTxnWithAD, stxn)
	}
	assert.Equal(t, "{}", string(extra))

	require.True(t, rows.Next())
	err = rows.Scan(&round, &intra, &typeenum, &asset, &txid, &txnbytes, &txn, &extra)
	require.NoError(t, err)
	assert.Equal(t, block.Round(), basics.Round(round))
	assert.Equal(t, uint64(1), intra)
	assert.Equal(t, idb.TypeEnumAssetConfig, idb.TxnTypeEnum(typeenum))
	assert.Equal(t, uint64(9), asset)
	{
		id := block.Payset[1].SignedTxnWithAD.ID()
		expected := []byte(
			base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:]))
		assert.Equal(t, expected, txid)
	}
	assert.Equal(t, protocol.Encode(&block.Payset[1].SignedTxnWithAD), txnbytes)
	{
		stxn, err := encoding.DecodeSignedTxnWithAD(txn)
		require.NoError(t, err)
		assert.Equal(t, block.Payset[1].SignedTxnWithAD, stxn)
	}
	assert.Equal(t, "{}", string(extra))

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Test that asset close amount is written even if it is missing in the apply data
// in the block (it is present in the "modified transactions").
func TestWriterTxnTableAssetCloseAmount(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block

	payset := make([]transactions.SignedTxnInBlock, 1)
	payset[0] = transactions.SignedTxnInBlock{
		SignedTxnWithAD: test.MakeAssetTransferTxn(
			1, 2, test.AccountA, test.AccountB, test.AccountC),
	}
	payset[0].ApplyData.AssetClosingAmount = 3

	block.Payset = make([]transactions.SignedTxnInBlock, 1)
	block.Payset[0] = payset[0]
	block.Payset[0].AssetClosingAmount = 0

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err := db.db.Query("SELECT txn, extra FROM txn ORDER BY intra")
	require.NoError(t, err)

	var txn []byte
	var extra []byte
	require.True(t, rows.Next())
	err = rows.Scan(&txn, &extra)
	require.NoError(t, err)

	{
		stxn, err := encoding.DecodeSignedTxnWithAD(txn)
		require.NoError(t, err)
		assert.Equal(t, block.Payset[0].SignedTxnWithAD, stxn)
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(2)
	block.Payset = make([]transactions.SignedTxnInBlock, 2)
	block.Payset[0] = transactions.SignedTxnInBlock{
		SignedTxnWithAD: test.MakePaymentTxn(
			1000, 1, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{},
			basics.Address{}),
	}
	block.Payset[1] = transactions.SignedTxnInBlock{
		SignedTxnWithAD: test.MakeCreateAssetTxn(
			100, 1, false, "ma", "myasset", "myasset.com", test.AccountC),
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err := db.db.Query("SELECT * FROM txn_participation ORDER BY round, intra, addr")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var voteID crypto.OneTimeSignatureVerifier
	voteID[0] = 1

	var selectionID crypto.VRFVerifier
	selectionID[0] = 2

	var authAddr basics.Address
	authAddr[0] = 3

	var block bookkeeping.Block
	block.BlockHeader.Round = 4

	delta := ledgercore.StateDelta{
		Accts: ledgercore.AccountDeltas{
			Accts: []basics.BalanceRecord{
				{
					Addr: test.AccountA,
					AccountData: basics.AccountData{
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
					},
				},
			},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err := db.db.Query("SELECT * FROM account")
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
	assert.Equal(t, delta.Accts.Accts[0].MicroAlgos, basics.MicroAlgos{Raw: microalgos})
	assert.Equal(t, delta.Accts.Accts[0].RewardsBase, rewardsbase)
	assert.Equal(
		t, delta.Accts.Accts[0].RewardedMicroAlgos,
		basics.MicroAlgos{Raw: rewardsTotal})
	assert.False(t, deleted)
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Nil(t, closedAt)
	assert.Nil(t, keytype)
	{
		accountDataRead, err := encoding.DecodeAccountData(accountData)
		require.NoError(t, err)

		assert.Equal(t, delta.Accts.Accts[0].Status, accountDataRead.Status)
		assert.Equal(t, delta.Accts.Accts[0].VoteID, accountDataRead.VoteID)
		assert.Equal(t, delta.Accts.Accts[0].SelectionID, accountDataRead.SelectionID)
		assert.Equal(t, delta.Accts.Accts[0].VoteFirstValid, accountDataRead.VoteFirstValid)
		assert.Equal(t, delta.Accts.Accts[0].VoteLastValid, accountDataRead.VoteLastValid)
		assert.Equal(t, delta.Accts.Accts[0].VoteKeyDilution, accountDataRead.VoteKeyDilution)
		assert.Equal(t, delta.Accts.Accts[0].AuthAddr, accountDataRead.AuthAddr)
	}

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())

	// Now delete this account.
	block.BlockHeader.Round++
	delta.Accts.Accts[0].AccountData = basics.AccountData{}

	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err = db.db.Query("SELECT * FROM account")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = 4

	delta := ledgercore.StateDelta{
		Accts: ledgercore.AccountDeltas{
			Accts: []basics.BalanceRecord{
				{
					Addr:        test.AccountA,
					AccountData: basics.AccountData{},
				},
			},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err := db.db.Query("SELECT * FROM account")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = 4

	txn := transactions.SignedTxnInBlock{
		SignedTxnWithAD: test.MakePaymentTxn(
			1000, 1, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{},
			basics.Address{}),
	}
	txn.Sig[0] = 5 // set signature so that keytype for account is updated

	block.Payset = make([]transactions.SignedTxnInBlock, 1)
	block.Payset[0] = txn

	delta := ledgercore.StateDelta{
		Accts: ledgercore.AccountDeltas{
			Accts: []basics.BalanceRecord{
				{
					Addr: test.AccountA,
					AccountData: basics.AccountData{
						MicroAlgos: basics.MicroAlgos{Raw: 5},
					},
				},
			},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var keytype string

	row := db.db.QueryRow("SELECT keytype FROM account")
	err = row.Scan(&keytype)
	require.NoError(t, err)
	assert.Equal(t, "sig", keytype)

	// Now delete this account.
	block.BlockHeader.Round = basics.Round(5)
	delta.Accts.Accts[0].AccountData = basics.AccountData{}

	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	row = db.db.QueryRow("SELECT keytype FROM account")
	err = row.Scan(&keytype)
	require.NoError(t, err)
	assert.Equal(t, "sig", keytype)
}

func TestWriterAccountAssetTableBasic(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	assetHolding := basics.AssetHolding{
		Amount: 4,
		Frozen: true,
	}
	delta := ledgercore.StateDelta{
		Accts: ledgercore.AccountDeltas{
			Accts: []basics.BalanceRecord{
				{
					Addr: test.AccountA,
					AccountData: basics.AccountData{
						MicroAlgos: basics.MicroAlgos{Raw: 5},
						Assets: map[basics.AssetIndex]basics.AssetHolding{
							assetID: assetHolding,
						},
					},
				},
			},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var addr []byte
	var assetid uint64
	var amount uint64
	var frozen bool
	var deleted bool
	var createdAt uint64
	var closedAt *uint64

	rows, err := db.db.Query("SELECT * FROM account_asset")
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

	delta.DeletedAssetHoldings = map[ledgercore.AccountAsset]struct{}{
		{Address: test.AccountA, Asset: assetID}: {},
	}
	delta.Accts.Accts[0].AccountData.Assets = nil

	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err = db.db.Query("SELECT * FROM account_asset")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	delta := ledgercore.StateDelta{
		DeletedAssetHoldings: map[ledgercore.AccountAsset]struct{}{
			{Address: test.AccountA, Asset: assetID}: {},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var addr []byte
	var assetid uint64
	var amount uint64
	var frozen bool
	var deleted bool
	var createdAt uint64
	var closedAt uint64

	row := db.db.QueryRow("SELECT * FROM account_asset")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	assetHolding := basics.AssetHolding{
		Amount: math.MaxUint64,
	}
	delta := ledgercore.StateDelta{
		Accts: ledgercore.AccountDeltas{
			Accts: []basics.BalanceRecord{
				{
					Addr: test.AccountA,
					AccountData: basics.AccountData{
						MicroAlgos: basics.MicroAlgos{Raw: 5},
						Assets: map[basics.AssetIndex]basics.AssetHolding{
							assetID: assetHolding,
						},
					},
				},
			},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var amount uint64

	rows, err := db.db.Query("SELECT amount FROM account_asset")
	require.NoError(t, err)

	require.True(t, rows.Next())
	err = rows.Scan(&amount)
	require.NoError(t, err)
	assert.Equal(t, assetHolding.Amount, amount)
}

func TestWriterAssetTableBasic(t *testing.T) {
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	assetParams := basics.AssetParams{
		Total:   99999,
		Manager: test.AccountB,
	}
	delta := ledgercore.StateDelta{
		Accts: ledgercore.AccountDeltas{
			Accts: []basics.BalanceRecord{
				{
					Addr: test.AccountA,
					AccountData: basics.AccountData{
						MicroAlgos: basics.MicroAlgos{Raw: 5},
						AssetParams: map[basics.AssetIndex]basics.AssetParams{
							assetID: assetParams,
						},
					},
				},
			},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var index uint64
	var creatorAddr []byte
	var params []byte
	var deleted bool
	var createdAt uint64
	var closedAt *uint64

	rows, err := db.db.Query("SELECT * FROM asset")
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
	delta.Accts.Accts[0].AccountData.AssetParams = nil

	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err = db.db.Query("SELECT * FROM asset")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

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

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var index uint64
	var creatorAddr []byte
	var params []byte
	var deleted bool
	var createdAt uint64
	var closedAt uint64

	row := db.db.QueryRow("SELECT * FROM asset")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

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
	delta := ledgercore.StateDelta{
		Accts: ledgercore.AccountDeltas{
			Accts: []basics.BalanceRecord{
				{
					Addr: test.AccountA,
					AccountData: basics.AccountData{
						MicroAlgos: basics.MicroAlgos{Raw: 5},
						AppParams: map[basics.AppIndex]basics.AppParams{
							appID: appParams,
						},
					},
				},
			},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var index uint64
	var creator []byte
	var params []byte
	var deleted bool
	var createdAt uint64
	var closedAt *uint64

	rows, err := db.db.Query("SELECT * FROM app")
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
	delta.Accts.Accts[0].AccountData.AppParams = nil

	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err = db.db.Query("SELECT * FROM app")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

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

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var index uint64
	var creator []byte
	var params []byte
	var deleted bool
	var createdAt uint64
	var closedAt uint64

	row := db.db.QueryRow("SELECT * FROM app")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

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
	delta := ledgercore.StateDelta{
		Accts: ledgercore.AccountDeltas{
			Accts: []basics.BalanceRecord{
				{
					Addr: test.AccountA,
					AccountData: basics.AccountData{
						MicroAlgos: basics.MicroAlgos{Raw: 5},
						AppLocalStates: map[basics.AppIndex]basics.AppLocalState{
							appID: appLocalState,
						},
					},
				},
			},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var addr []byte
	var app uint64
	var localstate []byte
	var deleted bool
	var createdAt uint64
	var closedAt *uint64

	rows, err := db.db.Query("SELECT * FROM account_app")
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

	delta.DeletedAppLocalStates = map[ledgercore.AccountApp]struct{}{
		{Address: test.AccountA, App: appID}: {},
	}
	delta.Accts.Accts[0].AccountData.Assets = nil

	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	rows, err = db.db.Query("SELECT * FROM account_app")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	appID := basics.AppIndex(3)
	delta := ledgercore.StateDelta{
		DeletedAppLocalStates: map[ledgercore.AccountApp]struct{}{
			{Address: test.AccountA, App: appID}: {},
		},
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, delta)
		require.NoError(t, err)

		return tx.Commit()
	}
	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	var addr []byte
	var app uint64
	var localstate []byte
	var deleted bool
	var createdAt uint64
	var closedAt uint64

	row := db.db.QueryRow("SELECT * FROM account_app")
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
	_, connStr, shutdownFunc := setupPostgres(t)
	defer shutdownFunc()
	db, err := OpenPostgres(connStr, idb.IndexerDbOptions{}, nil)
	assert.NoError(t, err)

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(2)
	block.BlockHeader.TimeStamp = 333
	block.BlockHeader.RewardsLevel = 111111
	block.Payset = make([]transactions.SignedTxnInBlock, 2)
	block.Payset[0] = transactions.SignedTxnInBlock{
		SignedTxnWithAD: test.MakePaymentTxn(
			1000, 1, 0, 0, 0, 0, test.AccountA, test.AccountB, basics.Address{},
			basics.Address{}),
	}
	block.Payset[1] = transactions.SignedTxnInBlock{
		SignedTxnWithAD: test.MakeCreateAssetTxn(
			100, 1, false, "ma", "myasset", "myasset.com", test.AccountA),
	}

	f := func(ctx context.Context, tx *sql.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)
		defer w.Close()

		err = w.AddBlock(block, block.Payset, ledgercore.StateDelta{})
		require.NoError(t, err)

		return tx.Commit()
	}

	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)

	err = db.txWithRetry(context.Background(), serializable, f)
	require.NoError(t, err)
}
