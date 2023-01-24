package writer

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/jackc/pgx/v4"

	"github.com/algorand/avm-abi/apps"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
	"github.com/algorand/indexer/types"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
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
	upsertAppBoxStmtName               = "upsert_app_box"
	deleteAppBoxStmtName               = "delete_app_box"
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
	upsertAppBoxStmtName: `INSERT INTO app_box AS ab
		(app, name, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (app, name) DO UPDATE SET
		value = EXCLUDED.value`,
	deleteAppBoxStmtName: `DELETE FROM app_box WHERE app = $1 and name = $2`,
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
			return Writer{}, fmt.Errorf("MakeWriter() prepare statement for name '%s' err: %w", name, err)
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

func addBlockHeader(blockHeader *sdk.BlockHeader, batch *pgx.Batch) {
	batch.Queue(
		addBlockHeaderStmtName,
		uint64(blockHeader.Round), time.Unix(blockHeader.TimeStamp, 0).UTC(),
		blockHeader.RewardsLevel, encoding.EncodeBlockHeader(*blockHeader))
}

func setSpecialAccounts(addresses types.SpecialAddresses, batch *pgx.Batch) {
	j := encoding.EncodeSpecialAddresses(addresses)
	batch.Queue(setSpecialAccountsStmtName, j)
}

// Describes a change to the `account.keytype` column. If `present` is true,
// `value` is the new value. Otherwise, NULL will be the new value.
type sigTypeDelta struct {
	present bool
	value   idb.SigType
}

