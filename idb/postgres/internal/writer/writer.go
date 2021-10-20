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
	addBlockHeaderStmtName       = "add_block_header"
	setSpecialAccountsStmtName   = "set_special_accounts"
	upsertAssetStmtName          = "upsert_asset"
	upsertAccountAssetStmtName   = "upsert_account_asset"
	upsertAppStmtName            = "upsert_app"
	upsertAccountAppStmtName     = "upsert_account_app"
	deleteAccountStmtName        = "delete_account"
	upsertAccountStmtName        = "upsert_account"
	deleteAssetStmtName          = "delete_asset"
	deleteAccountAssetStmtName   = "delete_account_asset"
	deleteAppStmtName            = "delete_app"
	deleteAccountAppStmtName     = "delete_account_app"
	updateAccountKeyTypeStmtName = "update_account_key_type"
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
	upsertAccountStmtName: `INSERT INTO account
		(addr, microalgos, rewardsbase, rewards_total, deleted, created_at, account_data)
		VALUES($1, $2, $3, $4, FALSE, $5, $6) ON CONFLICT (addr) DO UPDATE SET
		microalgos = EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase,
		rewards_total = EXCLUDED.rewards_total, deleted = FALSE,
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
	updateAccountKeyTypeStmtName: `UPDATE account SET keytype = $1 WHERE addr = $2`,
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

// Get the ID of the creatable referenced in the given transaction
// (0 if not an asset or app transaction).
// Note: ConsensusParams.MaxInnerTransactions could be overridden to force
//       generating ApplyData.{ApplicationID/ConfigAsset}. This function does
//       other things too, so it is not clear we should use it. The only
//       real benefit is that it would slightly simplify this function by
//       allowing us to leave out the intra / block parameters.
func transactionAssetID(txn transactions.SignedTxnWithAD, intra uint64, block *bookkeeping.Block) uint64 {
	assetid := uint64(0)

	switch txn.Txn.Type {
	case protocol.ApplicationCallTx:
		assetid = uint64(txn.ApplicationID)
		if assetid == 0 {
			assetid = uint64(txn.ApplyData.ApplicationID)
		}
		if assetid == 0 {
			// pre v30 transactions do not have ApplyData.ConfigAsset or InnerTxns
			// so txn counter + payset pos calculation is OK
			assetid = block.TxnCounter - uint64(len(block.Payset)) + intra + 1
		}
	case protocol.AssetConfigTx:
		assetid = uint64(txn.ConfigAsset)
		if assetid == 0 {
			assetid = uint64(txn.ApplyData.ConfigAsset)
		}
		if assetid == 0 {
			// pre v30 transactions do not have ApplyData.ApplicationID or InnerTxns
			// so txn counter + payset pos calculation is OK
			assetid = block.TxnCounter - uint64(len(block.Payset)) + intra + 1
		}
	case protocol.AssetTransferTx:
		assetid = uint64(txn.Txn.XferAsset)
	case protocol.AssetFreezeTx:
		assetid = uint64(txn.Txn.FreezeAsset)
	}

	return assetid
}

// addInnerTransactions traverses the inner transaction tree and adds them to
// the transaction table. It performs a preorder traversal to correctly compute
// the intra round offset, the offset for the next transaction is returned.
func (w *Writer) addInnerTransactions(stxnad *transactions.SignedTxnWithAD, block *bookkeeping.Block, intra uint64, rootTxid string, rows [][]interface{}) (uint64, [][]interface{}, error) {
	next := intra
	var err error
	for _, itxn := range stxnad.ApplyData.EvalDelta.InnerTxns {
		txn := &itxn.Txn
		typeenum, ok := idb.GetTypeEnum(txn.Type)
		if !ok {
			return 0, nil, fmt.Errorf("addInnerTransactions() get type enum")
		}
		assetid := transactionAssetID(itxn, 0, nil)
		extra := idb.TxnExtra{
			AssetCloseAmount: itxn.ApplyData.AssetClosingAmount,
			RootTxid:         rootTxid,
		}

		// When encoding an inner transaction we remove any further nested inner transactions.
		// To reconstruct a full object the root transaction must be fetched.
		txnNoInner := *stxnad
		txnNoInner.EvalDelta.InnerTxns = nil
		rows = append(rows, []interface{}{
			uint64(block.Round()), intra, int(typeenum), assetid,
			nil, // inner transactions do not have a txid.
			nil, // txn bytes are only in the parent.
			encoding.EncodeSignedTxnWithAD(txnNoInner),
			encoding.EncodeTxnExtra(&extra)})

		// Recurse at end for preorder traversal
		next, rows, err = w.addInnerTransactions(&itxn, block, next+1, rootTxid, rows)
		if err != nil {
			return 0, nil, err
		}
	}

	return next, rows, nil
}

// Add transactions from `block` to the database. `modifiedTxns` contains enhanced
// apply data generated by evaluator.
func (w *Writer) addTransactions(block *bookkeeping.Block, modifiedTxns []transactions.SignedTxnInBlock) error {
	var rows [][]interface{}

	intra := uint64(0)
	for idx, stib := range block.Payset {
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
		assetid := transactionAssetID(stxnad, intra, block)
		id := txn.ID().String()

		extra := idb.TxnExtra{
			AssetCloseAmount: modifiedTxns[idx].ApplyData.AssetClosingAmount,
		}
		rows = append(rows, []interface{}{
			uint64(block.Round()), intra, int(typeenum), assetid, id,
			protocol.Encode(&stxnad),
			encoding.EncodeSignedTxnWithAD(stxnad),
			encoding.EncodeTxnExtra(&extra)})

		intra, rows, err = w.addInnerTransactions(&stib.SignedTxnWithAD, block, intra+1, id, rows)
		if err != nil {
			return fmt.Errorf("addTransactions() adding inner: %w", err)
		}
	}

	_, err := w.tx.CopyFrom(
		context.Background(),
		pgx.Identifier{"txn"},
		[]string{"round", "intra", "typeenum", "asset", "txid", "txnbytes", "txn", "extra"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("addTransactions() copy from err: %w", err)
	}

	return nil
}

func getTransactionParticipantsImpl(stxnad *transactions.SignedTxnWithAD, includeInner bool, add func(address basics.Address)) {
	txn := stxnad.Txn

	add(txn.Sender)
	add(txn.Receiver)
	add(txn.CloseRemainderTo)
	add(txn.AssetSender)
	add(txn.AssetReceiver)
	add(txn.AssetCloseTo)
	add(txn.FreezeAccount)

	if includeInner {
		for _, inner := range stxnad.ApplyData.EvalDelta.InnerTxns {
			getTransactionParticipantsImpl(&inner, includeInner, add)
		}
	}
}

// getTransactionParticipants returns referenced addresses from the txn and all inner txns
func getTransactionParticipants(stxnad *transactions.SignedTxnWithAD, includeInner bool) []basics.Address {
	const acctsPerTxn = 7

	if !includeInner || len(stxnad.ApplyData.EvalDelta.InnerTxns) == 0 {
		// if no inner transactions then adding into a slice with in-place de-duplication
		res := make([]basics.Address, 0, acctsPerTxn)
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

		getTransactionParticipantsImpl(stxnad, includeInner, add)
		return res
	}

	// inner transactions might have inner transactions might have inner...
	// so the resultant slice is created after collecting all the data from nested transactions.
	// this is probably a bit slower than the default case due to two mem allocs and additional iterations
	size := acctsPerTxn * (1 + len(stxnad.ApplyData.EvalDelta.InnerTxns)) // approx
	participants := make(map[basics.Address]struct{}, size)
	add := func(address basics.Address) {
		if address.IsZero() {
			return
		}
		participants[address] = struct{}{}
	}

	getTransactionParticipantsImpl(stxnad, includeInner, add)

	res := make([]basics.Address, 0, len(participants))
	for addr := range participants {
		res = append(res, addr)
	}

	return res
}

// addInnerTransactionParticipation traverses the inner transaction tree and
// adds txn participation records for each. It performs a preorder traversal
// to correctly compute the intra round offset, the offset for the next
// transaction is returned.
func addInnerTransactionParticipation(stxnad *transactions.SignedTxnWithAD, round, intra uint64, rows [][]interface{}) (uint64, [][]interface{}) {
	next := intra
	for _, itxn := range stxnad.ApplyData.EvalDelta.InnerTxns {
		// Only search inner transactions by direct participation.
		// TODO: Should inner app calls be surfaced by their participants?
		participants := getTransactionParticipants(&itxn, false)

		for j := range participants {
			rows = append(rows, []interface{}{participants[j][:], round, next})
		}

		next, rows = addInnerTransactionParticipation(&itxn, round, next+1, rows)
	}
	return next, rows

}

func (w *Writer) addTransactionParticipation(block *bookkeeping.Block) error {
	var rows [][]interface{}
	next := uint64(0)

	for _, stxnib := range block.Payset {
		participants := getTransactionParticipants(&stxnib.SignedTxnWithAD, true)

		for j := range participants {
			rows = append(rows, []interface{}{participants[j][:], uint64(block.Round()), next})
		}

		next, rows = addInnerTransactionParticipation(&stxnib.SignedTxnWithAD, uint64(block.Round()), next+1, rows)
	}

	_, err := w.tx.CopyFrom(
		context.Background(),
		pgx.Identifier{"txn_participation"},
		[]string{"addr", "round", "intra"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("addTransactionParticipation() copy from err: %w", err)
	}

	return nil
}

func writeAccountData(round basics.Round, address basics.Address, accountData basics.AccountData, batch *pgx.Batch) {
	// Update `asset` table.
	for assetid, params := range accountData.AssetParams {
		batch.Queue(
			upsertAssetStmtName,
			uint64(assetid), address[:], encoding.EncodeAssetParams(params), uint64(round))
	}

	// Update `account_asset` table.
	for assetid, holding := range accountData.Assets {
		batch.Queue(
			upsertAccountAssetStmtName,
			address[:], uint64(assetid), strconv.FormatUint(holding.Amount, 10),
			holding.Frozen, uint64(round))
	}

	// Update `app` table.
	for appid, params := range accountData.AppParams {
		batch.Queue(
			upsertAppStmtName,
			uint64(appid), address[:], encoding.EncodeAppParams(params), uint64(round))
	}

	// Update `account_app` table.
	for appid, state := range accountData.AppLocalStates {
		batch.Queue(
			upsertAccountAppStmtName,
			address[:], uint64(appid), encoding.EncodeAppLocalState(state), uint64(round))
	}

	// Update `account` table.
	if accountData.IsZero() {
		// Delete account.
		batch.Queue(deleteAccountStmtName, address[:], uint64(round))
	} else {
		// Update account.
		accountDataJSON :=
			encoding.EncodeTrimmedAccountData(encoding.TrimAccountData(accountData))
		batch.Queue(
			upsertAccountStmtName,
			address[:], accountData.MicroAlgos.Raw, accountData.RewardsBase,
			accountData.RewardedMicroAlgos.Raw, uint64(round), accountDataJSON)
	}
}

func writeAccountDeltas(round basics.Round, deltas ledgercore.AccountDeltas, specialAddresses transactions.SpecialAddresses, batch *pgx.Batch) {
	// Update `account` table.
	for i := 0; i < deltas.Len(); i++ {
		address, accountData := deltas.GetByIdx(i)

		// Indexer currently doesn't support special accounts.
		// TODO: remove this check.
		if (address != specialAddresses.FeeSink) &&
			(address != specialAddresses.RewardsPool) {
			writeAccountData(round, address, accountData, batch)
		}
	}
}

func writeDeletedCreatables(round basics.Round, creatables map[basics.CreatableIndex]ledgercore.ModifiedCreatable, batch *pgx.Batch) {
	for index, creatable := range creatables {
		// If deleted.
		if !creatable.Created {
			creator := new(basics.Address)
			*creator = creatable.Creator

			if creatable.Ctype == basics.AssetCreatable {
				batch.Queue(deleteAssetStmtName, uint64(index), creator[:], uint64(round))
			} else {
				batch.Queue(deleteAppStmtName, uint64(index), creator[:], uint64(round))
			}
		}
	}
}

func writeDeletedAssetHoldings(round basics.Round, modifiedAssetHoldings map[ledgercore.AccountAsset]bool, batch *pgx.Batch) {
	for aa, created := range modifiedAssetHoldings {
		if !created {
			address := new(basics.Address)
			*address = aa.Address

			batch.Queue(
				deleteAccountAssetStmtName, address[:], uint64(aa.Asset), uint64(round))
		}
	}
}

func writeDeletedAppLocalStates(round basics.Round, modifiedAppLocalStates map[ledgercore.AccountApp]bool, batch *pgx.Batch) {
	for aa, created := range modifiedAppLocalStates {
		if !created {
			address := new(basics.Address)
			*address = aa.Address

			batch.Queue(deleteAccountAppStmtName, address[:], uint64(aa.App), uint64(round))
		}
	}
}

func writeStateDelta(round basics.Round, delta ledgercore.StateDelta, specialAddresses transactions.SpecialAddresses, batch *pgx.Batch) {
	writeAccountDeltas(round, delta.Accts, specialAddresses, batch)
	writeDeletedCreatables(round, delta.Creatables, batch)
	writeDeletedAssetHoldings(round, delta.ModifiedAssetHoldings, batch)
	writeDeletedAppLocalStates(round, delta.ModifiedAppLocalStates, batch)
}

func updateAccountSigType(payset []transactions.SignedTxnInBlock, batch *pgx.Batch) error {
	for i := range payset {
		if payset[i].Txn.RekeyTo == (basics.Address{}) {
			sigtype, err := idb.SignatureType(&payset[i].SignedTxn)
			if err != nil {
				return fmt.Errorf("updateAccountSigType() err: %w", err)
			}
			batch.Queue(updateAccountKeyTypeStmtName, sigtype, payset[i].Txn.Sender[:])
		} else {
			batch.Queue(updateAccountKeyTypeStmtName, nil, payset[i].Txn.Sender[:])
		}
	}

	return nil
}

// AddBlock writes the block and accounting state deltas to the database.
func (w *Writer) AddBlock(block *bookkeeping.Block, modifiedTxns []transactions.SignedTxnInBlock, delta ledgercore.StateDelta) error {
	err := w.addTransactions(block, modifiedTxns)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}
	err = w.addTransactionParticipation(block)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
	}

	var batch pgx.Batch

	specialAddresses := transactions.SpecialAddresses{
		FeeSink:     block.FeeSink,
		RewardsPool: block.RewardsPool,
	}

	addBlockHeader(&block.BlockHeader, &batch)
	setSpecialAccounts(specialAddresses, &batch)
	writeStateDelta(block.Round(), delta, specialAddresses, &batch)
	err = updateAccountSigType(block.Payset, &batch)
	if err != nil {
		return fmt.Errorf("AddBlock() err: %w", err)
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
	err = results.Close()
	if err != nil {
		return fmt.Errorf("AddBlock() close results err: %w", err)
	}

	return nil
}
