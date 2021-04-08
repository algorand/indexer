// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/migration"
	"github.com/algorand/indexer/types"
)

// rewardsMigrationIndex is the index of m7RewardsAndDatesPart2.
const rewardsMigrationIndex = 7

func init() {
	migrations = []migrationStruct{
		// function, blocking, description
		{m0fixupTxid, false, "Recompute the txid with corrected algorithm."},
		{m1fixupBlockTime, true, "Adjust block time to UTC timezone."},
		{m2apps, true, "Update DB Schema for Algorand application support."},
		{m3acfgFix, false, "Recompute asset configurations with corrected merge function."},

		// 2.2.2 hotfix
		{m4accountIndices, true, "Add indices to make sure account lookups remain fast when there are a lot of apps or assets."},

		// Migrations for 2.3.1 release
		{m5MarkTxnJSONSplit, true, "record round at which txn json recording changes, for future migration to fixup prior records"},
		{m6RewardsAndDatesPart1, true, "Update DB Schema for cumulative account reward support and creation dates."},
		{m7RewardsAndDatesPart2, false, "Compute cumulative account rewards for all accounts."},

		// Migrations for 2.3.2 release
		{m8StaleClosedAccounts, false, "clear some stale data from closed accounts"},
		{m9TxnJSONEncoding, false, "some txn JSON encodings need app keys base64 encoded"},
		{m10SpecialAccountCleanup, false, "The initial m7 implementation would miss special accounts."},
		{m11AssetHoldingFrozen, false, "Fix asset holding freeze states."},

		// Migrations for a next release
		{FixFreezeLookupMigration, false, "Fix search by asset freeze address."},
		{ClearAccountDataMigration, false, "clear account data for accounts that have been closed"},
	}

	// Verify ensure the constant is pointing to the right index
	var m7Ptr postgresMigrationFunc = m7RewardsAndDatesPart2
	a2 := fmt.Sprintf("%v", migrations[rewardsMigrationIndex].migrate)
	a1 := fmt.Sprintf("%v", m7Ptr)
	if a1 != a2 {
		fmt.Println("Bad constant in postgres_migrations.go")
		os.Exit(1)
	}
}

// MigrationState is metadata used by the postgres migrations.
type MigrationState struct {
	NextMigration int `json:"next"`

	// NextRound used for m0,m9 to checkpoint progress.
	NextRound int64 `json:"round,omitempty"`

	// NextAssetID used for m3 to checkpoint progress.
	NextAssetID int64 `json:"assetid,omitempty"`

	// The following two are used for m7 to save progress.
	PointerRound *int64 `json:"pointerRound,omitempty"`
	PointerIntra *int64 `json:"pointerIntra,omitempty"`

	// Note: a generic "data" field here could be a good way to deal with this growing over time.
	//       It would require a mechanism to clear the data field between migrations to avoid using migration data
	//       from the previous migration.
}

// A migration function should take care of writing back to metastate migration row
type postgresMigrationFunc func(*IndexerDb, *MigrationState) error

type migrationStruct struct {
	migrate postgresMigrationFunc

	blocking bool

	// Description of the migration
	description string
}

var migrations []migrationStruct

func wrapPostgresHandler(handler postgresMigrationFunc, db *IndexerDb, state *MigrationState) migration.Handler {
	return func() error {
		return handler(db, state)
	}
}

// migrationStateBlocked returns true if a migration is required for running in read only mode.
func migrationStateBlocked(state MigrationState) bool {
	return state.NextMigration < rewardsMigrationIndex
}

// needsMigration returns true if there is an incomplete migration.
func needsMigration(state MigrationState) bool {
	return state.NextMigration < len(migrations)
}

// upsertMigrationStateTx updates the migration state, and optionally increments the next counter with an existing
// transaction.
func upsertMigrationStateTx(tx *sql.Tx, state *MigrationState, incrementNextMigration bool) (err error) {
	if incrementNextMigration {
		state.NextMigration++
	}
	migrationStateJSON := idb.JSONOneLine(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)

	return err
}

// upsertMigrationState updates the migration state, and optionally increments the next counter.
func upsertMigrationState(db *IndexerDb, state *MigrationState, incrementNextMigration bool) (err error) {
	if incrementNextMigration {
		state.NextMigration++
	}
	migrationStateJSON := idb.JSONOneLine(state)
	_, err = db.db.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)

	return err
}

func (db *IndexerDb) runAvailableMigrations(migrationStateJSON string) (err error) {
	var state MigrationState
	if len(migrationStateJSON) > 0 {
		err = json.Decode([]byte(migrationStateJSON), &state)
		if err != nil {
			return fmt.Errorf("(%s) bad metastate migration json, %v", migrationStateJSON, err)
		}
	}

	// Make migration tasks
	nextMigration := state.NextMigration
	tasks := make([]migration.Task, 0)
	for nextMigration < len(migrations) {
		tasks = append(tasks, migration.Task{
			Handler:       wrapPostgresHandler(migrations[nextMigration].migrate, db, &state),
			MigrationID:   nextMigration,
			Description:   migrations[nextMigration].description,
			DBUnavailable: migrations[nextMigration].blocking,
		})
		nextMigration++
	}

	if len(tasks) > 0 {
		// Add a task to mark migrations as done instead of using a channel.
		tasks = append(tasks, migration.Task{
			MigrationID: 9999999,
			Handler: func() error {
				return db.markMigrationsAsDone()
			},
			Description: "Mark migrations done",
		})
	}

	db.migration, err = migration.MakeMigration(tasks, db.log)
	if err != nil {
		return err
	}

	go db.migration.RunMigrations()

	return nil
}

// after setting up a new database, mark state as if all migrations had been done
func (db *IndexerDb) markMigrationsAsDone() (err error) {
	state := MigrationState{
		NextMigration: len(migrations),
	}
	migrationStateJSON := idb.JSONOneLine(state)
	return db.setMetastate(migrationMetastateKey, migrationStateJSON)
}

func (db *IndexerDb) getMigrationState() (*MigrationState, error) {
	migrationStateJSON, err := db.getMetastate(migrationMetastateKey)
	if err == sql.ErrNoRows {
		// no previous state, ok
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("%s, get m state err", txidMigrationErrMsg)
	}
	var txstate MigrationState
	err = json.Decode([]byte(migrationStateJSON), &txstate)
	if err != nil {
		return nil, fmt.Errorf("%s, migration json err", txidMigrationErrMsg)
	}
	return &txstate, nil
}

// Cached values for processAccount
var hasRewardsSupport = false
var lastCheckTs time.Time

// hasTotalRewardsSupport helps check the migration state for whether or not rewards are supported.
func (db *IndexerDb) hasTotalRewardsSupport() bool {
	// It will never revert back to false, so return it if cached true.
	if hasRewardsSupport {
		return hasRewardsSupport
	}

	// If this is the read/write instance, check the migration status directly
	if s := db.migration.GetStatus(); !s.IsZero() {
		hasRewardsSupport = s.TaskID > rewardsMigrationIndex || s.TaskID == -1
		return hasRewardsSupport
	}

	// If this is a read-only instance, lookup the migration metstate from the DB once a minute.
	if time.Since(lastCheckTs) > time.Minute {
		// Set this unconditionally, if there's a failure lets not spam the DB.
		lastCheckTs = time.Now()

		state, err := db.getMigrationState()
		if err != nil || state == nil {
			hasRewardsSupport = false
			return hasRewardsSupport
		}

		// Check that we're beyond the rewards migration task
		hasRewardsSupport = state.NextMigration > rewardsMigrationIndex
		return hasRewardsSupport
	}

	return hasRewardsSupport
}

// processAccount is a helper to modify accounts based on migration state.
func (db *IndexerDb) processAccount(account *generated.Account) {
	if !db.hasTotalRewardsSupport() {
		account.Rewards = 0
	}
}

func m0fixupTxid(db *IndexerDb, state *MigrationState) error {
	mtxid := &txidFiuxpMigrationContext{db: db, state: state}
	return mtxid.asyncTxidFixup()
}

func m1fixupBlockTime(db *IndexerDb, state *MigrationState) error {
	sqlLines := []string{
		`UPDATE block_header SET realtime = to_timestamp(coalesce(header ->> 'ts', '0')::bigint) AT TIME ZONE 'UTC'`,
	}
	return sqlMigration(db, state, sqlLines)
}

func m2apps(db *IndexerDb, state *MigrationState) error {
	sqlLines := []string{
		`CREATE TABLE IF NOT EXISTS app (
  index bigint PRIMARY KEY,
  creator bytea, -- account address
  params jsonb
);`,
		`CREATE TABLE IF NOT EXISTS account_app (
  addr bytea,
  app bigint,
  localstate jsonb,
  PRIMARY KEY (addr, app)
);`,
	}
	return sqlMigration(db, state, sqlLines)
}