func getSigTypeDeltas(payset []sdk.SignedTxnInBlock) (map[sdk.Address]sigTypeDelta, error) {
	res := make(map[sdk.Address]sigTypeDelta, len(payset))

	for i := range payset {
		if payset[i].Txn.RekeyTo == (sdk.Address{}) && payset[i].Txn.Type != sdk.StateProofTx {
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

func writeAccount(round sdk.Round, address sdk.Address, accountData models.Account, sigtypeDelta optionalSigTypeDelta, batch *pgx.Batch) {
	sigtypeFunc := func(delta sigTypeDelta) *idb.SigType {
		if !delta.present {
			return nil
		}

		res := new(idb.SigType)
		*res = delta.value
		return res
	}

	if reflect.DeepEqual(accountData, models.Account{}) {
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
			encoding.EncodeTrimmedLcAccountData(encoding.TrimAccountData(accountData))

		if sigtypeDelta.present {
			batch.Queue(
				upsertAccountWithKeytypeStmtName,
				address[:], accountData.Amount, accountData.RewardBase,
				accountData.Rewards, uint64(round),
				sigtypeFunc(sigtypeDelta.value), accountDataJSON)
		} else {
			batch.Queue(
				upsertAccountStmtName,
				address[:], accountData.Amount, accountData.RewardBase,
				accountData.Rewards, uint64(round),
				accountDataJSON)
		}
	}
}

func writeAssetResource(round sdk.Round, resource *models.AssetResourceRecord, batch *pgx.Batch) {
	if resource.AssetDeleted {
		batch.Queue(deleteAssetStmtName, resource.AssetIndex, resource.Address[:], round)
	} else {
		if !reflect.DeepEqual(resource.AssetParams, models.AssetParams{}) {
			// convert models.AssetParams to types.AssetParams
			sdkAssetParams := convertModelAssetParams(resource.AssetParams)
			batch.Queue(
				upsertAssetStmtName, resource.AssetIndex, resource.Address[:],
				encoding.EncodeAssetParams(sdkAssetParams), round)
		}
	}

	if resource.AssetHoldingDeleted {
		batch.Queue(deleteAccountAssetStmtName, resource.Address[:], resource.AssetIndex, round)
	} else {
		if !reflect.DeepEqual(resource.AssetHolding, models.AssetHolding{}) {
			batch.Queue(
				upsertAccountAssetStmtName, resource.Address[:], resource.AssetIndex,
				strconv.FormatUint(resource.AssetHolding.Amount, 10),
				resource.AssetHolding.IsFrozen, round)
		}
	}
}

func convertModelAssetParams(params models.AssetParams) sdk.AssetParams {
	var metaDataHash [32]byte
	copy(metaDataHash[:], params.MetadataHash)
	managerAddr, _ := sdk.DecodeAddress(params.Manager)
	reserveAddr, _ := sdk.DecodeAddress(params.Reserve)
	freezeAddr, _ := sdk.DecodeAddress(params.Freeze)
	clawbackAddr, _ := sdk.DecodeAddress(params.Clawback)
	return sdk.AssetParams{
		Total:         params.Total,
		Decimals:      uint32(params.Decimals),
		DefaultFrozen: params.DefaultFrozen,
		UnitName:      params.UnitName,
		AssetName:     params.Name,
		URL:           params.Url,
		MetadataHash:  metaDataHash,
		Manager:       managerAddr,
		Reserve:       reserveAddr,
		Freeze:        freezeAddr,
		Clawback:      clawbackAddr,
	}
}
func writeAppResource(round sdk.Round, resource *models.AppResourceRecord, batch *pgx.Batch) {
	if resource.AppDeleted {
		batch.Queue(deleteAppStmtName, resource.AppIndex, resource.Address[:], round)
	} else {
		if !reflect.DeepEqual(resource.AppParams, models.ApplicationParams{}) {
			batch.Queue(
				upsertAppStmtName, resource.AppIndex, resource.Address[:],
				encoding.EncodeAppParams(resource.AppParams), round)
		}
	}

	if resource.AppLocalState.Deleted {
		batch.Queue(deleteAccountAppStmtName, resource.Address[:], resource.AppIndex, round)
	} else {
		if !reflect.DeepEqual(resource.AppLocalState, models.ApplicationLocalState{}) {
			batch.Queue(
				upsertAccountAppStmtName, resource.Address[:], resource.AppIndex,
				encoding.EncodeAppLocalState(resource.AppLocalState), round)
		}
	}
}

func writeAccountDeltas(round sdk.Round, accountDeltas *models.AccountDeltas, sigtypeDeltas map[sdk.Address]sigTypeDelta, batch *pgx.Batch) {
	// Update `account` table.
	for i := 0; i < len(accountDeltas.Accounts); i++ {

		address := accountDeltas.Accounts[i].Address
		accountData := accountDeltas.Accounts[i].AccountData

		var sigtypeDelta optionalSigTypeDelta
		addr, _ := sdk.DecodeAddress(address)
		sigtypeDelta.value, sigtypeDelta.present = sigtypeDeltas[addr]

		writeAccount(round, addr, accountData, sigtypeDelta, batch)
	}

	// Update `asset` and `account_asset` tables.
	{
		for _, assetResource := range accountDeltas.Assets {
			writeAssetResource(round, &assetResource, batch)
		}
	}

	// Update `app` and `account_app` tables.
	{
		for _, appResource := range accountDeltas.Apps {
			writeAppResource(round, &appResource, batch)
		}
	}

}

func writeBoxMods(kvMods []models.KvDelta, batch *pgx.Batch) error {
	// INSERT INTO / UPDATE / DELETE FROM `app_box`
	// WARNING: kvMods can in theory support more general storage types than app boxes.
	// However, here we assume that all the provided kvMods represent app boxes.
	// If a non-box is encountered inside kvMods, an error will be returned and
	// AddBlock() will fail with the import getting stuck at the corresponding round.
	for _, kvDelta := range kvMods {
		app, name, err := apps.SplitBoxKey(string(kvDelta.Key))
		if err != nil {
			return fmt.Errorf("writeBoxMods() err: %w", err)
		}
		if kvDelta.Value != nil {
			batch.Queue(upsertAppBoxStmtName, app, []byte(name), kvDelta.Value)
		} else {
			batch.Queue(deleteAppBoxStmtName, app, []byte(name))
		}
	}

	return nil
}

// AddBlock0 writes block 0 to the database.
func (w *Writer) AddBlock0(block *sdk.Block) error {
	var batch pgx.Batch

	addBlockHeader(&block.BlockHeader, &batch)
	specialAddresses := types.SpecialAddresses{
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
			return fmt.Errorf("AddBlock0() exec err: %w", err)
		}
	}
	err := results.Close()
	if err != nil {
		return fmt.Errorf("AddBlock0() close results err: %w", err)
	}

	return nil
}

// AddBlock writes the block and accounting state deltas to the database, except for
// transactions and transaction participation. Those are imported by free functions in
// the writer/ directory.
func (w *Writer) AddBlock(block *sdk.Block, delta models.LedgerStateDelta) error {
	var batch pgx.Batch
	addBlockHeader(&block.BlockHeader, &batch)
	specialAddresses := types.SpecialAddresses{
		FeeSink:     block.FeeSink,
		RewardsPool: block.RewardsPool,
	}
	setSpecialAccounts(specialAddresses, &batch)
	{
		sigTypeDeltas, err := getSigTypeDeltas(block.Payset)
		if err != nil {
			return fmt.Errorf("AddBlock() err: %w", err)
		}
		writeAccountDeltas(block.Round, &delta.Accts, sigTypeDeltas, &batch)
	}
	{
		err := writeBoxMods(delta.KvMods, &batch)
		if err != nil {
			return fmt.Errorf("AddBlock() err on boxes: %w", err)
		}
	}

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
