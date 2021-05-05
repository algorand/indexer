package writer

import (
	"database/sql"
	"encoding/base32"
	"fmt"
	"strconv"
	"time"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/protocol"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
)

const addBlockHeaderQuery = "INSERT INTO block_header " +
	"(round, realtime, rewardslevel, header) VALUES " +
	"($1, $2, $3, $4) ON CONFLICT DO NOTHING"
const addTxnQuery = "INSERT INTO txn " +
	"(round, intra, typeenum, asset, txid, txnbytes, txn, extra) " +
	"VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT DO NOTHING"
const addTxnParticipantQuery = "INSERT INTO txn_participation " +
	"(addr, round, intra) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING"
const updateAssetQuery = "INSERT INTO asset " +
	"(index, creator_addr, params, deleted, created_at) " +
	"VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (index) DO UPDATE SET " +
	"creator_addr = EXCLUDED.creator_addr, params = EXCLUDED.params, deleted = FALSE"
const updateAccountAssetQuery = "INSERT INTO account_asset " +
	"(addr, assetid, amount, frozen, deleted, created_at) " +
	"VALUES($1, $2, $3, $4, FALSE, $5) ON CONFLICT (addr, assetid) DO UPDATE SET " +
	"amount = EXCLUDED.amount, frozen = EXCLUDED.frozen, deleted = FALSE"
const updateAppQuery = "INSERT INTO app " +
	"(index, creator, params, deleted, created_at) " +
	"VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (index) DO UPDATE SET " +
	"creator = EXCLUDED.creator, params = EXCLUDED.params, deleted = FALSE"
const updateAccountAppQuery = "INSERT INTO account_app " +
	"(addr, app, localstate, deleted, created_at) " +
	"VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (addr, app) DO UPDATE SET " +
	"localstate = EXCLUDED.localstate, deleted = FALSE"
const deleteAccountQuery = "INSERT INTO account " +
	"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, closed_at) " +
	"VALUES($1, 0, 0, 0, TRUE, $2, $2) ON CONFLICT (addr) DO UPDATE SET " +
	"microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase, " +
	"rewards_total = EXCLUDED.rewards_total, deleted = TRUE, " +
	"closed_at = EXCLUDED.closed_at, account_data = EXCLUDED.account_data"
const updateAccountQuery = "INSERT INTO account " +
	"(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, account_data) " +
	"VALUES($1, $2, $3, $4, FALSE, $5, $6) ON CONFLICT (addr) DO UPDATE SET " +
	"microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase, " +
	"rewards_total = EXCLUDED.rewards_total, deleted = FALSE, " +
	"account_data = EXCLUDED.account_data"
const deleteAssetQuery = "INSERT INTO asset " +
	"(index, creator_addr, params, deleted, created_at, closed_at) " +
	"VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (index) DO UPDATE SET " +
	"creator_addr = EXCLUDED.creator_addr, params = EXCLUDED.params, deleted = TRUE, " +
	"closed_at = EXCLUDED.closed_at"
const deleteAccountAssetQuery = "INSERT INTO account_asset " +
	"(addr, assetid, amount, frozen, deleted, created_at, closed_at) " +
	"VALUES($1, $2, 0, false, TRUE, $3, $3) ON CONFLICT (addr, assetid) DO UPDATE SET " +
	"amount = EXCLUDED.amount, deleted = TRUE, closed_at = EXCLUDED.closed_at"
const deleteAppQuery = "INSERT INTO app " +
	"(index, creator, params, deleted, created_at, closed_at) " +
	"VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (index) DO UPDATE SET " +
	"creator = EXCLUDED.creator, params = EXCLUDED.params, deleted = TRUE, " +
	"closed_at = EXCLUDED.closed_at"
const deleteAccountAppQuery = "INSERT INTO account_app " +
	"(addr, app, localstate, deleted, created_at, closed_at) " +
	"VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (addr, app) DO UPDATE SET " +
	"localstate = EXCLUDED.localstate, deleted = TRUE, closed_at = EXCLUDED.closed_at"
const updateAccountKeyTypeQuery = "UPDATE account SET keytype = $1 WHERE addr = $2"

type Writer struct {
	tx *sql.Tx

	addBlockHeaderStmt       *sql.Stmt
	addTxnStmt               *sql.Stmt
	addTxnParticipantStmt    *sql.Stmt
	updateAssetStmt          *sql.Stmt
	updateAccountAssetStmt   *sql.Stmt
	updateAppStmt            *sql.Stmt
	updateAccountAppStmt     *sql.Stmt
	deleteAccountStmt        *sql.Stmt
	updateAccountStmt        *sql.Stmt
	deleteAssetStmt          *sql.Stmt
	deleteAccountAssetStmt   *sql.Stmt
	deleteAppStmt            *sql.Stmt
	deleteAccountAppStmt     *sql.Stmt
	updateAccountKeyTypeStmt *sql.Stmt
}