func m3acfgFix(db *IndexerDb, state *MigrationState) (err error) {
	db.log.Printf("asset config fix migration starting")
	rows, err := db.db.Query(`SELECT index FROM asset WHERE index >= $1 ORDER BY 1`, state.NextAssetID)
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix err getting assetids")
		return err
	}
	assetIds := make([]int64, 0, 1000)
	for rows.Next() {
		var aid int64
		err = rows.Scan(&aid)
		if err != nil {
			db.log.WithError(err).Errorf("acfg fix err getting assetid row")
			rows.Close()
			return
		}
		assetIds = append(assetIds, aid)
	}
	rows.Close()
	for {
		nexti, err := m3acfgFixAsyncInner(db, state, assetIds)
		if err != nil {
			db.log.WithError(err).Errorf("acfg fix chunk")
			return err
		}
		if nexti < 0 {
			break
		}
		assetIds = assetIds[nexti:]
	}
	db.log.Printf("acfg fix migration finished")
	return nil
}

// do a transactional batch of asset fixes
// updates asset rows and metastate
func m3acfgFixAsyncInner(db *IndexerDb, state *MigrationState, assetIds []int64) (next int, err error) {
	lastlog := time.Now()
	tx, err := db.db.BeginTx(context.Background(), &serializable)
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix tx begin")
		return -1, err
	}
	defer tx.Rollback() // ignored if .Commit() first
	setacfg, err := tx.Prepare(`INSERT INTO asset (index, creator_addr, params) VALUES ($1, $2, $3) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params`)
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix prepare set asset")
		return
	}
	defer setacfg.Close()
	for i, aid := range assetIds {
		now := time.Now()
		if now.Sub(lastlog) > (5 * time.Second) {
			db.log.Printf("acfg fix next=%d", aid)
			state.NextAssetID = aid
			migrationStateJSON := idb.JSONOneLine(state)
			_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
			if err != nil {
				db.log.WithError(err).Errorf("acfg fix migration %d meta err", state.NextMigration)
				return -1, err
			}
			err = tx.Commit()
			if err != nil {
				db.log.WithError(err).Errorf("acfg fix migration %d commit err", state.NextMigration)
				return -1, err
			}
			return i, nil
		}
		txrows := db.txTransactions(tx, idb.TransactionFilter{TypeEnum: 3, AssetID: uint64(aid)})
		prevRound := uint64(0)
		first := true
		var params types.AssetParams
		var creator types.Address
		for txrow := range txrows {
			if txrow.Round < prevRound {
				db.log.Printf("acfg rows out of order %d < %d", txrow.Round, prevRound)
				return
			}
			var stxn types.SignedTxnInBlock
			err = msgpack.Decode(txrow.TxnBytes, &stxn)
			if err != nil {
				db.log.WithError(err).Errorf("acfg fix bad txn bytes %d:%d", txrow.Round, txrow.Intra)
				return
			}
			if first {
				params = stxn.Txn.AssetParams
				creator = stxn.Txn.Sender
				first = false
			} else if stxn.Txn.AssetParams == (types.AssetParams{}) {
				// delete asset
				params = stxn.Txn.AssetParams
			} else {
				params = types.MergeAssetConfig(params, stxn.Txn.AssetParams)
			}
		}
		outparams := idb.JSONOneLine(params)
		_, err = setacfg.Exec(aid, creator[:], outparams)
		if err != nil {
			db.log.WithError(err).Errorf("acfg fix asset update")
			return -1, err
		}
	}
	state.NextAssetID = 0
	state.NextMigration++
	migrationStateJSON := idb.JSONOneLine(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix migration %d meta err", state.NextMigration)
		return -1, err
	}
	err = tx.Commit()
	if err != nil {
		db.log.WithError(err).Errorf("acfg fix migration %d commit err", state.NextMigration)
		return -1, err
	}
	return -1, nil
}

// m6RewardsAndDatesPart1 adds the new rewards_total column to the account table.
func m6RewardsAndDatesPart1(db *IndexerDb, state *MigrationState) error {
	// Cache the round in the migration metastate
	round, err := db.GetMaxRoundAccounted()
	if err == idb.ErrorNotInitialized {
		// Shouldn't end up in the migration if this were the case.
		round = 0
	} else if err != nil {
		db.log.WithError(err).Errorf("m6: problem caching max round: %v", err)
		return err
	}

	// state is updated in the DB when calling 'sqlMigration'
	state.NextRound = int64(round)

	// update metastate
	sqlLines := []string{
		// rewards
		`ALTER TABLE account ADD COLUMN rewards_total bigint NOT NULL DEFAULT 0`,

		// created/closed round
		`ALTER TABLE account ADD COLUMN deleted boolean DEFAULT NULL`,
		`ALTER TABLE account ADD COLUMN created_at bigint DEFAULT NULL`,
		`ALTER TABLE account ADD COLUMN closed_at bigint DEFAULT NULL`,
		`ALTER TABLE app ADD COLUMN deleted boolean DEFAULT NULL`,
		`ALTER TABLE app ADD COLUMN created_at bigint DEFAULT NULL`,
		`ALTER TABLE app ADD COLUMN closed_at bigint DEFAULT NULL`,
		`ALTER TABLE account_app ADD COLUMN deleted boolean DEFAULT NULL`,
		`ALTER TABLE account_app ADD COLUMN created_at bigint DEFAULT NULL`,
		`ALTER TABLE account_app ADD COLUMN closed_at bigint DEFAULT NULL`,
		`ALTER TABLE account_asset ADD COLUMN deleted boolean DEFAULT NULL`,
		`ALTER TABLE account_asset ADD COLUMN created_at bigint DEFAULT NULL`,
		`ALTER TABLE account_asset ADD COLUMN closed_at bigint DEFAULT NULL`,
		`ALTER TABLE asset ADD COLUMN deleted boolean DEFAULT NULL`,
		`ALTER TABLE asset ADD COLUMN created_at bigint DEFAULT NULL`,
		`ALTER TABLE asset ADD COLUMN closed_at bigint DEFAULT NULL`,
	}
	return sqlMigration(db, state, sqlLines)
}

type addressAccountData struct {
	address     sdk_types.Address
	accountData *m7AccountData
}

