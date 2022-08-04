package writer_test

import (
	"context"
	"math"
	"testing"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
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
	addr  basics.Address
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

func TestWriterSpecialAccounts(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	block := test.MakeGenesisBlock()

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, ledgercore.StateDelta{})
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

	expected := transactions.SpecialAddresses{
		FeeSink:     test.FeeAddr,
		RewardsPool: test.RewardAddr,
	}
	assert.Equal(t, expected, accounts)
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

	var block bookkeeping.Block
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

		err = w.AddBlock(&block, block.Payset, delta)
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
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
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)
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

	var block bookkeeping.Block
	block.BlockHeader.Round = 4

	var delta ledgercore.StateDelta
	delta.Accts.Upsert(test.AccountA, ledgercore.AccountData{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, delta)
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
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
	delta.Accts.Upsert(test.AccountA, ledgercore.AccountData{
		AccountBaseData: ledgercore.AccountBaseData{
			MicroAlgos: basics.MicroAlgos{Raw: 5},
		},
	})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, delta)
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
	block.BlockHeader.Round = basics.Round(5)
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

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

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

		err = w.AddBlock(&block, block.Payset, delta)
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
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
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an asset holding is added and deleted in the same round.
func TestWriterAccountAssetTableCreateDeleteSameRound(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{},
		ledgercore.AssetHoldingDelta{Deleted: true})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, delta)
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
}

func TestWriterAccountAssetTableLargeAmount(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

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

		err = w.AddBlock(&block, block.Payset, delta)
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

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

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

		err = w.AddBlock(&block, block.Payset, delta)
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
		assert.Equal(t, assetParams, paramsRead)
	}
	assert.False(t, deleted)
	assert.Equal(t, block.Round(), basics.Round(createdAt))
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
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	assetID := basics.AssetIndex(3)
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAssetResource(
		test.AccountA, assetID, ledgercore.AssetParamsDelta{Deleted: true},
		ledgercore.AssetHoldingDelta{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, delta)
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
		assert.Equal(t, basics.AssetParams{}, paramsRead)
	}
	assert.True(t, deleted)
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
}

func TestWriterAppTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
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
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{Params: &appParams},
		ledgercore.AppLocalStateDelta{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, delta)
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
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
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an app is added and deleted in the same round.
func TestWriterAppTableCreateDeleteSameRound(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	appID := basics.AppIndex(3)
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{Deleted: true},
		ledgercore.AppLocalStateDelta{})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, delta)
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
}

func TestWriterAccountAppTableBasic(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
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
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{},
		ledgercore.AppLocalStateDelta{LocalState: &appLocalState})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, delta)
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
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
	assert.Equal(t, uint64(block.Round())-1, createdAt)
	require.NotNil(t, closedAt)
	assert.Equal(t, uint64(block.Round()), *closedAt)

	assert.False(t, rows.Next())
	assert.NoError(t, rows.Err())
}

// Simulate a scenario where an account app is added and deleted in the same round.
func TestWriterAccountAppTableCreateDeleteSameRound(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	var block bookkeeping.Block
	block.BlockHeader.Round = basics.Round(1)

	appID := basics.AppIndex(3)
	var delta ledgercore.StateDelta
	delta.Accts.UpsertAppResource(
		test.AccountA, appID, ledgercore.AppParamsDelta{},
		ledgercore.AppLocalStateDelta{Deleted: true})

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, delta)
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
	assert.Equal(t, block.Round(), basics.Round(createdAt))
	assert.Equal(t, block.Round(), basics.Round(closedAt))
}

func TestWriterAccountTotals(t *testing.T) {
	db, _, shutdownFunc := pgtest.SetupPostgresWithSchema(t)
	defer shutdownFunc()

	// Set empty account totals.
	err := pgutil.SetMetastate(db, nil, schema.AccountTotals, "{}")
	require.NoError(t, err)

	block := test.MakeGenesisBlock()

	accountTotals := ledgercore.AccountTotals{
		Online: ledgercore.AlgoCount{
			Money: basics.MicroAlgos{Raw: 33},
		},
	}

	f := func(tx pgx.Tx) error {
		w, err := writer.MakeWriter(tx)
		require.NoError(t, err)

		err = w.AddBlock(&block, block.Payset, ledgercore.StateDelta{Totals: accountTotals})
		require.NoError(t, err)

		w.Close()
		return nil
	}
	err = pgutil.TxWithRetry(db, serializable, f, nil)
	require.NoError(t, err)

	j, err := pgutil.GetMetastate(
		context.Background(), db, nil, schema.AccountTotals)
	require.NoError(t, err)
	accountTotalsRead, err := encoding.DecodeAccountTotals([]byte(j))
	require.NoError(t, err)

	assert.Equal(t, accountTotals, accountTotalsRead)
}
