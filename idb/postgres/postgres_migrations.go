// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package postgres

import (
	"context"
	"database/sql"
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

// rewardsMigrationIndex is the index of m5RewardsAndDatesPart2.
const rewardsMigrationIndex = 5

func init() {
	migrations = []migrationStruct{
		{m0fixupTxid, false, "Recompute the txid with corrected algorithm."},
		{m1fixupBlockTime, true, "Adjust block time to UTC timezone."},
		{m2apps, true, "Update DB Schema for Algorand application support."},
		{m3acfgFix, false, "Recompute asset configurations with corrected merge function."},
		{m4RewardsAndDatesPart1, true, "Update DB Schema for cumulative account reward support and creation dates."},
		{m5RewardsAndDatesPart2, false, "Compute cumulative account rewards for all accounts."},
		{m6MarkTxnJSONSplit, true, "record round at which txn json recording changes, for future migration to fixup prior records"},
	}

	// Verify ensure the constant is pointing to the right index
	var m5Ptr postgresMigrationFunc = m5RewardsAndDatesPart2
	a2 := fmt.Sprintf("%v", migrations[rewardsMigrationIndex].migrate)
	a1 := fmt.Sprintf("%v", m5Ptr)
	if a1 != a2 {
		fmt.Println("Bad constant in postgres_migrations.go")
		os.Exit(1)
	}
}

// MigrationState is metadata used by the postgres migrations.
type MigrationState struct {
	NextMigration int `json:"next"`

	// NextRound used for m0 to checkpoint progress.
	NextRound int64 `json:"round,omitempty"`

	// NextAssetID used for m3 to checkpoint progress.
	NextAssetID int64 `json:"assetid,omitempty"`

	// NextAccount used for m5 to checkpoint progress.
	NextAccount []byte `json:"nextaccount,omitempty"`

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
	migrationStateJSON := string(json.Encode(state))
	return db.SetMetastate(migrationMetastateKey, migrationStateJSON)
}

func (db *IndexerDb) getMigrationState() (*MigrationState, error) {
	migrationStateJSON, err := db.GetMetastate(migrationMetastateKey)
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
	tx, err := db.db.Begin()
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
			migrationStateJSON := json.Encode(state)
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
		outparams := json.Encode(params)
		_, err = setacfg.Exec(aid, creator[:], outparams)
		if err != nil {
			db.log.WithError(err).Errorf("acfg fix asset update")
			return -1, err
		}
	}
	state.NextAssetID = 0
	state.NextMigration++
	migrationStateJSON := json.Encode(state)
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

// m4RewardsAndDatesPart1 adds the new rewards_total column to the account table.
func m4RewardsAndDatesPart1(db *IndexerDb, state *MigrationState) error {
	// Cache the round in the migration metastate
	round, err := db.GetMaxRound()
	if err != nil {
		db.log.WithError(err).Errorf("%s: problem caching max round: %v", rewardsCreateCloseUpdateErr, err)
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

const rewardsCreateCloseUpdateMessage = "rewards, create_at, close_at migration error"
const rewardsCreateCloseUpdateErr = rewardsCreateCloseUpdateMessage + " error"

func b32ToIndex(c byte) int {
	i := int(c) - 'A'
	if i < 0 {
		//b32 skips 0 and 1.
		return 26 + int(c) - '2'
	}
	return i
}

func addrToPercent(addr string) float64 {
	if len(addr) < 3 {
		return 0.0
	}

	val := b32ToIndex(addr[0])
	val = (val * 32) + b32ToIndex(addr[1])
	val = (val * 32) + b32ToIndex(addr[2])

	return float64(val) / (32 * 32 * 32) * 100
}

// m5RewardsAndDatesPart2 computes the cumulative rewards for each account one at a time.
func m5RewardsAndDatesPart2(db *IndexerDb, state *MigrationState) error {
	db.log.Println("account cumulative rewards migration starting")

	var feeSinkAddr string
	var rewardsAddr string
	{
		accounts, err := db.GetSpecialAccounts()
		if err != nil {
			return fmt.Errorf("unable to get special accounts: %v", err)
		}
		feeSinkAddr = accounts.FeeSink.String()
		rewardsAddr = accounts.RewardsPool.String()
	}

	options := idb.AccountQueryOptions{}
	if len(state.NextAccount) != 0 {
		var address sdk_types.Address
		copy(address[:], state.NextAccount)
		db.log.Println("after " + address.String())
		options.GreaterThanAddress = state.NextAccount[:]
	}

	accountChan := db.GetAccounts(context.Background(), options)

	batchSize := 500
	batchNumber := 1
	// loop through all of the accounts, update them in batches of batchSize.
	accounts := make([]string, 0, batchSize)
	for acct := range accountChan {
		if acct.Error != nil {
			err := fmt.Errorf("%s: problem querying accounts: %v", rewardsCreateCloseUpdateErr, acct.Error)
			db.log.Errorln(err.Error())
			return err
		}

		// Don't update special accounts
		if feeSinkAddr != acct.Account.Address && rewardsAddr != acct.Account.Address {
			accounts = append(accounts, acct.Account.Address)
		}

		if len(accounts) == batchSize {
			db.log.Printf("Cumulative rewards migration processing %.2f%% complete. Batch %d up through account %s",
				addrToPercent(accounts[0]),
				batchNumber,
				accounts[len(accounts)-1])
			err := m5RewardsAndDatesPart2UpdateAccounts(db, state, accounts, false)
			if err != nil {
				return err
			}
			accounts = accounts[:0]
			batchNumber++
		}
	}

	// Get the remainder
	if len(accounts) > 0 {
		db.log.Println("Processing final batch of accounts.")
		err := m5RewardsAndDatesPart2UpdateAccounts(db, state, accounts, true)
		if err != nil {
			return err
		}
		accounts = accounts[:0]
	}

	return nil
}

type createClose struct {
	deleted sql.NullBool
	created sql.NullInt64
	closed  sql.NullInt64
}

// updateClose will only allow the value to be set once.
func updateClose(cc *createClose, value uint64) *createClose {
	if cc == nil {
		return &createClose{
			closed: sql.NullInt64{
				Valid: true,
				Int64: int64(value),
			},
			deleted: sql.NullBool{
				Valid: true,
				Bool: true,
			},
		}
	}

	if !cc.closed.Valid {
		cc.closed.Valid = true
		cc.closed.Int64 = int64(value)
	}

	// Initialize deleted.
	if !cc.deleted.Valid {
		cc.deleted.Valid = true
		cc.deleted.Bool = true
	}

	return cc
}

// updateCreate will update the created round.
func updateCreate(cc *createClose, value uint64) *createClose {
	if cc == nil {
		return &createClose{
			created: sql.NullInt64{
				Valid: true,
				Int64: int64(value),
			},
			deleted: sql.NullBool{
				Valid: true,
				Bool: false,
			},
		}
	}

	cc.created.Valid = true
	cc.created.Int64 = int64(value)

	if !cc.deleted.Valid {
		cc.deleted.Valid = true
		cc.deleted.Bool = false
	}

	return cc
}

func executeForEachCreatable(stmt *sql.Stmt, address []byte, m map[uint64]*createClose) (err error) {
	for index, round := range m {
		_, err = stmt.Exec(address, index, round.created, round.closed, round.deleted)
		if err != nil {
			return
		}
	}
	return
}

type m5AccountData struct {
	cumulativeRewards types.MicroAlgos
	account           createClose
	asset             map[uint64]*createClose
	assetHolding      map[uint64]*createClose
	app               map[uint64]*createClose
	appLocal          map[uint64]*createClose
}

func initM5AccountData() *m5AccountData {
	return &m5AccountData{
		cumulativeRewards: 0,
		account:           createClose{},
		asset:             make(map[uint64]*createClose),
		assetHolding:      make(map[uint64]*createClose),
		app:               make(map[uint64]*createClose),
		appLocal:          make(map[uint64]*createClose),
	}
}

func processAccountTransactionsWithRetry(db *IndexerDb, addressStr string, address types.Address, nextRound uint64, retries int) (results *m5AccountData, err error) {
	for i := 0; i < retries; i++ {
		// Query transactions for the account
		txnrows := db.Transactions(context.Background(), idb.TransactionFilter{
			Address:  address[:],
			MaxRound: nextRound,
		})

		// Process transactions!
		results, err = processAccountTransactions(txnrows, addressStr, address)
		if err != nil {
			db.log.Errorf("%s: (attempt %d) failed to update %s: %v", rewardsCreateCloseUpdateErr, i+1, addressStr, err)
			time.Sleep(10 * time.Second)
		} else {
			return
		}
	}
	return
}

// processAccountTransactions contains all the accounting logic to recompute total rewards and create/close rounds.
func processAccountTransactions(txnrows <-chan idb.TxnRow, addressStr string, address types.Address) (*m5AccountData, error) {
	var err error
	result := initM5AccountData()
	numTxn := 0

	// Loop through transactions
	for txn := range txnrows {
		if txn.Error != nil {
			return nil, fmt.Errorf("%s: processing account %s: found txnrow error:  %v", rewardsCreateCloseUpdateErr, addressStr, txn.Error)
		}
		if len(txn.TxnBytes) == 0 {
			return nil, fmt.Errorf("%s: processing account %s: found empty TxnBytes (rnd %d, intra %d):  %v", rewardsCreateCloseUpdateErr, addressStr, txn.Round, txn.Intra, err)
		}
		numTxn++

		// Transactions are ordered most recent to oldest, so this makes sure created is set to the oldest transaction.
		result.account.created.Valid = true
		result.account.created.Int64 = int64(txn.Round)

		// process transactions one at a time
		var stxn types.SignedTxnWithAD
		err = msgpack.Decode(txn.TxnBytes, &stxn)
		if err != nil {
			return nil, fmt.Errorf("%s: processing account %s: decoding txn (rnd %d, intra %d):  %v", rewardsCreateCloseUpdateErr, addressStr, txn.Round, txn.Intra, err)
		}

		// When the account is closed rewards reset to zero.
		// Because transactions are newest to oldest, stop accumulating once we see a close.
		if !result.account.closed.Valid {
			if accounting.AccountCloseTxn(address, stxn) {
				result.account.closed.Valid = true
				result.account.closed.Int64 = int64(txn.Round)

				if !result.account.deleted.Valid {
					result.account.deleted.Bool = true
					result.account.deleted.Valid = true
				}
			} else {
				if !result.account.deleted.Valid {
					result.account.deleted.Bool = false
					result.account.deleted.Valid = true
				}

				if stxn.Txn.Sender == address {
					result.cumulativeRewards += stxn.ApplyData.SenderRewards
				}

				if stxn.Txn.Receiver == address {
					result.cumulativeRewards += stxn.ApplyData.ReceiverRewards
				}

				if stxn.Txn.CloseRemainderTo == address {
					result.cumulativeRewards += stxn.ApplyData.CloseRewards
				}
			}
		}

		if txn.AssetID == 82 && stxn.Txn.Sender.String() == "CM333ZN3KMASBRIP7N4QIN7AANVK7EJGNUQCNONGVVKURZIU2GG7XJIZ4Q" && stxn.Txn.Type == sdk_types.ApplicationCallTx {
		//if txn.AssetID == 82 && stxn.Txn.Sender.String() ==  addressStr {
			fmt.Println(stxn.Txn.Sender.String())
			fmt.Println("we have arrived")
		}

		if accounting.AssetCreateTxn(stxn) {
			result.asset[txn.AssetID] = updateCreate(result.asset[txn.AssetID], txn.Round)
		}

		if accounting.AssetDestroyTxn(stxn) {
			result.asset[txn.AssetID] = updateClose(result.asset[txn.AssetID], txn.Round)
		}

		if accounting.AssetOptInTxn(stxn) {
			result.assetHolding[txn.AssetID] = updateCreate(result.assetHolding[txn.AssetID], txn.Round)
		}

		if accounting.AssetOptOutTxn(stxn) {
			result.assetHolding[txn.AssetID] = updateClose(result.assetHolding[txn.AssetID], txn.Round)
		}

		if accounting.AppCreateTxn(stxn) {
			result.app[txn.AssetID] = updateCreate(result.app[txn.AssetID], txn.Round)
		}

		if accounting.AppDestroyTxn(stxn) {
			result.app[txn.AssetID] = updateClose(result.app[txn.AssetID], txn.Round)
		}

		if accounting.AppOptInTxn(stxn) {
			result.appLocal[txn.AssetID] = updateCreate(result.appLocal[txn.AssetID], txn.Round)
		}

		if accounting.AppOptOutTxn(stxn) {
			result.appLocal[txn.AssetID] = updateClose(result.appLocal[txn.AssetID], txn.Round)
		}
	}

	// Genesis accounts could have this property
	if numTxn == 0 {
		result.account.created.Valid = true
		result.account.created.Int64 = 0
		result.account.deleted.Valid = true
		result.account.deleted.Bool = false
	}

	return result, nil
}

// m5RewardsAndDatesPart2UpdateAccounts loops through the provided accounts and generates a bunch of updates in a
// single transactional commit. These queries are written so that they can run in the background.
//
// For each account we run several queries:
// 1. updateTotalRewards            - conditionally update the total rewards if the account wasn't closed during iteration.
// 2. setCreateCloseAccount      - set the accounts create/close rounds.
// 3. setCreateCloseAsset        - set the accounts created assets create/close rounds.
// 4. setCreateCloseAssetHolding - (upsert) set the accounts asset holding create/close rounds.
// 5. setCreateCloseApp          - set the accounts created apps create/close rounds.
// 6. setCreateCloseAppLocal     - (upsert) set the accounts local apps create/close rounds.
//
// Note: These queries only work if closed_at was reset before the migration is started. That is true
//       for the initial migration, but if we need to reuse it in the future we'll need to fix the queries
//       or redo the query.
func m5RewardsAndDatesPart2UpdateAccounts(db *IndexerDb, state *MigrationState, accounts []string, finalBatch bool) error {
	// finalAddress is cached for updating the state at the end.
	var finalAddress []byte

	// Process transactions for each account.
	accountData := make(map[types.Address]*m5AccountData, 0)
	for _, addressStr := range accounts {
		address, err := sdk_types.DecodeAddress(addressStr)
		if err != nil {
			return fmt.Errorf("%s: failed to decode address: %s", rewardsCreateCloseUpdateErr, addressStr)
		}
		finalAddress = address[:]

		// Process transactions!
		start := time.Now()
		result, err := processAccountTransactionsWithRetry(db, addressStr, address, uint64(state.NextRound), 3)
		dur := time.Since(start)
		if err != nil {
			return fmt.Errorf("%s: failed to update %s: %v", rewardsCreateCloseUpdateErr, addressStr, err)
		}
		if dur > 5*time.Minute {
			db.log.Warnf("%s: slowness detected, spent %s migrating %s", rewardsCreateCloseUpdateMessage, dur, addressStr)
		}

		accountData[address] = result
	}

	// Open a postgres transaction and submit results for each account.
	tx, err := db.db.Begin()
	if err != nil {
		return fmt.Errorf("%s: tx begin: %v", rewardsCreateCloseUpdateErr, err)
	}
	defer tx.Rollback() // ignored if .Commit() first

	// 1. updateTotalRewards            - conditionally update the total rewards if the account wasn't closed during iteration.
	// $3 is the round after which new blocks will have the closed_at field set.
	// We only set rewards_total when closed_at was set before that round.
	updateTotalRewards, err := tx.Prepare(`UPDATE account SET rewards_total = coalesce(rewards_total, 0) + $2 WHERE addr = $1 AND coalesce(closed_at, 0) < $3`)
	if err != nil {
		return fmt.Errorf("%s: set rewards prepare: %v", rewardsCreateCloseUpdateErr, err)
	}
	defer updateTotalRewards.Close()

	// 2. setCreateCloseAccount      - set the accounts create/close rounds.
	// We always set the created_at field because it will never change.
	// closed_at may already be set by the time the migration runs, or it might need to be cleared out.
	setCreateCloseAccount, err := tx.Prepare(`UPDATE account SET created_at = $2, closed_at = coalesce(closed_at, $3), deleted = coalesce(deleted, $4) WHERE addr = $1`)
	if err != nil {
		return fmt.Errorf("%s: set create close prepare: %v", rewardsCreateCloseUpdateErr, err)
	}
	defer setCreateCloseAccount.Close()

	// 3. setCreateCloseAsset        - set the accounts created assets create/close rounds.
	setCreateCloseAsset, err := tx.Prepare(`UPDATE asset SET created_at = $3, closed_at = coalesce(closed_at, $4), deleted = coalesce(deleted, $5) WHERE creator_addr = $1 AND index=$2`)
	if err != nil {
		return fmt.Errorf("%s: set create close asset prepare: %v", rewardsCreateCloseUpdateErr, err)
	}
	defer setCreateCloseAsset.Close()

	// 4. setCreateCloseAssetHolding - (upsert) set the accounts asset holding create/close rounds.
	setCreateCloseAssetHolding, err := tx.Prepare(`INSERT INTO account_asset(addr, assetid, amount, frozen, created_at, closed_at, deleted) VALUES ($1, $2, 0, false, $3, $4, $5) ON CONFLICT (addr, assetid) DO UPDATE SET created_at = EXCLUDED.created_at, closed_at = coalesce(account_asset.closed_at, EXCLUDED.closed_at), deleted = coalesce(account_asset.deleted, EXCLUDED.deleted)`)
	if err != nil {
		return fmt.Errorf("%s: set create close asset holding prepare: %v", rewardsCreateCloseUpdateErr, err)
	}
	defer setCreateCloseAssetHolding.Close()

	// 5. setCreateCloseApp          - set the accounts created apps create/close rounds.
	setCreateCloseApp, err := tx.Prepare(`UPDATE app SET created_at = $3, closed_at = coalesce(closed_at, $4), deleted = coalesce(deleted, $5) WHERE creator = $1 AND index=$2`)
	if err != nil {
		return fmt.Errorf("%s: set create close app prepare: %v", rewardsCreateCloseUpdateErr, err)
	}
	defer setCreateCloseApp.Close()

	// 6. setCreateCloseAppLocal     - (upsert) set the accounts local apps create/close rounds.
	setCreateCloseAppLocal, err := tx.Prepare(`INSERT INTO account_app (addr, app, created_at, closed_at, deleted) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (addr, app) DO UPDATE SET created_at = EXCLUDED.created_at, closed_at = coalesce(account_app.closed_at, EXCLUDED.closed_at), deleted = coalesce(account_app.deleted, EXCLUDED.deleted)`)
	if err != nil {
		return fmt.Errorf("%s: set create close app local prepare: %v", rewardsCreateCloseUpdateErr, err)
	}
	defer setCreateCloseAppLocal.Close()

	// loop through all of the accounts.
	for address, result := range accountData {
		addressStr := address.String()

		// 1. updateTotalRewards            - conditionally update the total rewards if the account wasn't closed during iteration.
		_, err = updateTotalRewards.Exec(address[:], result.cumulativeRewards, state.NextRound)
		if err != nil {
			return fmt.Errorf("%s: failed to update %s with rewards %d: %v", rewardsCreateCloseUpdateErr, addressStr, result.cumulativeRewards, err)
		}

		// 2. setCreateCloseAccount      - set the accounts create/close rounds.
		_, err = setCreateCloseAccount.Exec(address[:], result.account.created, result.account.closed, result.account.deleted)
		if err != nil {
			return fmt.Errorf("%s: failed to update %s with create/close: %v", rewardsCreateCloseUpdateErr, addressStr, err)
		}

		// 3. setCreateCloseAsset        - set the accounts created assets create/close rounds.
		err = executeForEachCreatable(setCreateCloseAsset, address[:], result.asset)
		if err != nil {
			return fmt.Errorf("%s: failed to update %s with asset create/close: %v", rewardsCreateCloseUpdateErr, addressStr, err)
		}

		// 4. setCreateCloseAssetHolding - (upsert) set the accounts asset holding create/close rounds.
		err = executeForEachCreatable(setCreateCloseAssetHolding, address[:], result.assetHolding)
		if err != nil {
			return fmt.Errorf("%s: failed to update %s with asset holding create/close: %v", rewardsCreateCloseUpdateErr, addressStr, err)
		}

		// 5. setCreateCloseApp          - set the accounts created apps create/close rounds.
		err = executeForEachCreatable(setCreateCloseApp, address[:], result.app)
		if err != nil {
			return fmt.Errorf("%s: failed to update %s with app create/close: %v", rewardsCreateCloseUpdateErr, addressStr, err)
		}

		// 6. setCreateCloseAppLocal     - (upsert) set the accounts local apps create/close rounds.
		err = executeForEachCreatable(setCreateCloseAppLocal, address[:], result.appLocal)
		if err != nil {
			return fmt.Errorf("%s: failed to update %s with app local create/close: %v", rewardsCreateCloseUpdateErr, addressStr, err)
		}
	}

	// Update checkpoint
	if finalBatch {
		state.NextAccount = nil
	} else {
		state.NextAccount = finalAddress[:]
	}
	migrationStateJSON := json.Encode(state)
	_, err = tx.Exec(setMetastateUpsert, migrationMetastateKey, migrationStateJSON)
	if err != nil {
		return fmt.Errorf("%s: failed to update migration checkpoint: %v", rewardsCreateCloseUpdateErr, err)
	}

	// Commit transactions
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("%s: failed to commit changes: %v", rewardsCreateCloseUpdateErr, err)
	}

	return nil
}

// sqlMigration executes a sql statements as the entire migration.
func sqlMigration(db *IndexerDb, state *MigrationState, sqlLines []string) error {
	thisMigration := state.NextMigration
	tx, err := db.db.Begin()
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
	migrationStateJSON := json.Encode(state)
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
	prevRound := state.NextRound - 1
	txns := db.YieldTxns(context.Background(), prevRound)
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
	migrationStateJSON := string(json.Encode(state))
	err = db.SetMetastate(migrationMetastateKey, migrationStateJSON)
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
	tx, err := db.db.Begin()
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
	migrationStateJSON := json.Encode(txstate)
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
	return headers, nil
}

// Record round at which behavior changed for encoding txn.txn JSON.
// A future migration should go back and apply new encoding to prior txn rows then delete this row in metastate.
func m6MarkTxnJSONSplit(db *IndexerDb, state *MigrationState) error {
	sqlLines := []string{
		`INSERT INTO metastate (k,v) SELECT 'm6MarkTxnJSONSplit', m.v FROM metastate m WHERE m.k = 'state'`,
	}
	return sqlMigration(db, state, sqlLines)
}