func getParticipants(stxn types.SignedTxnWithAD) []sdk_types.Address {
	res := make([]sdk_types.Address, 0, 6)

	add := func(address types.Address) {
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

	add(stxn.Txn.Sender)
	add(stxn.Txn.Receiver)
	add(stxn.Txn.CloseRemainderTo)
	add(stxn.Txn.AssetSender)
	add(stxn.Txn.AssetReceiver)
	add(stxn.Txn.AssetCloseTo)

	return res
}

type txnID struct {
	round uint32
	intra uint32
}

func txnIDLess(l txnID, r txnID) bool {
	if l.round < r.round {
		return true
	}
	if l.round > r.round {
		return false
	}
	return l.intra < r.intra
}

type createClose struct {
	created      uint32
	closed       uint32
	deleted      bool
	createdValid bool
	closedValid  bool
	deletedValid bool
}

// updateClose will only allow the value to be set once.
func updateClose(value uint32, cc createClose) createClose {
	res := cc

	if !res.closedValid {
		res.closedValid = true
		res.closed = uint32(value)
	}

	if !res.deletedValid {
		res.deletedValid = true
		res.deleted = true
	}

	return res
}

// updateCreate will update the created round.
func updateCreate(value uint32, cc createClose) createClose {
	res := cc

	res.createdValid = true
	res.created = uint32(value)

	if !res.deletedValid {
		res.deletedValid = true
		res.deleted = false
	}

	return res
}

func executeCreatableCC(stmt *sql.Stmt, address sdk_types.Address, index uint32, cc createClose) error {
	deleted := sql.NullBool{
		Bool:  cc.deleted,
		Valid: cc.deletedValid,
	}
	created := sql.NullInt64{
		Int64: int64(cc.created),
		Valid: cc.createdValid,
	}
	closed := sql.NullInt64{
		Int64: int64(cc.closed),
		Valid: cc.closedValid,
	}
	_, err := stmt.Exec(address[:], index, created, closed, deleted)
	return err
}

func executeForEachCreatable(stmt *sql.Stmt, address sdk_types.Address, m map[uint32]createClose) error {
	for index, cc := range m {
		err := executeCreatableCC(stmt, address, index, cc)
		if err != nil {
			return err
		}
	}
	return nil
}

type m7AdditionalAccountData struct {
	// Asset dates are stored separately.
	asset    map[uint32]struct{}
	app      map[uint32]createClose
	appLocal map[uint32]createClose
}

type m7AccountData struct {
	cumulativeRewards types.MicroAlgos
	account           createClose
	assetHolding      map[uint32]createClose
	// Store other maps separately to save space, since most accounts do not use them.
	additional *m7AdditionalAccountData
}

func initM7AdditionalData() *m7AdditionalAccountData {
	return &m7AdditionalAccountData{
		asset:    make(map[uint32]struct{}),
		app:      make(map[uint32]createClose),
		appLocal: make(map[uint32]createClose),
	}
}

func initM7AccountData() *m7AccountData {
	return &m7AccountData{
		cumulativeRewards: 0,
		account:           createClose{},
		assetHolding:      make(map[uint32]createClose),
	}
}

func maybeInitializeAdditionalAccountData(accountData *m7AccountData) {
	if accountData.additional == nil {
		accountData.additional = initM7AdditionalData()
	}
}

// updateAccountData contains all the accounting logic to recompute total rewards and create/close
// rounds. It modifies `accountData` and `assetDataMap`, and need to be called with every
// transaction from most recent to oldest.
func updateAccountData(address types.Address, round uint32, assetID uint32, stxn types.SignedTxnWithAD, accountData *m7AccountData, assetDataMap map[uint32]createClose) {
	// Transactions are ordered most recent to oldest, so this makes sure created is set to the
	// oldest transaction.
	accountData.account.createdValid = true
	accountData.account.created = uint32(round)

	// When the account is closed rewards reset to zero.
	// Because transactions are newest to oldest, stop accumulating once we see a close.
	if !accountData.account.closedValid {
		if accounting.AccountCloseTxn(address, stxn) {
			accountData.account.closedValid = true
			accountData.account.closed = uint32(round)

			if !accountData.account.deletedValid {
				accountData.account.deletedValid = true
				accountData.account.deleted = true
			}
		} else {
			if !accountData.account.deletedValid {
				accountData.account.deletedValid = true
				accountData.account.deleted = false
			}

			if stxn.Txn.Sender == address {
				accountData.cumulativeRewards += stxn.ApplyData.SenderRewards
			}

			if stxn.Txn.Receiver == address {
				accountData.cumulativeRewards += stxn.ApplyData.ReceiverRewards
			}

			if stxn.Txn.CloseRemainderTo == address {
				accountData.cumulativeRewards += stxn.ApplyData.CloseRewards
			}
		}
	}

	if accounting.AssetCreateTxn(stxn) {
		maybeInitializeAdditionalAccountData(accountData)
		cc := updateCreate(round, assetDataMap[assetID])
		assetDataMap[assetID] = cc
		accountData.additional.asset[assetID] = struct{}{}
		// Special handling of asset holding since creating and deleting an asset also creates and
		// deletes an asset holding for the creator, but a different manager address can delete an
		// asset.
		accountData.assetHolding[assetID] = cc
	}

	if accounting.AssetDestroyTxn(stxn) {
		assetDataMap[assetID] = updateClose(round, assetDataMap[assetID])
	}

	if accounting.AssetOptInTxn(stxn) {
		accountData.assetHolding[assetID] = updateCreate(round, accountData.assetHolding[assetID])
	}

	if accounting.AssetOptOutTxn(stxn) && (stxn.Txn.Sender == address) {
		accountData.assetHolding[assetID] = updateClose(round, accountData.assetHolding[assetID])
	}

	if accounting.AppCreateTxn(stxn) {
		maybeInitializeAdditionalAccountData(accountData)
		accountData.additional.app[assetID] = updateCreate(round, accountData.additional.app[assetID])
	}

	if accounting.AppDestroyTxn(stxn) {
		maybeInitializeAdditionalAccountData(accountData)
		accountData.additional.app[assetID] = updateClose(round, accountData.additional.app[assetID])
	}

	if accounting.AppOptInTxn(stxn) {
		maybeInitializeAdditionalAccountData(accountData)
		accountData.additional.appLocal[assetID] =
			updateCreate(round, accountData.additional.appLocal[assetID])
	}

	if accounting.AppOptOutTxn(stxn) {
		maybeInitializeAdditionalAccountData(accountData)
		accountData.additional.appLocal[assetID] =
			updateClose(round, accountData.additional.appLocal[assetID])
	}
}

// m7RewardsAndDatesPart2UpdateAccounts loops through the provided accounts and generates a bunch of updates in a
// single transactional commit. These queries are written so that they can run in the background.
//
// For each account we run several queries:
// 1. updateTotalRewards         - conditionally update the total rewards if the account wasn't closed during iteration.
// 2. setCreateCloseAccount      - set the accounts create/close rounds.
// 3. setCreateCloseAsset        - set the accounts created assets create/close rounds.
// 4. setCreateCloseAssetHolding - (upsert) set the accounts asset holding create/close rounds.
// 5. setCreateCloseApp          - set the accounts created apps create/close rounds.
// 6. setCreateCloseAppLocal     - (upsert) set the accounts local apps create/close rounds.
//
// Note: These queries only work if closed_at was reset before the migration is started. That is true
//       for the initial migration, but if we need to reuse it in the future we'll need to fix the queries
//       or redo the query.
//
// This function also deletes unnecessary elements from `assetDataMap`. `txnId` is the new
// committed "pointer" in the transactions sequence.
func m7RewardsAndDatesPart2UpdateAccounts(db *IndexerDb, accountData []addressAccountData, assetDataMap map[uint32]createClose, txnID txnID, state *MigrationState) error {
	// Make sure round accounting doesn't interfere with updating these accounts.
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	// Open a postgres transaction and submit results for each account.
	tx, err := db.db.BeginTx(context.Background(), &serializable)
	if err != nil {
		return fmt.Errorf("m7: tx begin: %v", err)
	}
	defer tx.Rollback() // ignored if .Commit() first

	// 1. updateTotalRewards            - conditionally update the total rewards if the account wasn't closed during iteration.
	// $3 is the round after which new blocks will have the closed_at field set.
	// We only set rewards_total when closed_at was set before that round.
	updateTotalRewards, err := tx.Prepare(`UPDATE account SET rewards_total = coalesce(rewards_total, 0) + $2 WHERE addr = $1 AND coalesce(closed_at, 0) < $3`)
	if err != nil {
		return fmt.Errorf("m7: set rewards prepare: %v", err)
	}
	defer updateTotalRewards.Close()

	// 2. setCreateCloseAccount      - set the accounts create/close rounds.
	// We always set the created_at field because it will never change.
	// closed_at may already be set by the time the migration runs, or it might need to be cleared out.
	setCreateCloseAccount, err := tx.Prepare(`UPDATE account SET created_at = $2, closed_at = coalesce(closed_at, $3), deleted = coalesce(deleted, $4) WHERE addr = $1`)
	if err != nil {
		return fmt.Errorf("m7: set create close prepare: %v", err)
	}
	defer setCreateCloseAccount.Close()

	// 3. setCreateCloseAsset        - set the accounts created assets create/close rounds.
	setCreateCloseAsset, err := tx.Prepare(`UPDATE asset SET created_at = $3, closed_at = coalesce(closed_at, $4), deleted = coalesce(deleted, $5) WHERE creator_addr = $1 AND index=$2`)
	if err != nil {
		return fmt.Errorf("m7: set create close asset prepare: %v", err)
	}
	defer setCreateCloseAsset.Close()

	// 4. setCreateCloseAssetHolding - (upsert) set the accounts asset holding create/close rounds.
	setCreateCloseAssetHolding, err := tx.Prepare(`INSERT INTO account_asset(addr, assetid, amount, frozen, created_at, closed_at, deleted) VALUES ($1, $2, 0, false, $3, $4, $5) ON CONFLICT (addr, assetid) DO UPDATE SET created_at = EXCLUDED.created_at, closed_at = coalesce(account_asset.closed_at, EXCLUDED.closed_at), deleted = coalesce(account_asset.deleted, EXCLUDED.deleted)`)
	if err != nil {
		return fmt.Errorf("m7: set create close asset holding prepare: %v", err)
	}
	defer setCreateCloseAssetHolding.Close()

	// 5. setCreateCloseApp          - set the accounts created apps create/close rounds.
	setCreateCloseApp, err := tx.Prepare(`UPDATE app SET created_at = $3, closed_at = coalesce(closed_at, $4), deleted = coalesce(deleted, $5) WHERE creator = $1 AND index=$2`)
	if err != nil {
		return fmt.Errorf("m7: set create close app prepare: %v", err)
	}
	defer setCreateCloseApp.Close()

	// 6. setCreateCloseAppLocal     - (upsert) set the accounts local apps create/close rounds.
	setCreateCloseAppLocal, err := tx.Prepare(`INSERT INTO account_app (addr, app, created_at, closed_at, deleted) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (addr, app) DO UPDATE SET created_at = EXCLUDED.created_at, closed_at = coalesce(account_app.closed_at, EXCLUDED.closed_at), deleted = coalesce(account_app.deleted, EXCLUDED.deleted)`)
	if err != nil {
		return fmt.Errorf("m7: set create close app local prepare: %v", err)
	}
	defer setCreateCloseAppLocal.Close()

	// loop through all of the accounts.
	for _, ad := range accountData {
		addressStr := ad.address.String()

		// 1. updateTotalRewards            - conditionally update the total rewards if the account wasn't closed during iteration.
		_, err = updateTotalRewards.Exec(
			ad.address[:], ad.accountData.cumulativeRewards, state.NextRound)
		if err != nil {
			return fmt.Errorf("m7: failed to update %s with rewards %d: %v",
				addressStr, ad.accountData.cumulativeRewards, err)
		}

		// 2. setCreateCloseAccount      - set the accounts create/close rounds.
		{
			deleted := sql.NullBool{
				Bool:  ad.accountData.account.deleted,
				Valid: ad.accountData.account.deletedValid,
			}
			created := sql.NullInt64{
				Int64: int64(ad.accountData.account.created),
				Valid: ad.accountData.account.createdValid,
			}
			closed := sql.NullInt64{
				Int64: int64(ad.accountData.account.closed),
				Valid: ad.accountData.account.closedValid,
			}
			_, err = setCreateCloseAccount.Exec(ad.address[:], created, closed, deleted)
			if err != nil {
				return fmt.Errorf("m7: failed to update %s with create/close: %v", addressStr, err)
			}
		}

		// 4. setCreateCloseAssetHolding - (upsert) set the accounts asset holding create/close rounds.
		err = executeForEachCreatable(setCreateCloseAssetHolding, ad.address,
			ad.accountData.assetHolding)
		if err != nil {
			return fmt.Errorf("m7: failed to update %s with asset holding create/close: %v",
				addressStr, err)
		}

		if ad.accountData.additional != nil {
			// 3. setCreateCloseAsset        - set the accounts created assets create/close rounds.
			for index := range ad.accountData.additional.asset {
				cc, ok := assetDataMap[index]
				if !ok {
					return fmt.Errorf("m7: asset index %d created by %s is not in assetDataMap",
						index, addressStr)
				}
				err := executeCreatableCC(setCreateCloseAsset, ad.address, index, cc)
				if err != nil {
					return fmt.Errorf("m7: failed to update %s with asset index %d create/close: %v",
						addressStr, index, err)
				}
				delete(assetDataMap, index)
			}

			// 5. setCreateCloseApp          - set the accounts created apps create/close rounds.
			err = executeForEachCreatable(setCreateCloseApp, ad.address, ad.accountData.additional.app)
			if err != nil {
				return fmt.Errorf("m7: failed to update %s with app create/close: %v", addressStr, err)
			}

			// 6. setCreateCloseAppLocal     - (upsert) set the accounts local apps create/close rounds.
			err = executeForEachCreatable(setCreateCloseAppLocal, ad.address,
				ad.accountData.additional.appLocal)
			if err != nil {
				return fmt.Errorf("m7: failed to update %s with app local create/close: %v",
					addressStr, err)
			}
		}
	}

	{
		round := int64(txnID.round)
		intra := int64(txnID.intra)
		state.PointerRound = &round
		state.PointerIntra = &intra
	}
	migrationStateJSON := idb.JSONOneLine(state)
	_, err = db.db.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		return fmt.Errorf("m7: failed to update migration checkpoint: %v", err)
	}

	// Commit transactions.
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("m7: failed to commit changes: %v", err)
	}

	return nil
}