func MakeWriter(tx *sql.Tx) (Writer, error) {
	w := Writer{
		tx: tx,
	}

	var err error

	w.addBlockHeaderStmt, err = tx.Prepare(addBlockHeaderQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare add block header stmt err: %w", err)
	}
	w.addTxnStmt, err = tx.Prepare(addTxnQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare add txn stmt err: %w", err)
	}
	w.addTxnParticipantStmt, err = tx.Prepare(addTxnParticipantQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare add txn participant stmt err: %w", err)
	}
	w.updateAssetStmt, err = tx.Prepare(updateAssetQuery)
	if err != nil {
		return Writer{}, fmt.Errorf("MakeWriter(): prepare update asset stmt err: %w", err)
	}
	w.updateAccountAssetStmt, err = tx.Prepare(updateAccountAssetQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare update account asset stmt err: %w", err)
	}
	w.updateAppStmt, err = tx.Prepare(updateAppQuery)
	if err != nil {
		return Writer{}, fmt.Errorf("MakeWriter(): prepare update app stmt err: %w", err)
	}
	w.updateAccountAppStmt, err = tx.Prepare(updateAccountAppQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare update account app stmt err: %w", err)
	}
	w.deleteAccountStmt, err = tx.Prepare(deleteAccountQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare delete account stmt err: %w", err)
	}
	w.updateAccountStmt, err = tx.Prepare(updateAccountQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare update account stmt err: %w", err)
	}
	w.deleteAssetStmt, err = tx.Prepare(deleteAssetQuery)
	if err != nil {
		return Writer{}, fmt.Errorf("MakeWriter(): prepare delete asset stmt err: %w", err)
	}
	w.deleteAccountAssetStmt, err = tx.Prepare(deleteAccountAssetQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare delete account asset stmt err: %w", err)
	}
	w.deleteAppStmt, err = tx.Prepare(deleteAppQuery)
	if err != nil {
		return Writer{}, fmt.Errorf("MakeWriter(): prepare delete app stmt err: %w", err)
	}
	w.deleteAccountAppStmt, err = tx.Prepare(deleteAccountAppQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare delete account app stmt err: %w", err)
	}
	w.updateAccountKeyTypeStmt, err = tx.Prepare(updateAccountKeyTypeQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare update account sig type stmt err: %w", err)
	}

	return w, nil
}

func (w *Writer) Close() {
	w.addBlockHeaderStmt.Close()
	w.addTxnStmt.Close()
	w.addTxnParticipantStmt.Close()
	w.updateAssetStmt.Close()
	w.updateAccountAssetStmt.Close()
	w.updateAppStmt.Close()
	w.updateAccountAppStmt.Close()
	w.deleteAccountStmt.Close()
	w.updateAccountStmt.Close()
	w.deleteAssetStmt.Close()
	w.deleteAccountAssetStmt.Close()
	w.deleteAppStmt.Close()
	w.deleteAccountAppStmt.Close()
	w.updateAccountKeyTypeStmt.Close()
}

func (w *Writer) addBlockHeader(blockHeader bookkeeping.BlockHeader) error {
	_, err := w.addBlockHeaderStmt.Exec(
		uint64(blockHeader.Round), time.Unix(blockHeader.TimeStamp, 0).UTC(),
		blockHeader.RewardsLevel, encoding.EncodeBlockHeader(blockHeader))
	if err != nil {
		return fmt.Errorf("addBlockHeader() err: %w", err)
	}
	return nil
}

func transactionAsset(block *bookkeeping.Block, intra uint64, typeenum idb.TxnTypeEnum) uint64 {
	assetid := uint64(0)
	txn := block.Payset[intra].Txn

	switch typeenum {
	case idb.TypeEnumAssetConfig:
		assetid = uint64(txn.ConfigAsset)
		if assetid == 0 {
			assetid = block.TxnCounter - uint64(len(block.Payset)) + uint64(intra) + 1
		}
	case idb.TypeEnumAssetTransfer:
		assetid = uint64(txn.XferAsset)
	case idb.TypeEnumAssetFreeze:
		assetid = uint64(txn.FreezeAsset)
	case idb.TypeEnumApplication:
		assetid = uint64(txn.ApplicationID)
		if assetid == 0 {
			assetid = block.TxnCounter - uint64(len(block.Payset)) + uint64(intra) + 1
		}
	}

	return assetid
}

