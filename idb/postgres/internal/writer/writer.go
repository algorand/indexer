package writer

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/protocol"
	"github.com/jackc/pgx/v4"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
)

const (
	addBlockHeaderStmtName             = "add_block_header"
	setSpecialAccountsStmtName         = "set_special_accounts"
	upsertAssetStmtName                = "upsert_asset"
	upsertAccountAssetStmtName         = "upsert_account_asset"
	upsertAppStmtName                  = "upsert_app"
	upsertAccountAppStmtName           = "upsert_account_app"
	deleteAccountStmtName              = "delete_account"
	deleteAccountUpdateKeytypeStmtName = "delete_account_update_keytype"
	upsertAccountStmtName              = "upsert_account"
	upsertAccountWithKeytypeStmtName   = "upsert_account_with_keytype"
	deleteAssetStmtName                = "delete_asset"
	deleteAccountAssetStmtName         = "delete_account_asset"
	deleteAppStmtName                  = "delete_app"
	deleteAccountAppStmtName           = "delete_account_app"
	updateAccountTotalsStmtName        = "update_account_totals"
)

var statements = map[string]string{
	addBlockHeaderStmtName: `INSERT INTO block_header
		(round, realtime, rewardslevel, header)
		VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
	setSpecialAccountsStmtName: `INSERT INTO metastate (k, v) VALUES ('` +
		schema.SpecialAccountsMetastateKey +
		`', $1) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`,
	upsertAssetStmtName: `INSERT INTO asset
		(index, creator_addr, params, deleted, created_at)
		VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (index) DO UPDATE SET
		creator_addr = EXCLUDED.creator_addr, params = EXCLUDED.params, deleted = FALSE`,
	upsertAccountAssetStmtName: `INSERT INTO account_asset
		(addr, assetid, amount, frozen, deleted, created_at)
		VALUES($1, $2, $3, $4, FALSE, $5) ON CONFLICT (addr, assetid) DO UPDATE SET
		amount = EXCLUDED.amount, frozen = EXCLUDED.frozen, deleted = FALSE`,
	upsertAppStmtName: `INSERT INTO app
		(index, creator, params, deleted, created_at)
		VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (index) DO UPDATE SET
		creator = EXCLUDED.creator, params = EXCLUDED.params, deleted = FALSE`,
	upsertAccountAppStmtName: `INSERT INTO account_app
		(addr, app, localstate, deleted, created_at)
		VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (addr, app) DO UPDATE SET
		localstate = EXCLUDED.localstate, deleted = FALSE`,
	deleteAccountStmtName: `INSERT INTO account
		(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, closed_at,
		 account_data)
		VALUES($1, 0, 0, 0, TRUE, $2, $2, 'null'::jsonb) ON CONFLICT (addr) DO UPDATE SET
		microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase,
		rewards_total = EXCLUDED.rewards_total, deleted = TRUE,
		closed_at = EXCLUDED.closed_at, account_data = EXCLUDED.account_data`,
	deleteAccountUpdateKeytypeStmtName: `INSERT INTO account
		(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, closed_at,
		 keytype, account_data)
		VALUES($1, 0, 0, 0, TRUE, $2, $2, $3, 'null'::jsonb) ON CONFLICT (addr) DO UPDATE SET
		microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase,
		rewards_total = EXCLUDED.rewards_total, deleted = TRUE,
		closed_at = EXCLUDED.closed_at, keytype = EXCLUDED.keytype,
		account_data = EXCLUDED.account_data`,
	upsertAccountStmtName: `INSERT INTO account
		(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, account_data)
		VALUES($1, $2, $3, $4, FALSE, $5, $6) ON CONFLICT (addr) DO UPDATE SET
		microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase,
		rewards_total = EXCLUDED.rewards_total, deleted = FALSE,
		account_data = EXCLUDED.account_data`,
	upsertAccountWithKeytypeStmtName: `INSERT INTO account
		(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, keytype,
		 account_data)
		VALUES($1, $2, $3, $4, FALSE, $5, $6, $7) ON CONFLICT (addr) DO UPDATE SET
		microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase,
		rewards_total = EXCLUDED.rewards_total, deleted = FALSE, keytype = EXCLUDED.keytype,
		account_data = EXCLUDED.account_data`,
	deleteAssetStmtName: `INSERT INTO asset
		(index, creator_addr, params, deleted, created_at, closed_at)
		VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (index) DO UPDATE SET
		creator_addr = EXCLUDED.creator_addr, params = EXCLUDED.params, deleted = TRUE,
		closed_at = EXCLUDED.closed_at`,
	deleteAccountAssetStmtName: `INSERT INTO account_asset
		(addr, assetid, amount, frozen, deleted, created_at, closed_at)
		VALUES($1, $2, 0, false, TRUE, $3, $3) ON CONFLICT (addr, assetid) DO UPDATE SET
		amount = EXCLUDED.amount, deleted = TRUE, closed_at = EXCLUDED.closed_at`,
	deleteAppStmtName: `INSERT INTO app
		(index, creator, params, deleted, created_at, closed_at)
		VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (index) DO UPDATE SET
		creator = EXCLUDED.creator, params = EXCLUDED.params, deleted = TRUE,
		closed_at = EXCLUDED.closed_at`,
	deleteAccountAppStmtName: `INSERT INTO account_app
		(addr, app, localstate, deleted, created_at, closed_at)
		VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (addr, app) DO UPDATE SET
		localstate = EXCLUDED.localstate, deleted = TRUE, closed_at = EXCLUDED.closed_at`,
	updateAccountTotalsStmtName: `UPDATE metastate SET v = $1 WHERE k = '` +
		schema.AccountTotals + `'`,
}

// Writer is responsible for writing blocks and accounting state deltas to the database.
type Writer struct {
	tx pgx.Tx
}

// MakeWriter creates a Writer object.
func MakeWriter(tx pgx.Tx) (Writer, error) {
	w := Writer{
		tx: tx,
	}

	for name, query := range statements {
		_, err := tx.Prepare(context.Background(), name, query)
		if err != nil {
			return Writer{}, fmt.Errorf("MakeWriter() prepare statement err: %w", err)
		}
	}

	return w, nil
}

// Close shuts down Writer.
func (w *Writer) Close() {
	for name := range statements {
		w.tx.Conn().Deallocate(context.Background(), name)
	}
}

func addBlockHeader(blockHeader *bookkeeping.BlockHeader, batch *pgx.Batch) {
	batch.Queue(
		addBlockHeaderStmtName,
		uint64(blockHeader.Round), time.Unix(blockHeader.TimeStamp, 0).UTC(),
		blockHeader.RewardsLevel, encoding.EncodeBlockHeader(*blockHeader))
}

func setSpecialAccounts(addresses transactions.SpecialAddresses, batch *pgx.Batch) {
	j := encoding.EncodeSpecialAddresses(addresses)
	batch.Queue(setSpecialAccountsStmtName, j)
}

// Describes a change to the `account.keytype` column. If `present` is true,
// `value` is the new value. Otherwise, NULL will be the new value.
type sigTypeDelta struct {
	present bool
	value   idb.SigType
}

func getSigTypeDeltas(payset []transactions.SignedTxnInBlock) (map[basics.Address]sigTypeDelta, error) {
	res := make(map[basics.Address]sigTypeDelta, len(payset))

	for i := range payset {
		if payset[i].Txn.RekeyTo == (basics.Address{}) && payset[i].Txn.Type != protocol.StateProofTx {
			sigtype, err := idb.SignatureType(&payset[i].SignedTxn)
			if err != nil {
				return nil, fmt.Errorf("getSigTypeDelta() err: %w", err)
			}
			res[payset[i].Txn.Sender] = sigTypeDelta{present: true, value: sigtype}
		} else {
			res[payset[i].Txn.Sender] = sigTypeDelta{}
		}
	}

	return res, nil
}

type optionalSigTypeDelta struct {
	present bool
	value   sigTypeDelta
}

func writeAccount(round basics.Round, address basics.Address, accountData ledgercore.AccountData, sigtypeDelta optionalSigTypeDelta, batch *pgx.Batch) {
	sigtypeFunc := func(delta sigTypeDelta) *idb.SigType {
		if !delta.present {
			return nil
		}

		res := new(idb.SigType)
		*res = delta.value
		return res
	}

	if accountData.IsZero() {
		// Delete account.
		if sigtypeDelta.present {
			batch.Queue(
				deleteAccountUpdateKeytypeStmtName,
				address[:], uint64(round), sigtypeFunc(sigtypeDelta.value))
		} else {
			batch.Queue(deleteAccountStmtName, address[:], uint64(round))
		}
	} else {
		// Update account.
		accountDataJSON :=
			encoding.EncodeTrimmedLcAccountData(encoding.TrimLcAccountData(accountData))

		if sigtypeDelta.present {
			batch.Queue(
				upsertAccountWithKeytypeStmtName,
				address[:], accountData.MicroAlgos.Raw, accountData.RewardsBase,
				accountData.RewardedMicroAlgos.Raw, uint64(round),
				sigtypeFunc(sigtypeDelta.value), accountDataJSON)
		} else {
			batch.Queue(
				upsertAccountStmtName,
				address[:], accountData.MicroAlgos.Raw, accountData.RewardsBase,
				accountData.RewardedMicroAlgos.Raw, uint64(round),
				accountDataJSON)
		}
	}
}

func writeAssetResource(round basics.Round, resource *ledgercore.AssetResourceRecord, batch *pgx.Batch) {
	if resource.Params.Deleted {
		batch.Queue(deleteAssetStmtName, resource.Aidx, resource.Addr[:], round)
	} else {
		if resource.Params.Params != nil {
			batch.Queue(
				upsertAssetStmtName, resource.Aidx, resource.Addr[:],
				encoding.EncodeAssetParams(*resource.Params.Params), round)
		}
	}

	if resource.Holding.Deleted {
		batch.Queue(deleteAccountAssetStmtName, resource.Addr[:], resource.Aidx, round)
	} else {
		if resource.Holding.Holding != nil {
			batch.Queue(
				upsertAccountAssetStmtName, resource.Addr[:], resource.Aidx,
				strconv.FormatUint(resource.Holding.Holding.Amount, 10),
				resource.Holding.Holding.Frozen, round)
		}
	}
}

func writeAppResource(round basics.Round, resource *ledgercore.AppResourceRecord, batch *pgx.Batch) {
	if resource.Params.Deleted {
		batch.Queue(deleteAppStmtName, resource.Aidx, resource.Addr[:], round)
	} else {
		if resource.Params.Params != nil {
			batch.Queue(
				upsertAppStmtName, resource.Aidx, resource.Addr[:],
				encoding.EncodeAppParams(*resource.Params.Params), round)
		}
	}

	if resource.State.Deleted {
		batch.Queue(deleteAccountAppStmtName, resource.Addr[:], resource.Aidx, round)
	} else {
		if resource.State.LocalState != nil {
			batch.Queue(
				upsertAccountAppStmtName, resource.Addr[:], resource.Aidx,
				encoding.EncodeAppLocalState(*resource.State.LocalState), round)
		}
	}
}

func writeAccountDeltas(round basics.Round, accountDeltas *ledgercore.AccountDeltas, sigtypeDeltas map[basics.Address]sigTypeDelta, batch *pgx.Batch) {
	// Update `account` table.
	for i := 0; i < accountDeltas.Len(); i++ {
		address, accountData := accountDeltas.GetByIdx(i)

		var sigtypeDelta optionalSigTypeDelta
		sigtypeDelta.value, sigtypeDelta.present = sigtypeDeltas[address]

		writeAccount(round, address, accountData, sigtypeDelta, batch)
	}

	// Update `asset` and `account_asset` tables.
	{
		assetResources := accountDeltas.GetAllAssetResources()
		for i := range assetResources {
			writeAssetResource(round, &assetResources[i], batch)
		}
	}

	// Update `app` and `account_app` tables.
	{
		appResources := accountDeltas.GetAllAppResources()
		for i := range appResources {
			writeAppResource(round, &appResources[i], batch)
		}
	}
}

// AddBlock0 writes block 0 to the database.
func (w *Writer) AddBlock0(block *bookkeeping.Block) error {
	var batch pgx.Batch

	addBlockHeader(&block.BlockHeader, &batch)
	specialAddresses := transactions.SpecialAddresses{
		FeeSink:     block.FeeSink,
		RewardsPool: block.RewardsPool,
	}
	setSpecialAccounts(specialAddresses, &batch)

	results := w.tx.SendBatch(context.Background(), &batch)
	// Clean the results off the connection's queue. Without this, weird things happen.
	for i := 0; i < batch.Len(); i++ {
		_, err := results.Exec()
		if err != nil {
			results.Close()
			return fmt.Errorf("AddBlock() exec err: %w", err)
		}
	}
	err := results.Close()
	if err != nil {
		return fmt.Errorf("AddBlock() close results err: %w", err)
	}

	return nil
}

// AddBlock writes the block and accounting state deltas to the database, except for
// transactions and transaction participation. Those are imported by free functions in
// the writer/ directory.
func (w *Writer) AddBlock(block *bookkeeping.Block, modifiedTxns []transactions.SignedTxnInBlock, delta ledgercore.StateDelta) error {
	var batch pgx.Batch

	addBlockHeader(&block.BlockHeader, &batch)
	specialAddresses := transactions.SpecialAddresses{
		FeeSink:     block.FeeSink,
		RewardsPool: block.RewardsPool,
	}
	setSpecialAccounts(specialAddresses, &batch)
	{
		sigTypeDeltas, err := getSigTypeDeltas(block.Payset)
		if err != nil {
			return fmt.Errorf("AddBlock() err: %w", err)
		}
		writeAccountDeltas(block.Round(), &delta.Accts, sigTypeDeltas, &batch)
	}
	batch.Queue(updateAccountTotalsStmtName, encoding.EncodeAccountTotals(&delta.Totals))

	results := w.tx.SendBatch(context.Background(), &batch)
	// Clean the results off the connection's queue. Without this, weird things happen.
	for i := 0; i < batch.Len(); i++ {
		_, err := results.Exec()
		if err != nil {
			results.Close()
			return fmt.Errorf("AddBlock() exec err: %w", err)
		}
	}
	err := results.Close()
	if err != nil {
		return fmt.Errorf("AddBlock() close results err: %w", err)
	}

	return nil
}
