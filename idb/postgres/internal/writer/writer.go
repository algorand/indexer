package writer

import (
	"database/sql"
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

const addBlockHeaderQuery = `INSERT INTO block_header
	(round, realtime, rewardslevel, header)
	VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`
const setSpecialAccountsQuery = `INSERT INTO metastate (k, v)
	VALUES ('accounts', $1) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`
const addTxnQuery = `INSERT INTO txn
	(round, intra, typeenum, asset, txid, txnbytes, txn, extra)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT DO NOTHING`
const addTxnParticipantQuery = `INSERT INTO txn_participation
	(addr, round, intra) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`
const upsertAssetQuery = `INSERT INTO asset
	(index, creator_addr, params, deleted, created_at)
	VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (index) DO UPDATE SET
	creator_addr = EXCLUDED.creator_addr, params = EXCLUDED.params, deleted = FALSE`
const upsertAccountAssetQuery = `INSERT INTO account_asset
	(addr, assetid, amount, frozen, deleted, created_at)
	VALUES($1, $2, $3, $4, FALSE, $5) ON CONFLICT (addr, assetid) DO UPDATE SET
	amount = EXCLUDED.amount, frozen = EXCLUDED.frozen, deleted = FALSE`
const upsertAppQuery = `INSERT INTO app
	(index, creator, params, deleted, created_at)
	VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (index) DO UPDATE SET
	creator = EXCLUDED.creator, params = EXCLUDED.params, deleted = FALSE`
const upsertAccountAppQuery = `INSERT INTO account_app
	(addr, app, localstate, deleted, created_at)
	VALUES($1, $2, $3, FALSE, $4) ON CONFLICT (addr, app) DO UPDATE SET
	localstate = EXCLUDED.localstate, deleted = FALSE`
const deleteAccountQuery = `INSERT INTO account
	(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, closed_at)
	VALUES($1, 0, 0, 0, TRUE, $2, $2) ON CONFLICT (addr) DO UPDATE SET
	microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase,
	rewards_total = EXCLUDED.rewards_total, deleted = TRUE,
	closed_at = EXCLUDED.closed_at, account_data = EXCLUDED.account_data`
const upsertAccountQuery = `INSERT INTO account
	(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, account_data)
	VALUES($1, $2, $3, $4, FALSE, $5, $6) ON CONFLICT (addr) DO UPDATE SET
	microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase,
	rewards_total = EXCLUDED.rewards_total, deleted = FALSE,
	account_data = EXCLUDED.account_data`
const deleteAssetQuery = `INSERT INTO asset
	(index, creator_addr, params, deleted, created_at, closed_at)
	VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (index) DO UPDATE SET
	creator_addr = EXCLUDED.creator_addr, params = EXCLUDED.params, deleted = TRUE,
	closed_at = EXCLUDED.closed_at`
const deleteAccountAssetQuery = `INSERT INTO account_asset
	(addr, assetid, amount, frozen, deleted, created_at, closed_at)
	VALUES($1, $2, 0, false, TRUE, $3, $3) ON CONFLICT (addr, assetid) DO UPDATE SET
	amount = EXCLUDED.amount, deleted = TRUE, closed_at = EXCLUDED.closed_at`
const deleteAppQuery = `INSERT INTO app
	(index, creator, params, deleted, created_at, closed_at)
	VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (index) DO UPDATE SET
	creator = EXCLUDED.creator, params = EXCLUDED.params, deleted = TRUE,
	closed_at = EXCLUDED.closed_at`
const deleteAccountAppQuery = `INSERT INTO account_app
	(addr, app, localstate, deleted, created_at, closed_at)
	VALUES($1, $2, 'null'::jsonb, TRUE, $3, $3) ON CONFLICT (addr, app) DO UPDATE SET
	localstate = EXCLUDED.localstate, deleted = TRUE, closed_at = EXCLUDED.closed_at`
const updateAccountKeyTypeQuery = `UPDATE account SET keytype = $1 WHERE addr = $2`

// Writer is responsible for writing blocks and accounting state deltas to the database.
type Writer struct {
	tx *sql.Tx

	addBlockHeaderStmt       *sql.Stmt
	setSpecialAccountsStmt   *sql.Stmt
	addTxnStmt               *sql.Stmt
	addTxnParticipantStmt    *sql.Stmt
	upsertAssetStmt          *sql.Stmt
	upsertAccountAssetStmt   *sql.Stmt
	upsertAppStmt            *sql.Stmt
	upsertAccountAppStmt     *sql.Stmt
	deleteAccountStmt        *sql.Stmt
	upsertAccountStmt        *sql.Stmt
	deleteAssetStmt          *sql.Stmt
	deleteAccountAssetStmt   *sql.Stmt
	deleteAppStmt            *sql.Stmt
	deleteAccountAppStmt     *sql.Stmt
	updateAccountKeyTypeStmt *sql.Stmt
}

// MakeWriter creates a Writer object.
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
	w.setSpecialAccountsStmt, err = tx.Prepare(setSpecialAccountsQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare set special accounts stmt err: %w", err)
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
	w.upsertAssetStmt, err = tx.Prepare(upsertAssetQuery)
	if err != nil {
		return Writer{}, fmt.Errorf("MakeWriter(): prepare update asset stmt err: %w", err)
	}
	w.upsertAccountAssetStmt, err = tx.Prepare(upsertAccountAssetQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare update account asset stmt err: %w", err)
	}
	w.upsertAppStmt, err = tx.Prepare(upsertAppQuery)
	if err != nil {
		return Writer{}, fmt.Errorf("MakeWriter(): prepare update app stmt err: %w", err)
	}
	w.upsertAccountAppStmt, err = tx.Prepare(upsertAccountAppQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare update account app stmt err: %w", err)
	}
	w.deleteAccountStmt, err = tx.Prepare(deleteAccountQuery)
	if err != nil {
		return Writer{},
			fmt.Errorf("MakeWriter(): prepare delete account stmt err: %w", err)
	}
	w.upsertAccountStmt, err = tx.Prepare(upsertAccountQuery)
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

// Close shuts down Writer.
func (w *Writer) Close() {
	w.addBlockHeaderStmt.Close()
	w.setSpecialAccountsStmt.Close()
	w.addTxnStmt.Close()
	w.addTxnParticipantStmt.Close()
	w.upsertAssetStmt.Close()
	w.upsertAccountAssetStmt.Close()
	w.upsertAppStmt.Close()
	w.upsertAccountAppStmt.Close()
	w.deleteAccountStmt.Close()
	w.upsertAccountStmt.Close()
	w.deleteAssetStmt.Close()
	w.deleteAccountAssetStmt.Close()
	w.deleteAppStmt.Close()
	w.deleteAccountAppStmt.Close()
	w.updateAccountKeyTypeStmt.Close()
}

func (w *Writer) addBlockHeader(blockHeader *bookkeeping.BlockHeader) error {
	_, err := w.addBlockHeaderStmt.Exec(
		uint64(blockHeader.Round), time.Unix(blockHeader.TimeStamp, 0).UTC(),
		blockHeader.RewardsLevel, encoding.EncodeBlockHeader(*blockHeader))
	if err != nil {
		return fmt.Errorf("addBlockHeader() err: %w", err)
	}
	return nil
}

func (w *Writer) setSpecialAccounts(addresses transactions.SpecialAddresses) error {
	j := encoding.EncodeSpecialAddresses(addresses)
	_, err := w.setSpecialAccountsStmt.Exec(j)
	if err != nil {
		return fmt.Errorf("setSpecialAccounts() err: %w", err)
	}
	return nil
}

// Get the ID of the creatable referenced in the given transaction
// (0 if not an asset or app transaction).
func transactionAssetID(block *bookkeeping.Block, intra uint64, typeenum idb.TxnTypeEnum) uint64 {
	assetid := uint64(0)
	txn := block.Payset[intra].Txn

	switch typeenum {
	case idb.TypeEnumAssetConfig:
		assetid = uint64(txn.ConfigAsset)
		if assetid == 0 {
			assetid = block.TxnCounter - uint64(len(block.Payset)) + intra + 1
		}
	case idb.TypeEnumAssetTransfer:
		assetid = uint64(txn.XferAsset)
	case idb.TypeEnumAssetFreeze:
		assetid = uint64(txn.FreezeAsset)
	case idb.TypeEnumApplication:
		assetid = uint64(txn.ApplicationID)
		if assetid == 0 {
			assetid = block.TxnCounter - uint64(len(block.Payset)) + intra + 1
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

		txn := &stxnad.Txn
		typeenum, ok := idb.GetTypeEnum(txn.Type)
		if !ok {
			return fmt.Errorf("addTransactions() get type enum")
		}
		assetid := transactionAssetID(block, uint64(i), typeenum)
		id := txn.ID().String()
		extra := idb.TxnExtra{
			AssetCloseAmount: modifiedTxns[i].ApplyData.AssetClosingAmount,
		}
		_, err = w.addTxnStmt.Exec(
			uint64(block.Round()), i, int(typeenum), assetid, id,
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

func (w *Writer) writeAccountData(round basics.Round, address basics.Address, accountData basics.AccountData) error {
	// Update `asset` table.
	for assetid, params := range accountData.AssetParams {
		_, err := w.upsertAssetStmt.Exec(
			uint64(assetid), address[:], encoding.EncodeAssetParams(params), uint64(round))
		if err != nil {
			return fmt.Errorf("writeAccountData() exec update asset err: %w", err)
		}
	}

	// Update `account_asset` table.
	for assetid, holding := range accountData.Assets {
		_, err := w.upsertAccountAssetStmt.Exec(
			address[:], uint64(assetid), strconv.FormatUint(holding.Amount, 10),
			holding.Frozen, uint64(round))
		if err != nil {
			return fmt.Errorf("writeAccountData() exec update account asset err: %w", err)
		}
	}

	// Update `app` table.
	for appid, params := range accountData.AppParams {
		_, err := w.upsertAppStmt.Exec(
			uint64(appid), address[:], encoding.EncodeAppParams(params), uint64(round))
		if err != nil {
			return fmt.Errorf("writeAccountData() exec update app err: %w", err)
		}
	}

	// Update `account_app` table.
	for appid, state := range accountData.AppLocalStates {
		_, err := w.upsertAccountAppStmt.Exec(
			address[:], uint64(appid), encoding.EncodeAppLocalState(state), uint64(round))
		if err != nil {
			return fmt.Errorf("writeAccountData() exec update account app err: %w", err)
		}
	}

	// Update `account` table.
	if accountData.IsZero() {
		// Delete account.
		_, err := w.deleteAccountStmt.Exec(address[:], uint64(round))
		if err != nil {
			return fmt.Errorf("writeAccountData() exec delete account err: %w", err)
		}
	} else {
		// Update account.
		accountDataJSON :=
			encoding.EncodeTrimmedAccountData(encoding.TrimAccountData(accountData))
		_, err := w.upsertAccountStmt.Exec(
			address[:], accountData.MicroAlgos.Raw, accountData.RewardsBase,
			accountData.RewardedMicroAlgos.Raw, uint64(round), accountDataJSON)
		if err != nil {
			return fmt.Errorf("writeAccountData() exec update account err: %w", err)
		}
	}

	return nil
}

func (w *Writer) writeAccountDeltas(round basics.Round, deltas ledgercore.AccountDeltas, specialAddresses transactions.SpecialAddresses) error {
	// Update `account` table.
	for i := 0; i < deltas.Len(); i++ {
		address, accountData := deltas.GetByIdx(i)

		// Indexer currently doesn't support special accounts.
		// TODO: remove this check.
		if (address != specialAddresses.FeeSink) &&
			(address != specialAddresses.RewardsPool) {
			err := w.writeAccountData(round, address, accountData)
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

func (w *Writer) writeDeletedAssetHoldings(round basics.Round, modifiedAssetHoldings map[ledgercore.AccountAsset]bool) error {
	for aa, created := range modifiedAssetHoldings {
		if !created {
			_, err := w.deleteAccountAssetStmt.Exec(
				aa.Address[:], uint64(aa.Asset), uint64(round))
			if err != nil {
				return fmt.Errorf(
					"writeDeletedAssetHoldings() exec delete account asset err: %w", err)
			}
		}
	}

	return nil
}

func (w *Writer) writeDeletedAppLocalStates(round basics.Round, modifiedAppLocalStates map[ledgercore.AccountApp]bool) error {
	for aa, created := range modifiedAppLocalStates {
		if !created {
			_, err := w.deleteAccountAppStmt.Exec(aa.Address[:], uint64(aa.App), uint64(round))
			if err != nil {
				return fmt.Errorf(
					"writeDeletedAppLocalStates() exec delete account app err: %w", err)
			}
		}
	}

	return nil
}

func (w *Writer) writeStateDelta(round basics.Round, delta ledgercore.StateDelta, specialAddresses transactions.SpecialAddresses) error {
	err := w.writeAccountDeltas(round, delta.Accts, specialAddresses)
	if err != nil {
		return err
	}

	err = w.writeDeletedCreatables(round, delta.Creatables)
	if err != nil {
		return err
	}

	err = w.writeDeletedAssetHoldings(round, delta.ModifiedAssetHoldings)
	if err != nil {
		return err
	}

	err = w.writeDeletedAppLocalStates(round, delta.ModifiedAppLocalStates)
	if err != nil {
		return err
	}

	return nil
}

func (w *Writer) updateAccountSigType(payset []transactions.SignedTxnInBlock) error {
	for _, stxnib := range payset {
		if stxnib.Txn.RekeyTo == (basics.Address{}) {
			sigtype, err := idb.SignatureType(&stxnib.SignedTxn)
			if err != nil {
				return fmt.Errorf("updateAccountSigType() err: %w", err)
			}
			_, err = w.updateAccountKeyTypeStmt.Exec(sigtype, stxnib.Txn.Sender[:])
			if err != nil {
				return fmt.Errorf("updateAccountSigType() set sigtype err: %w", err)
			}
		} else {
			_, err := w.updateAccountKeyTypeStmt.Exec(nil, stxnib.Txn.Sender[:])
			if err != nil {
				return fmt.Errorf("updateAccountSigType() reset sigtype err: %w", err)
			}
		}
	}

	return nil
}

// AddBlock writes the block and accounting state deltas to the database.
func (w *Writer) AddBlock(block *bookkeeping.Block, modifiedTxns []transactions.SignedTxnInBlock, delta ledgercore.StateDelta) error {
	specialAddresses := transactions.SpecialAddresses{
		FeeSink:     block.FeeSink,
		RewardsPool: block.RewardsPool,
	}

	err := w.addBlockHeader(&block.BlockHeader)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	err = w.setSpecialAccounts(specialAddresses)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	err = w.addTransactions(block, modifiedTxns)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	err = w.addTransactionParticipation(block)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
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