// Add transactions from `block` to the database. `modifiedTxns` contains enhanced
// apply data generated by evaluator.
func (w *Writer) addTransactions(block *bookkeeping.Block, modifiedTxns []transactions.SignedTxnInBlock) error {
	for i, stib := range block.Payset {
		var stxnad transactions.SignedTxnWithAD
		var err error
		// This function makes sure to set correct genesis information so we can get the
		// correct transaction hash.
		stxnad.SignedTxn, stxnad.ApplyData, err = block.BlockHeader.DecodeSignedTxn(stib)
		if err != nil {
			return fmt.Errorf("addTransactions() decode signed txn err: %w", err)
		}

		txn := stxnad.Txn

		typeenum, ok := idb.GetTypeEnum(txn.Type)
		if !ok {
			return fmt.Errorf("addTransactions() get type enum")
		}

		assetid := transactionAsset(block, uint64(i), typeenum)

		id := txn.ID()
		idStr := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:])

		extra := idb.TxnExtra{
			AssetCloseAmount: modifiedTxns[i].ApplyData.AssetClosingAmount,
		}

		_, err = w.addTxnStmt.Exec(
			uint64(block.Round()), i, int(typeenum), assetid, idStr,
			protocol.Encode(&stxnad),
			encoding.EncodeSignedTxnWithAD(stxnad),
			encoding.EncodeJSON(extra))
		if err != nil {
			return fmt.Errorf("addTransactions() exec err: %w", err)
		}
	}

	return nil
}

func getTransactionParticipants(txn transactions.Transaction) []basics.Address {
	res := make([]basics.Address, 0, 7)

	add := func(address basics.Address) {
		if address.IsZero() {
			return
		}
		for _, p := range res {
			if address == p {
				return
			}
		}
		res = append(res, address)
	}

	add(txn.Sender)
	add(txn.Receiver)
	add(txn.CloseRemainderTo)
	add(txn.AssetSender)
	add(txn.AssetReceiver)
	add(txn.AssetCloseTo)
	add(txn.FreezeAccount)

	return res
}

func (w *Writer) addTransactionParticipation(block *bookkeeping.Block) error {
	for i, stxnad := range block.Payset {
		participants := getTransactionParticipants(stxnad.Txn)

		for _, addr := range participants {
			_, err := w.addTxnParticipantStmt.Exec(addr[:], uint64(block.Round()), i)
			if err != nil {
				return fmt.Errorf("addTransactionParticipation() exec err: %w", err)
			}
		}
	}

	return nil
}

func trimAccountData(ad basics.AccountData) basics.AccountData {
	ad.MicroAlgos = basics.MicroAlgos{}
	ad.RewardsBase = 0
	ad.RewardedMicroAlgos = basics.MicroAlgos{}
	ad.AssetParams = nil
	ad.Assets = nil
	ad.AppLocalStates = nil
	ad.AppParams = nil
	ad.TotalAppSchema = basics.StateSchema{}

	return ad
}

func (w *Writer) writeBalanceRecord(round basics.Round, record basics.BalanceRecord) error {
	// Update `asset` table.
	for assetid, params := range record.AccountData.AssetParams {
		_, err := w.updateAssetStmt.Exec(
			uint64(assetid), record.Addr[:], encoding.EncodeAssetParams(params), uint64(round))
		if err != nil {
			return fmt.Errorf("writeBalanceRecord() exec update asset err: %w", err)
		}
	}

	// Update `account_asset` table.
	for assetid, holding := range record.AccountData.Assets {
		_, err := w.updateAccountAssetStmt.Exec(
			record.Addr[:], uint64(assetid), strconv.FormatUint(holding.Amount, 10),
			holding.Frozen, uint64(round))
		if err != nil {
			return fmt.Errorf("writeBalanceRecord() exec update account asset err: %w", err)
		}
	}

	// Update `app` table.
	for appid, params := range record.AccountData.AppParams {
		_, err := w.updateAppStmt.Exec(
			uint64(appid), record.Addr[:], encoding.EncodeAppParams(params), uint64(round))
		if err != nil {
			return fmt.Errorf("writeBalanceRecord() exec update app err: %w", err)
		}
	}

	// Update `account_app` table.
	for appid, state := range record.AccountData.AppLocalStates {
		_, err := w.updateAccountAppStmt.Exec(
			record.Addr[:], uint64(appid), encoding.EncodeAppLocalState(state), uint64(round))
		if err != nil {
			return fmt.Errorf("writeBalanceRecord() exec update account app err: %w", err)
		}
	}

	// Update `account` table.
	if record.AccountData.IsZero() {
		// Delete account.
		_, err := w.deleteAccountStmt.Exec(record.Addr[:], uint64(round))
		if err != nil {
			return fmt.Errorf("writeBalanceRecord() exec delete account err: %w", err)
		}
	} else {
		// Update account.
		accountDataJSON := encoding.EncodeAccountData(trimAccountData(record.AccountData))
		_, err := w.updateAccountStmt.Exec(
			record.Addr[:], record.AccountData.MicroAlgos.Raw, record.AccountData.RewardsBase,
			record.AccountData.RewardedMicroAlgos.Raw, uint64(round), accountDataJSON)
		if err != nil {
			return fmt.Errorf("writeBalanceRecord() exec update account err: %w", err)
		}
	}

	return nil
}