func warnUser(db *IndexerDb, maxRound uint32) error {
	query := "SELECT COUNT(*) FROM account WHERE (created_at IS NULL) OR (created_at <= $1)"
	row := db.db.QueryRow(query, maxRound)

	var count uint64
	err := row.Scan(&count)
	if err != nil {
		return fmt.Errorf("m7: unable to query the number of rows: %v", err)
	}

	// This many accounts need about 4GB of memory.
	threshold := 10000000 / 3 * 4
	if count > uint64(threshold) {
		db.log.Print("The current migration (m7) is likely to use more than 4GB of RAM.")

		envVar := "FORCEM7"
		if _, ok := os.LookupEnv(envVar); !ok {
			db.log.Printf("To avoid overuse the process has stopped automatically. "+
				"To force start the migration, please set the environment variable %s to TRUE.", envVar)
			return fmt.Errorf("m7: set %s environment variable to force the migration", envVar)
		}
	}

	return nil
}

func getAccountsFirstUsed(db *IndexerDb, maxRound uint32, specialAccounts idb.SpecialAccounts) (map[sdk_types.Address]txnID, error) {
	res := make(map[sdk_types.Address]txnID)

	// Read all transactions in arbitrary order.
	db.log.Print("querying transactions (pass 1)")
	query := "SELECT round, intra, txnbytes FROM txn WHERE round <= $1"
	rows, err := db.db.Query(query, maxRound)
	if err != nil {
		return nil, fmt.Errorf("m7: unable to query transactions (pass 1): %v", err)
	}
	defer rows.Close()

	numRows := 0
	db.log.Print("started reading transactions (pass 1)")
	for rows.Next() {
		var round uint32
		var intra uint32
		var txnBytes []byte
		err = rows.Scan(&round, &intra, &txnBytes)
		if err != nil {
			return nil, fmt.Errorf("m7: unable to scan a row: %v", err)
		}

		var stxn types.SignedTxnWithAD
		err = msgpack.Decode(txnBytes, &stxn)
		if err != nil {
			return nil, fmt.Errorf("m7: unable to scan a row: %v", err)
		}

		participants := getParticipants(stxn)
		for _, address := range participants {
			// Don't update special accounts (m10 fixes this).
			if (address != specialAccounts.RewardsPool) && (address != specialAccounts.FeeSink) {
				txnID := txnID{round, intra}

				storedTxnID, ok := res[address]
				if !ok {
					res[address] = txnID
				} else {
					if txnIDLess(txnID, storedTxnID) {
						res[address] = txnID
					}
				}
			}
		}

		numRows++
		if numRows%1000000 == 0 {
			db.log.Printf("read %d transactions (pass 1)", numRows)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("m7: error scanning rows: %v", err)
	}
	db.log.Print("finished reading transactions (pass 1)")

	return res, nil
}

// Set Created, Deleted for accounts with no transactions.
// Genesis accounts could have this property.
func getAccountsWithoutTxnData(db *IndexerDb, maxRound uint32, specialAccounts idb.SpecialAccounts, accountsFirstUsed map[sdk_types.Address]txnID) ([]addressAccountData, error) {
	// Query accounts.
	res := []addressAccountData{}

	options := idb.AccountQueryOptions{}
	options.IncludeDeleted = true

	db.log.Print("querying accounts")
	accountCh, _ := db.GetAccounts(context.Background(), options)

	// Read all accounts.
	numRows := 0
	db.log.Print("started reading accounts")
	for accountRow := range accountCh {
		if accountRow.Error != nil {
			return nil, fmt.Errorf("m7: problem querying accounts: %v", accountRow.Error)
		}

		if (accountRow.Account.CreatedAtRound == nil) ||
			(*accountRow.Account.CreatedAtRound <= uint64(maxRound)) {
			address, err := sdk_types.DecodeAddress(accountRow.Account.Address)
			if err != nil {
				return nil, fmt.Errorf("m7: failed to decode address %s err: %v",
					accountRow.Account.Address, err)
			}

			// Don't update special accounts (m10 fixes this)
			if (address != specialAccounts.FeeSink) && (address != specialAccounts.RewardsPool) {
				if _, ok := accountsFirstUsed[address]; !ok {
					accountData := initM7AccountData()

					accountData.account.createdValid = true
					accountData.account.created = 0
					accountData.account.deletedValid = true
					accountData.account.deleted = false

					res = append(res, addressAccountData{address, accountData})
				}
			}

			numRows++
			if numRows%1000000 == 0 {
				db.log.Printf("m7: read %d accounts", numRows)
			}
		}
	}
	db.log.Print("m7: finished reading accounts")

	return res, nil
}

func updateAccounts(db *IndexerDb, specialAccounts idb.SpecialAccounts, accountsFirstUsed map[sdk_types.Address]txnID, readyAccountData []addressAccountData, state *MigrationState) error {
	// Query all transactions again.
	batchSize := 500

	accountDataMap := make(map[sdk_types.Address]*m7AccountData)
	assetDataMap := make(map[uint32]createClose)

	numAccounts := len(accountsFirstUsed)
	numAccountsUpdated := 0

	db.log.Print("m7: querying transactions (pass 2)")
	query := "SELECT round, intra, txnbytes, asset FROM txn WHERE round <= $1 ORDER BY round DESC, intra DESC"
	rows, err := db.db.Query(query, state.NextRound)
	if err != nil {
		return fmt.Errorf("m7: unable to query transactions: %v", err)
	}
	defer rows.Close()

	var writeDuration time.Duration = 0

	// Loop through all transactions and compute account data.
	db.log.Print("m7: started reading transactions (pass 2)")
	numRows := 0
	numBatches := 0
	for rows.Next() {
		var round uint32
		var intra uint32
		var txnBytes []byte
		var assetID uint32
		err = rows.Scan(&round, &intra, &txnBytes, &assetID)
		if err != nil {
			return fmt.Errorf("m7: unable to scan a row: %v", err)
		}

		var stxn types.SignedTxnWithAD
		err = msgpack.Decode(txnBytes, &stxn)
		if err != nil {
			return fmt.Errorf("m7: unable to parse txnBytes round: %d intra: %d err: %v",
				round, intra, err)
		}

		participants := getParticipants(stxn)
		for _, address := range participants {
			// Don't update special accounts (m10 fixes this).
			if (address != specialAccounts.RewardsPool) && (address != specialAccounts.FeeSink) {
				accountData, ok := accountDataMap[address]
				if !ok {
					accountData = initM7AccountData()
					accountDataMap[address] = accountData
				}

				updateAccountData(address, round, assetID, stxn, accountData, assetDataMap)

				firstUsed, ok := accountsFirstUsed[address]
				if !ok {
					return fmt.Errorf("m7: accountFirstUsed does not contain address: %v", address)
				}
				if (txnID{uint32(round), uint32(intra)}) == firstUsed {
					delete(accountDataMap, address)
					delete(accountsFirstUsed, address)

					if (state.PointerRound == nil) ||
						txnIDLess(firstUsed, txnID{uint32(*state.PointerRound), uint32(*state.PointerIntra)}) {
						readyAccountData = append(readyAccountData, addressAccountData{address, accountData})
						if len(readyAccountData) >= batchSize {
							start := time.Now()
							// Write account data to the database. This function also removes any assets
							// from `assetDataMap` that are not needed anymore.
							err = m7RewardsAndDatesPart2UpdateAccounts(
								db, readyAccountData, assetDataMap, firstUsed, state)
							if err != nil {
								return err
							}
							writeDuration += time.Since(start)

							numAccountsUpdated += len(readyAccountData)
							readyAccountData = readyAccountData[:0]

							numBatches++
							if numBatches%100 == 0 {
								db.log.Printf("m7: written %d (%.2f%%) accounts, %d batches; "+
									"writing has taken %v in total",
									numAccountsUpdated, float64(100*numAccountsUpdated)/float64(numAccounts),
									numBatches, writeDuration)
							}
						}
					} else {
						// Delete assets created by `address`.
						if accountData.additional != nil {
							for assetID := range accountData.additional.asset {
								delete(assetDataMap, assetID)
							}
						}
					}
				}
			}
		}

		numRows++
		if numRows%1000000 == 0 {
			db.log.Printf("m7: read %d transactions (pass 2)", numRows)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("m7: error scanning rows: %v", err)
	}
	db.log.Print("m7: finished reading transactions (pass 2)")

	// Update remaining accounts.
	err = m7RewardsAndDatesPart2UpdateAccounts(db, readyAccountData, assetDataMap, txnID{0, 0}, state)
	if err != nil {
		return err
	}

	if len(accountsFirstUsed) > 0 {
		return fmt.Errorf("m7: len(accountsFirstUsed): %d > 0", len(accountsFirstUsed))
	}
	if len(assetDataMap) > 0 {
		return fmt.Errorf("m7: len(assetDataMap): %d > 0", len(assetDataMap))
	}

	db.log.Print("m7: finished updating accounts")

	return nil
}

// m7RewardsAndDatesPart2 computes the cumulative rewards for each account one at a time.
func m7RewardsAndDatesPart2(db *IndexerDb, state *MigrationState) error {
	db.log.Print("m7 account cumulative rewards migration starting")

	// Skip the work if all accounts have previously been updated.
	if (state.PointerRound == nil) || (*state.PointerRound != 0) || (*state.PointerIntra != 0) {
		maxRound := uint32(state.NextRound)

		// Get the number of accounts to potentially warn the user about high memory usage.
		err := warnUser(db, maxRound)
		if err != nil {
			return err
		}
		// Get special accounts, so that we can ignore them throughout the migration. A later migration
		// handles them.
		specialAccounts, err := db.GetSpecialAccounts()
		if err != nil {
			return fmt.Errorf("m7: unable to get special accounts: %v", err)
		}
		// Get the transaction id that created each account. This function simple loops over all
		// transactions from rounds <= `maxRound` in arbitrary order.
		accountsFirstUsed, err := getAccountsFirstUsed(db, maxRound, specialAccounts)
		if err != nil {
			return err
		}
		// Get account data for accounts without transactions such as genesis accounts.
		// This function reads the `account` table but only considers accounts created before or at
		// `maxRound`.
		readyAccountData, err := getAccountsWithoutTxnData(
			db, maxRound, specialAccounts, accountsFirstUsed)
		if err != nil {
			return err
		}
		// Finally, read all transactions from most recent to oldest, update rewards and create/close dates,
		// and write this account data to the database. To save memory, this function removes account's
		// data as soon as we reach the transaction that created this account at which point older
		// transactions cannot update its state. It writes account data to the database in batches.
		err = updateAccounts(db, specialAccounts, accountsFirstUsed, readyAccountData, state)
		if err != nil {
			return err
		}
	}

	// Update migration state.
	state.NextMigration++
	state.NextRound = 0
	state.PointerRound = nil
	state.PointerIntra = nil
	migrationStateJSON := idb.JSONOneLine(state)
	_, err := db.db.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		return fmt.Errorf("m7: failed to write final migration state: %v", err)
	}

	return nil
}

// sqlMigration executes a sql statements as the entire migration.
func sqlMigration(db *IndexerDb, state *MigrationState, sqlLines []string) error {
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	thisMigration := state.NextMigration
	tx, err := db.db.BeginTx(context.Background(), &serializable)
	if err != nil {
		db.log.WithError(err).Errorf("migration %d tx err", thisMigration)
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first
	for i, cmd := range sqlLines {
		_, err = tx.Exec(cmd)
		if err != nil {
			db.log.WithError(err).Errorf("migration %d sql[%d] err", thisMigration, i)
			return err
		}
	}
	state.NextMigration++
	migrationStateJSON := idb.JSONOneLine(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("migration %d meta err", thisMigration)
		return err
	}
	err = tx.Commit()
	if err != nil {
		db.log.WithError(err).Errorf("migration %d commit err", thisMigration)
		return err
	}
	db.log.Printf("migration %d done", thisMigration)
	return nil
}

const txidMigrationErrMsg = "ERROR migrating txns for txid, stopped, will retry on next indexer startup"

type migrationContext struct {
	db      *IndexerDb
	state   *MigrationState
	lastlog time.Time
}
type txidFiuxpMigrationContext migrationContext

// read batches of at least 2 blocks or up to 10000 txns,
// write a temporary table, UPDATE from temporary table into txn.
// repeat until all txns consumed.
func (mtxid *txidFiuxpMigrationContext) asyncTxidFixup() (err error) {
	db := mtxid.db
	state := mtxid.state
	db.log.Println("txid fixup migration starting")
	txns := db.YieldTxns(context.Background(), uint64(state.NextRound))
	batch := make([]idb.TxnRow, 15000)
	txInBatch := 0
	roundsInBatch := 0
	prevBatchRound := uint64(math.MaxUint64)
	for txr := range txns {
		if txr.Error != nil {
			db.log.WithError(txr.Error).Errorf("ERROR migrating txns for txid rewrite")
			err = txr.Error
			return
		}
		if txr.Round != prevBatchRound {
			if txInBatch > 10000 {
				err = mtxid.putTxidFixupBatch(batch[:txInBatch])
				if err != nil {
					return
				}
				// start next batch
				batch[0] = txr
				txInBatch = 1
				roundsInBatch = 1
				prevBatchRound = txr.Round
				continue
			}
			roundsInBatch++
			prevBatchRound = txr.Round
		}
		batch[txInBatch] = txr
		txInBatch++
		if roundsInBatch > 2 && txInBatch > 10000 {
			// post the first complete rounds
			split := txInBatch - 1
			for batch[split].Round == txr.Round {
				split--
			}
			split++ // now the first txn of the incomplete current round
			err = mtxid.putTxidFixupBatch(batch[:split])
			if err != nil {
				return
			}
			// move incomplete round to next batch
			copy(batch, batch[split:txInBatch])
			txInBatch = txInBatch - split
			roundsInBatch = 1
			prevBatchRound = txr.Round
			continue
		}
	}
	if txInBatch > 0 {
		err = mtxid.putTxidFixupBatch(batch[:txInBatch])
		if err != nil {
			return
		}
	}
	// all done, mark migration state
	state.NextMigration++
	state.NextRound = 0
	migrationStateJSON := idb.JSONOneLine(state)
	err = db.setMetastate(migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("%s, error setting final migration state", txidMigrationErrMsg)
		return
	}
	db.log.Println("txid fixup migration finished")
	return nil
}

type txidFixupRow struct {
	round uint64
	intra int
	txid  string // base32 string
}

func (mtxid *txidFiuxpMigrationContext) putTxidFixupBatch(batch []idb.TxnRow) error {
	db := mtxid.db
	state := mtxid.state
	minRound := batch[0].Round
	maxRound := batch[0].Round
	for _, txr := range batch {
		if txr.Round < minRound {
			minRound = txr.Round
		}
		if txr.Round > maxRound {
			maxRound = txr.Round
		}
	}
	headers, err := mtxid.readHeaders(minRound, maxRound)
	if err != nil {
		return err
	}
	outrows := make([]txidFixupRow, len(batch))
	for i, txr := range batch {
		block := headers[txr.Round]
		proto, err := types.Protocol(string(block.CurrentProtocol))
		if err != nil {
			db.log.WithError(err).Errorf("%s, proto", txidMigrationErrMsg)
			return err
		}
		var stxn types.SignedTxnInBlock
		err = msgpack.Decode(txr.TxnBytes, &stxn)
		if err != nil {
			db.log.WithError(err).Errorf("%s, txnb msgpack err", txidMigrationErrMsg)
			return err
		}
		if stxn.HasGenesisID {
			stxn.Txn.GenesisID = block.GenesisID
		}
		if stxn.HasGenesisHash || proto.RequireGenesisHash {
			stxn.Txn.GenesisHash = block.GenesisHash
		}
		outrows[i].round = txr.Round
		outrows[i].intra = txr.Intra
		outrows[i].txid = crypto.TransactionIDString(stxn.Txn)
	}

	// do a transaction to update a batch
	tx, err := db.db.BeginTx(context.Background(), &serializable)
	if err != nil {
		db.log.WithError(err).Errorf("%s, batch tx err", txidMigrationErrMsg)
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first
	// Check that migration state in db is still what we think it is
	txstate, err := db.getMigrationState()
	if err != nil {
		db.log.WithError(err).Errorf("%s, get m state err", txidMigrationErrMsg)
		return err
	} else if state == nil {
		// no previous state, ok
	} else {
		// Check that we're beyond the rewards migration task
		if state.NextMigration != txstate.NextMigration || state.NextRound != txstate.NextRound {
			db.log.Printf("%s, migration state changed when we weren't looking: %v -> %v", txidMigrationErrMsg, state, txstate)
		}
	}

	// _sometimes_ the temp table exists from the previous cycle.
	// So, 'create if not exists' and truncate.
	_, err = tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS txid_fix_batch (round bigint NOT NULL, intra smallint NOT NULL, txid bytea NOT NULL, PRIMARY KEY ( round, intra ))`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, create temp err", txidMigrationErrMsg)
		return err
	}
	_, err = tx.Exec(`TRUNCATE TABLE txid_fix_batch`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, truncate temp err", txidMigrationErrMsg)
		return err
	}
	batchadd, err := tx.Prepare(`COPY txid_fix_batch (round, intra, txid) FROM STDIN`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp prepare err", txidMigrationErrMsg)
		return err
	}
	defer batchadd.Close()
	for _, tr := range outrows {
		_, err = batchadd.Exec(tr.round, tr.intra, tr.txid)
		if err != nil {
			db.log.WithError(err).Errorf("%s, temp row err", txidMigrationErrMsg)
			return err
		}
	}
	_, err = batchadd.Exec()
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp empty row err", txidMigrationErrMsg)
		return err
	}
	err = batchadd.Close()
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp add close err", txidMigrationErrMsg)
		return err
	}

	_, err = tx.Exec(`UPDATE txn SET txid = x.txid FROM txid_fix_batch x WHERE txn.round = x.round AND txn.intra = x.intra`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, update err", txidMigrationErrMsg)
		return err
	}
	txstate.NextRound = int64(maxRound + 1)
	migrationStateJSON := idb.JSONOneLine(txstate)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("%s, set metastate err", txidMigrationErrMsg)
		return err
	}
	err = tx.Commit()
	if err != nil {
		db.log.WithError(err).Errorf("%s, batch commit err", txidMigrationErrMsg)
		return err
	}
	mtxid.state = txstate
	_, err = db.db.Exec(`DROP TABLE IF EXISTS txid_fix_batch`)
	if err != nil {
		db.log.WithError(err).Errorf("warning txid migration, drop temp err")
		// we don't actually care; psql should garbage collect the temp table eventually
	}
	now := time.Now()
	dt := now.Sub(mtxid.lastlog)
	if dt > 5*time.Second {
		mtxid.lastlog = now
		db.log.Printf("txid fixup migration through %d", maxRound)
	}
	return nil
}

func (mtxid *txidFiuxpMigrationContext) readHeaders(minRound, maxRound uint64) (map[uint64]types.Block, error) {
	db := mtxid.db
	return readHeaders(db, minRound, maxRound)
}

func readHeaders(db *IndexerDb, minRound, maxRound uint64) (map[uint64]types.Block, error) {
	rows, err := db.db.Query(`SELECT round, header FROM block_header WHERE round >= $1 AND round <= $2`, minRound, maxRound)
	if err != nil {
		db.log.WithError(err).Errorf("%s, header err", txidMigrationErrMsg)
		return nil, err
	}
	defer rows.Close()
	headers := make(map[uint64]types.Block)
	for rows.Next() {
		var round int64
		var headerjson []byte
		err = rows.Scan(&round, &headerjson)
		if err != nil {
			db.log.WithError(err).Errorf("%s, header row err", txidMigrationErrMsg)
			return nil, err
		}
		var tblock types.Block
		json.Decode(headerjson, &tblock)
		headers[uint64(round)] = tblock
	}
	if err := rows.Err(); err != nil {
		db.log.WithError(err).Errorf("%s, error reading rows", txidMigrationErrMsg)
		return nil, err
	}
	return headers, nil
}

// This was added during a hotfix
func m4accountIndices(db *IndexerDb, state *MigrationState) error {
	sqlLines := []string{
		"CREATE INDEX IF NOT EXISTS account_asset_by_addr ON account_asset ( addr )",
		"CREATE INDEX IF NOT EXISTS asset_by_creator_addr ON asset ( creator_addr )",
		"CREATE INDEX IF NOT EXISTS app_by_creator ON app ( creator )",
		"CREATE INDEX IF NOT EXISTS account_app_by_addr ON account_app ( addr )",
	}
	return sqlMigration(db, state, sqlLines)
}

// Record round at which behavior changed for encoding txn.txn JSON.
// A future migration should go back and apply new encoding to prior txn rows then delete this row in metastate.
func m5MarkTxnJSONSplit(db *IndexerDb, state *MigrationState) error {
	sqlLines := []string{
		`INSERT INTO metastate (k,v) SELECT 'm5MarkTxnJSONSplit', m.v FROM metastate m WHERE m.k = 'state'`,
	}
	return sqlMigration(db, state, sqlLines)
}

func m8StaleClosedAccounts(db *IndexerDb, state *MigrationState) error {
	sqlLines := []string{
		// remove stale data from closed accounts
		`UPDATE account SET account_data = NULL WHERE microalgos = 0`,
		// don't leave empty arrays around
		`UPDATE app SET params = app.params - 'gs'  WHERE app.params ->> 'gs' = '[]'`,
		`UPDATE account_app SET localstate = account_app.localstate - 'tkv' WHERE account_app.localstate ->> 'tkv' = '[]'`,
	}
	return sqlMigration(db, state, sqlLines)
}

var jsonFixupTxnQuery string

func init() {
	jsonFixupTxnQuery = fmt.Sprintf(`SELECT t.round, t.intra, t.txnbytes, t.txn FROM txn t WHERE t.round > $1 AND t.round <= $2 ORDER BY round, intra LIMIT %d`, txnQueryBatchSize)
}

type jsonFixupTxnRow struct {
	Round    uint64
	Intra    int
	TxnBytes []byte
	JSON     []byte

	Error error
}

func (db *IndexerDb) yieldJSONFixupTxnsThread(ctx context.Context, rows *sql.Rows, lastRound int64, results chan<- jsonFixupTxnRow) {
	keepGoing := true
	for keepGoing {
		keepGoing = false
		rounds := make([]uint64, txnQueryBatchSize)
		intras := make([]int, txnQueryBatchSize)
		txnbytess := make([][]byte, txnQueryBatchSize)
		txnjsons := make([][]byte, txnQueryBatchSize)
		pos := 0
		// read from db
		for rows.Next() {
			var round uint64
			var intra int
			var txnbytes []byte
			var txnjson []byte
			err := rows.Scan(&round, &intra, &txnbytes, &txnjson)
			if err != nil {
				var row jsonFixupTxnRow
				row.Error = err
				results <- row
				rows.Close()
				close(results)
				return
			}

			rounds[pos] = round
			intras[pos] = intra
			txnbytess[pos] = txnbytes
			txnjsons[pos] = txnjson
			pos++

			keepGoing = true
		}
		if err := rows.Err(); err != nil {
			results <- jsonFixupTxnRow{Error: err}
			rows.Close()
			close(results)
			return
		}
		rows.Close()
		if pos == 0 {
			break
		}
		if pos == txnQueryBatchSize {
			// figure out last whole round we got
			lastpos := pos - 1
			lastround := rounds[lastpos]
			lastpos--
			for lastpos >= 0 && rounds[lastpos] == lastround {
				lastpos--
			}
			if lastpos == 0 {
				panic("unwound whole fetch!")
			}
			pos = lastpos + 1
		}
		// yield to chan
		for i := 0; i < pos; i++ {
			var row jsonFixupTxnRow
			row.Round = rounds[i]
			row.Intra = intras[i]
			row.TxnBytes = txnbytess[i]
			row.JSON = txnjsons[i]
			select {
			case <-ctx.Done():
				close(results)
				return
			case results <- row:
			}
		}
		if keepGoing {
			var err error
			prevRound := rounds[pos-1]
			rows, err = db.db.QueryContext(ctx, jsonFixupTxnQuery, prevRound, lastRound)
			if err != nil {
				results <- jsonFixupTxnRow{Error: err}
				break
			}
		}
	}
	close(results)
}

const m9ErrPrefix = "m9 txn json fixup"

// read batches of at least 2 blocks or up to 10000 txns,
// write a temporary table, UPDATE from temporary table into txn.
// repeat until all txns consumed.
func m9TxnJSONEncoding(db *IndexerDb, state *MigrationState) (err error) {
	db.log.Infof("txn json fixup migration starting")
	row := db.db.QueryRow(`SELECT (v -> 'account_round')::bigint FROM metastate WHERE k = 'm7MarkTxnJSONSplit'`)
	var lastRound int64
	err = row.Scan(&lastRound)
	if err == sql.ErrNoRows {
		// Indexer may be new after m7, marking it as done without running it, so we don't need to do anything here.
		state.NextMigration++
		migrationStateJSON := json.Encode(state)
		_, err = db.db.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
		if err != nil {
			db.log.WithError(err).Errorf("%s, meta err", m9ErrPrefix)
		}
		return err
	} else if err != nil {
		db.log.WithError(err).Errorf("%s, getting m7MarkTxnJSONSplit", m9ErrPrefix)
		return err
	}

	prevRound := state.NextRound - 1
	ctx, cf := context.WithCancel(context.Background())
	defer cf()
	rows, err := db.db.QueryContext(ctx, jsonFixupTxnQuery, prevRound, lastRound)
	if err != nil {
		db.log.WithError(err).Errorf("%s, getting txns", m9ErrPrefix)
	}
	txns := make(chan jsonFixupTxnRow, 10)
	go db.yieldJSONFixupTxnsThread(ctx, rows, lastRound, txns)
	batch := make([]jsonFixupTxnRow, 15000)
	txInBatch := 0
	roundsInBatch := 0
	prevBatchRound := uint64(math.MaxUint64)
	for txr := range txns {
		if txr.Error != nil {
			db.log.WithError(txr.Error).Errorf("%s, reading txns", m9ErrPrefix)
			err = txr.Error
			return
		}
		if txr.Round != prevBatchRound {
			if txInBatch > 10000 {
				err = putTxnJSONBatch(db, state, batch[:txInBatch])
				if err != nil {
					return
				}
				// start next batch
				batch[0] = txr
				txInBatch = 1
				roundsInBatch = 1
				prevBatchRound = txr.Round
				continue
			}
			roundsInBatch++
			prevBatchRound = txr.Round
		}
		batch[txInBatch] = txr
		txInBatch++
		if roundsInBatch > 2 && txInBatch > 10000 {
			// post the first complete rounds
			split := txInBatch - 1
			for batch[split].Round == txr.Round {
				split--
			}
			split++ // now the first txn of the incomplete current round
			err = putTxnJSONBatch(db, state, batch[:split])
			if err != nil {
				return
			}
			// move incomplete round to next batch
			copy(batch, batch[split:txInBatch])
			txInBatch = txInBatch - split
			roundsInBatch = 1
			prevBatchRound = txr.Round
			continue
		}
	}
	if txInBatch > 0 {
		err = putTxnJSONBatch(db, state, batch[:txInBatch])
		if err != nil {
			return
		}
	}
	// all done, mark migration state
	state.NextMigration++
	state.NextRound = 0
	migrationStateJSON := string(json.Encode(state))
	err = db.setMetastate(migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("%s, error setting final migration state", m9ErrPrefix)
		return
	}
	db.log.Println("txn json fixup migration finished")
	return nil
}

type jsonFixupUpdateRow struct {
	round   uint64
	intra   int
	txnJSON string
}

func putTxnJSONBatch(db *IndexerDb, state *MigrationState, batch []jsonFixupTxnRow) error {
	minRound := batch[0].Round
	maxRound := batch[0].Round
	for _, txr := range batch {
		if txr.Round < minRound {
			minRound = txr.Round
		}
		if txr.Round > maxRound {
			maxRound = txr.Round
		}
	}
	headers, err := readHeaders(db, minRound, maxRound)
	if err != nil {
		return err
	}
	outrows := make([]jsonFixupUpdateRow, len(batch))
	pos := 0
	for _, txr := range batch {
		block := headers[txr.Round]
		proto, err := types.Protocol(string(block.CurrentProtocol))
		if err != nil {
			db.log.WithError(err).Errorf("%s, proto", m9ErrPrefix)
			return err
		}
		var stxn types.SignedTxnInBlock
		err = msgpack.Decode(txr.TxnBytes, &stxn)
		if err != nil {
			db.log.WithError(err).Errorf("%s, txnb msgpack err", m9ErrPrefix)
			return err
		}
		if stxn.HasGenesisID {
			stxn.Txn.GenesisID = block.GenesisID
		}
		if stxn.HasGenesisHash || proto.RequireGenesisHash {
			stxn.Txn.GenesisHash = block.GenesisHash
		}
		js := stxnToJSON(stxn.SignedTxnWithAD)
		if js == string(txr.JSON) {
			outrows[pos].round = txr.Round
			outrows[pos].intra = txr.Intra
			outrows[pos].txnJSON = js
			pos++
		}
	}

	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	// do a transaction to update a batch
	tx, err := db.db.Begin()
	if err != nil {
		db.log.WithError(err).Errorf("%s, batch tx err", m9ErrPrefix)
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first
	// Check that migration state in db is still what we think it is
	txstate, err := db.getMigrationState()
	if err != nil {
		db.log.WithError(err).Errorf("%s, get m state err", m9ErrPrefix)
		return err
	} else if state == nil {
		// no previous state, ok
	} else {
		if state.NextMigration != txstate.NextMigration || state.NextRound != txstate.NextRound {
			return fmt.Errorf("%s, migration state changed when we weren't looking: %v -> %v", m9ErrPrefix, state, txstate)
		}
	}

	// _sometimes_ the temp table exists from the previous cycle.
	// So, 'create if not exists' and truncate.
	_, err = tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS txjson_fix_batch (round bigint NOT NULL, intra smallint NOT NULL, txn jsonb NOT NULL, PRIMARY KEY ( round, intra ))`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, create temp err", m9ErrPrefix)
		return err
	}
	_, err = tx.Exec(`TRUNCATE TABLE txjson_fix_batch`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, truncate temp err", m9ErrPrefix)
		return err
	}
	batchadd, err := tx.Prepare(`COPY txjson_fix_batch (round, intra, txn) FROM STDIN`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp prepare err", m9ErrPrefix)
		return err
	}
	defer batchadd.Close()
	for _, tr := range outrows {
		_, err = batchadd.Exec(tr.round, tr.intra, tr.txnJSON)
		if err != nil {
			db.log.WithError(err).Errorf("%s, temp row err", m9ErrPrefix)
			return err
		}
	}
	_, err = batchadd.Exec()
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp empty row err", m9ErrPrefix)
		return err
	}
	err = batchadd.Close()
	if err != nil {
		db.log.WithError(err).Errorf("%s, temp add close err", m9ErrPrefix)
		return err
	}

	_, err = tx.Exec(`UPDATE txn SET txn = x.txn FROM txjson_fix_batch x WHERE txn.round = x.round AND txn.intra = x.intra`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, update err", m9ErrPrefix)
		return err
	}
	txstate.NextRound = int64(maxRound + 1)
	migrationStateJSON := json.Encode(txstate)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		db.log.WithError(err).Errorf("%s, set metastate err", m9ErrPrefix)
		return err
	}
	err = tx.Commit()
	if err != nil {
		db.log.WithError(err).Errorf("%s, batch commit err", m9ErrPrefix)
		return err
	}
	_, err = db.db.Exec(`DROP TABLE IF EXISTS txjson_fix_batch`)
	if err != nil {
		db.log.WithError(err).Errorf("%s, warning, drop temp err", m9ErrPrefix)
		// we don't actually care; psql should garbage collect the temp table eventually
	}
	return nil
}