func (w *Writer) writeBalanceRecords(round basics.Round, records []basics.BalanceRecord, specialAddresses transactions.SpecialAddresses) error {
	// Update `account` table.
	for _, record := range records {
		// Indexer currently doesn't support special accounts.
		// TODO: remove this check.
		if (record.Addr != specialAddresses.FeeSink) &&
			(record.Addr != specialAddresses.RewardsPool) {
			err := w.writeBalanceRecord(round, record)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Writer) writeDeletedCreatables(round basics.Round, creatables map[basics.CreatableIndex]ledgercore.ModifiedCreatable) error {
	for index, creatable := range creatables {
		// If deleted.
		if !creatable.Created {
			if creatable.Ctype == basics.AssetCreatable {
				_, err := w.deleteAssetStmt.Exec(
					uint64(index), creatable.Creator[:], uint64(round))
				if err != nil {
					return fmt.Errorf(
						"writeDeletedCreatables() exec delete asset err: %w", err)
				}
			} else {
				_, err := w.deleteAppStmt.Exec(
					uint64(index), creatable.Creator[:], uint64(round))
				if err != nil {
					return fmt.Errorf(
						"writeDeletedCreatables() exec delete app err: %w", err)
				}
			}
		}
	}

	return nil
}

func (w *Writer) writeDeletedAssetHoldings(round basics.Round, deletedAssetHoldings map[ledgercore.AccountAsset]struct{}) error {
	for aa := range deletedAssetHoldings {
		_, err := w.deleteAccountAssetStmt.Exec(
			aa.Address[:], uint64(aa.Asset), uint64(round))
		if err != nil {
			return fmt.Errorf(
				"writeDeletedAssetHoldings() exec delete account asset err: %w", err)
		}
	}

	return nil
}

func (w *Writer) writeDeletedAppLocalStates(round basics.Round, deletedAppLocalStates map[ledgercore.AccountApp]struct{}) error {
	for aa := range deletedAppLocalStates {
		_, err := w.deleteAccountAppStmt.Exec(aa.Address[:], uint64(aa.App), uint64(round))
		if err != nil {
			return fmt.Errorf(
				"writeDeletedAppLocalStates() exec delete account app err: %w", err)
		}
	}

	return nil
}

func (w *Writer) writeStateDelta(round basics.Round, delta ledgercore.StateDelta, specialAddresses transactions.SpecialAddresses) error {
	err := w.writeBalanceRecords(round, delta.Accts.Accts, specialAddresses)
	if err != nil {
		return err
	}

	err = w.writeDeletedCreatables(round, delta.Creatables)
	if err != nil {
		return err
	}

	err = w.writeDeletedAssetHoldings(round, delta.DeletedAssetHoldings)
	if err != nil {
		return err
	}

	err = w.writeDeletedAppLocalStates(round, delta.DeletedAppLocalStates)
	if err != nil {
		return err
	}

	return nil
}

func (w *Writer) updateAccountSigType(payset []transactions.SignedTxnInBlock) error {
	for _, stxnib := range payset {
		var err error
		if stxnib.Txn.RekeyTo == (basics.Address{}) {
			_, err = w.updateAccountKeyTypeStmt.Exec(
				string(idb.SignatureType(stxnib.SignedTxn)), stxnib.Txn.Sender[:])
		} else {
			_, err = w.updateAccountKeyTypeStmt.Exec(nil, stxnib.Txn.Sender[:])
		}
		if err != nil {
			return fmt.Errorf("updateSigType() exec err: %w", err)
		}
	}

	return nil
}

func (w *Writer) AddBlock(block bookkeeping.Block, modifiedTxns []transactions.SignedTxnInBlock, delta ledgercore.StateDelta) error {
	err := w.addBlockHeader(block.BlockHeader)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	err = w.addTransactions(&block, modifiedTxns)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	err = w.addTransactionParticipation(&block)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	specialAddresses := transactions.SpecialAddresses{
		FeeSink: block.FeeSink,
		RewardsPool: block.RewardsPool,
	}
	err = w.writeStateDelta(block.Round(), delta, specialAddresses)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	err = w.updateAccountSigType(block.Payset)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	return nil
}