func m10SpecialAccountCleanup(db *IndexerDb, state *MigrationState) error {
	accounts, err := db.GetSpecialAccounts()
	if err != nil {
		return fmt.Errorf("unable to get special accounts: %v", err)
	}

	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	tx, err := db.db.BeginTx(context.Background(), &serializable)
	if err != nil {
		return fmt.Errorf("failed to begin m10 migration: %v", err)
	}
	defer tx.Rollback() // ignored if .Commit() first

	initstmt, err := tx.Prepare(`UPDATE account SET deleted=false, created_at=0 WHERE addr=$1`)
	if err != nil {
		return fmt.Errorf("failed to prepare m10 query: %v", err)
	}

	for _, account := range []string{accounts.FeeSink.String(), accounts.RewardsPool.String()} {
		address, err := sdk_types.DecodeAddress(account)
		if err != nil {
			return fmt.Errorf("failed to decode address: %v", err)
		}
		initstmt.Exec(address[:])
	}

	upsertMigrationStateTx(tx, state, true)
	if err != nil {
		return fmt.Errorf("m10 metastate upsert error: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("m10 commit error: %v", err)
	}

	return nil
}

// Search for the freeze accounts freeze transactions.
// The Transaction filter doesn't support searching on multiple addresses.
// $1 = freeze account - we have to assume that the freeze account has not been changed, the query does not perform well otherwise.
// $2 = the holder account
// $3 = assetID
// $4 = round limit
var freezeTransactionsQuery = `select count(*) from txn_participation p JOIN txn t ON t.round = p.round AND p.intra = t.intra
WHERE p.addr = $1
AND (t.txn -> 'txn' ->> 'fadd' = $2)
AND t.asset = $3
AND p.round > $4
LIMIT 1`

// Helper functions for m11 migration
func updateFrozenState(db *IndexerDb, assetID uint64, closedAt *uint64, creator, freeze, holder types.Address) error {
	// Semi-blocking migration.
	// Hold accountingLock for the duration of the Transaction search + account_asset update.
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	minRound := uint64(0)
	if closedAt != nil {
		minRound = *closedAt
	}

	holderb64 := base64.StdEncoding.EncodeToString(holder[:])
	row := db.db.QueryRow(freezeTransactionsQuery, freeze[:], holderb64, assetID, minRound)
	var found uint64
	err := row.Scan(&found)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// If there are any freeze transactions then the default no longer applies.
	// Exit early if the asset was frozen
	if found != 0 {
		return nil
	}

	// If there were no freeze transactions, re-initialize the frozen value.
	frozen := !bytes.Equal(creator[:], holder[:])
	db.db.Exec(`UPDATE account_asset SET frozen = $1 WHERE assetid = $2 and addr = $3`, frozen, assetID, holder[:])

	return nil
}

func getAsset(db *IndexerDb, assetID uint64) (idb.AssetRow, error) {
	assets, _ := db.Assets(context.Background(), idb.AssetsQuery{AssetID: assetID})
	num := 0
	var asset idb.AssetRow
	for assetRow := range assets {
		if assetRow.Error != nil {
			return idb.AssetRow{}, assetRow.Error
		}
		asset = assetRow
		num++
	}

	if num > 1 {
		return idb.AssetRow{}, fmt.Errorf("multiple assets returned for asset %d", assetID)
	}

	if num == 0 {
		return idb.AssetRow{}, fmt.Errorf("asset %d not found", assetID)
	}

	return asset, nil
}

func m11AssetHoldingFrozen(db *IndexerDb, state *MigrationState) error {
	defaultFrozenCache, err := db.GetDefaultFrozen()
	if err != nil {
		return fmt.Errorf("unable to get default frozen cache: %v", err)
	}

	// For all assets with default-frozen = true.
	for assetID := range defaultFrozenCache {
		asset, err := getAsset(db, assetID)
		if err != nil {
			return fmt.Errorf("unable to fetch asset %d: %v", assetID, err)
		}
		var creator types.Address
		copy(creator[:], asset.Creator)

		balances, _ := db.AssetBalances(context.Background(), idb.AssetBalanceQuery{AssetID: assetID})
		for balance := range balances {
			if balance.Error != nil {
				return fmt.Errorf("unable to process asset balance for asset %d: %v", assetID, err)
			}

			var holder types.Address
			copy(holder[:], balance.Address)

			err := updateFrozenState(db, assetID, balance.ClosedRound, creator, asset.Params.Freeze, holder)
			if balance.Error != nil {
				return fmt.Errorf("unable to process update frozen state asset %d / address %s: %v", assetID, holder.String(), err)
			}
		}
	}

	upsertMigrationState(db, state, true)
	if err != nil {
		return fmt.Errorf("m11 metastate upsert error: %v", err)
	}

	return nil
}

// Reusable update batch function. Provide a query and an array of argument arrays to pass  to that query.
func updateBatch(db *IndexerDb, updateQuery string, data [][]interface{}) error {
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // ignored if .Commit() first

	update, err := tx.Prepare(updateQuery)
	if err != nil {
		return fmt.Errorf("error preparing update query: %v", err)
	}
	defer update.Close()

	for _, txpr := range data {
		_, err = update.Exec(txpr...)
		if err != nil {
			return fmt.Errorf("problem updating row (%v): %v", txpr, err)
		}
	}

	return tx.Commit()
}

// FixFreezeLookupMigration is a migration to add txn_participation entries for freeze address in freeze transactions.
func FixFreezeLookupMigration(db *IndexerDb, state *MigrationState) error {
	// Technically with this query no transactions are needed, and the accounting state doesn't need to be locked.
	updateQuery := "INSERT INTO txn_participation (addr, round, intra) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING"
	query := fmt.Sprintf("select decode(txn.txn->'txn'->>'fadd','base64'),round,intra from txn where typeenum = %d AND txn.txn->'txn'->'snd' != txn.txn->'txn'->'fadd'", idb.TypeEnumAssetFreeze)
	rows, err := db.db.Query(query)
	if err != nil {
		return fmt.Errorf("unable to query transactions: %v", err)
	}
	defer rows.Close()

	txprows := make([][]interface{}, 0)

	// Loop through all transactions and compute account data.
	db.log.Print("loop through all freeze transactions")
	for rows.Next() {
		var addr []byte
		var round, intra uint64
		err = rows.Scan(&addr, &round, &intra)
		if err != nil {
			return fmt.Errorf("error scanning row: %v", err)
		}

		txprows = append(txprows, []interface{}{addr, round, intra})

		if len(txprows) > 5000 {
			err = updateBatch(db, updateQuery, txprows)
			if err != nil {
				return fmt.Errorf("updating batch: %v", err)
			}
			txprows = txprows[:0]
		}
	}

	if rows.Err() != nil {
		return fmt.Errorf("error while processing freeze transactions: %v", rows.Err())
	}

	// Commit any leftovers
	if len(txprows) > 0 {
		err = updateBatch(db, updateQuery, txprows)
		if err != nil {
			return fmt.Errorf("updating batch: %v", err)
		}
	}

	// Update migration state
	return upsertMigrationState(db, state, true)
}

type account struct {
	address  sdk_types.Address
	closedAt uint64 // the round when the account was last closed
}

func getAccounts(db *sql.DB) ([]account, error) {
	query := "SELECT addr, closed_at FROM account WHERE closed_at IS NOT NULL AND deleted = false " +
		"AND account_data IS NOT NULL"
	rows, err := db.Query(query)
	if err != nil {
		return []account{}, err
	}
	defer rows.Close()

	var res []account

	for rows.Next() {
		var addrBytes []byte
		var closedAt sql.NullInt64

		err = rows.Scan(&addrBytes, &closedAt)
		if err != nil {
			return []account{}, err
		}

		var addr sdk_types.Address
		copy(addr[:], addrBytes)
		res = append(res, account{addr, uint64(closedAt.Int64)})
	}
	if err := rows.Err(); err != nil {
		return []account{}, err
	}

	return res, nil
}

func fixAuthAddr(db *IndexerDb, account account) error {
	f := func(ctx context.Context, tx *sql.Tx) error {
		defer tx.Rollback()

		rowsCh := make(chan idb.TxnRow)

		// This will not work properly if the account was closed and reopened in the same round
		// but that's unlikely to happen.
		trueValue := true
		tf := idb.TransactionFilter{
			Address:     account.address[:],
			AddressRole: idb.AddressRoleSender,
			RekeyTo:     &trueValue,
			MinRound:    account.closedAt,
			Limit:       1,
		}
		go func() {
			db.yieldTxns(ctx, tx, tf, rowsCh)
			close(rowsCh)
		}()

		found := false
		for txnRow := range rowsCh {
			if txnRow.Error != nil {
				return txnRow.Error
			}
			found = true
		}

		if found {
			return nil
		}

		// No results. Delete the key.
		db.log.Printf("clearing auth addr for account %s", account.address.String())

		query := "UPDATE account SET account_data = account_data - 'spend' WHERE addr = $1"
		_, err := tx.Exec(query, account.address[:])
		if err != nil {
			return err
		}

		return tx.Commit()
	}

	return db.txWithRetry(context.Background(), serializable, f)
}

func fixKeyreg(db *IndexerDb, account account) error {
	f := func(ctx context.Context, tx *sql.Tx) error {
		defer tx.Rollback()

		rowsCh := make(chan idb.TxnRow)

		tf := idb.TransactionFilter{
			Address:     account.address[:],
			AddressRole: idb.AddressRoleSender,
			MinRound:    account.closedAt,
			TypeEnum:    idb.TypeEnumKeyreg,
			Limit:       1,
		}
		go func() {
			db.yieldTxns(ctx, tx, tf, rowsCh)
			close(rowsCh)
		}()

		found := false
		for txnRow := range rowsCh {
			if txnRow.Error != nil {
				return txnRow.Error
			}
			found = true
		}

		if found {
			return nil
		}

		// No results. Delete keyreg fields.
		db.log.Printf("clearing keyreg fields for account %s", account.address.String())

		query := "UPDATE account SET account_data = account_data - 'vote' - 'sel' - 'onl' - " +
			"'voteFst' - 'voteLst' - 'voteKD' WHERE addr = $1"
		_, err := tx.Exec(query, account.address[:])
		if err != nil {
			return err
		}

		return tx.Commit()
	}

	return db.txWithRetry(context.Background(), serializable, f)
}

// ClearAccountDataMigration clears account data for accounts that have been closed.
func ClearAccountDataMigration(db *IndexerDb, state *MigrationState) error {
	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	// Clear account_data column for deleted accounts.
	query := "UPDATE account SET account_data = NULL WHERE deleted = true;"
	if _, err := db.db.Exec(query); err != nil {
		return fmt.Errorf("error clearing deleted accounts: %v", err)
	}

	// Clear account data for some reopened accounts.
	accounts, err := getAccounts(db.db)
	if err != nil {
		return fmt.Errorf("error getting accounts: %v", err)
	}
	db.log.Printf("checking %d accounts that are reopened and have account data", len(accounts))

	for _, account := range accounts {
		if err := fixAuthAddr(db, account); err != nil {
			return fmt.Errorf("error clearing auth addr: %v", err)
		}
		if err := fixKeyreg(db, account); err != nil {
			return fmt.Errorf("error clearing keyreg fields: %v", err)
		}
	}

	state.NextMigration++
	migrationStateJSON := idb.JSONOneLine(state)
	_, err = db.db.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		return fmt.Errorf("metastate upsert error: %v", err)
	}

	return nil
}
