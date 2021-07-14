// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package postgres

// import text to constants setup_postgres_sql reset_sql
//go:generate go run ../../cmd/texttosource/main.go postgres setup_postgres.sql reset.sql

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	// Load the postgres sql.DB implementation
	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/migration"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/types"
)

type importState struct {
	// DEPRECATED. Last accounted round.
	AccountRound *int64 `codec:"account_round"`
	// Next round to account.
	NextRoundToAccount *uint64 `codec:"next_account_round"`
}

const stateMetastateKey = "state"
const migrationMetastateKey = "migration"
const specialAccountsMetastateKey = "accounts"

var serializable = sql.TxOptions{Isolation: sql.LevelSerializable} // be a real ACID database
var readonlyRepeatableRead = sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}

// OpenPostgres is available for creating test instances of postgres.IndexerDb
func OpenPostgres(connection string, opts idb.IndexerDbOptions, log *log.Logger) (pdb *IndexerDb, err error) {
	db, err := sql.Open("postgres", connection)

	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %v", err)
	}

	if strings.Contains(connection, "readonly") {
		opts.ReadOnly = true
	}

	return openPostgres(db, opts, log)
}

// Allow tests to inject a DB
func openPostgres(db *sql.DB, opts idb.IndexerDbOptions, logger *log.Logger) (pdb *IndexerDb, err error) {
	pdb = &IndexerDb{
		readonly: opts.ReadOnly,
		log:      logger,
		db:       db,
	}

	if pdb.log == nil {
		pdb.log = log.New()
		pdb.log.SetFormatter(&log.JSONFormatter{})
		pdb.log.SetOutput(os.Stdout)
		pdb.log.SetLevel(log.TraceLevel)
	}

	// e.g. a user named "readonly" is in the connection string
	if !opts.ReadOnly {
		err = pdb.init(opts)
		if err != nil {
			return nil, fmt.Errorf("initializing postgres: %v", err)
		}
	}
	return
}

// IndexerDb is an idb.IndexerDB implementation
type IndexerDb struct {
	readonly bool
	log      *log.Logger

	db *sql.DB

	// state for StartBlock/AddTransaction/CommitBlock
	txrows  [][]interface{}
	txprows [][]interface{}

	migration *migration.Migration

	accountingLock sync.Mutex
}

// A helper function that retries the function `f` in case the database transaction in it
// fails due to a serialization error. `f` is provided context `ctx` and a transaction created
// using this context and `opts`. `f` takes ownership of the transaction and must either call
// sql.Tx.Rollback() or sql.Tx.Commit(). In the second case, `f` must return an error which
// contains the error returned by sql.Tx.Commit(). The easiest way is to just return the result
// of sql.Tx.Commit().
func (db *IndexerDb) txWithRetry(ctx context.Context, opts sql.TxOptions, f func(context.Context, *sql.Tx) error) error {
	count := 0
	for {
		tx, err := db.db.BeginTx(ctx, &opts)
		if err != nil {
			return err
		}

		err = f(ctx, tx)

		// If not serialization error.
		var pqerr *pq.Error
		if !errors.As(err, &pqerr) || (pqerr.Code != "40001") {
			if count > 0 {
				db.log.Printf("transaction was retried %d times", count)
			}
			return err
		}

		count++
		db.log.Printf("retrying transaction, count: %d", count)
	}
}

func (db *IndexerDb) isSetup() (bool, error) {
	query := `SELECT 0 FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = 'metastate'`
	row := db.db.QueryRow(query)

	var tmp int
	err := row.Scan(&tmp)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("isSetup() err: %w", err)
	}
	return true, nil
}

func (db *IndexerDb) init(opts idb.IndexerDbOptions) error {
	setup, err := db.isSetup()
	if err != nil {
		return fmt.Errorf("init() err: %w", err)
	}

	if !setup {
		// new database, run setup
		_, err = db.db.Exec(setup_postgres_sql)
		if err != nil {
			return fmt.Errorf("unable to setup postgres: %v", err)
		}

		err = db.markMigrationsAsDone()
		if err != nil {
			return fmt.Errorf("unable to confirm migration: %v", err)
		}

		return nil
	}

	// see postgres_migrations.go
	return db.runAvailableMigrations()
}

// Reset is part of idb.IndexerDB
func (db *IndexerDb) Reset() (err error) {
	// new database, run setup
	_, err = db.db.Exec(reset_sql)
	if err != nil {
		return fmt.Errorf("db reset failed, %v", err)
	}
	db.log.Debugf("reset.sql done")
	return
}

// StartBlock is part of idb.IndexerDB
func (db *IndexerDb) StartBlock() (err error) {
	db.txrows = make([][]interface{}, 0, 6000)
	db.txprows = make([][]interface{}, 0, 10000)
	return nil
}

// AddTransaction is part of idb.IndexerDB
func (db *IndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error {
	txnbytes := msgpack.Encode(txn)
	jsonbytes := encoding.EncodeSignedTxnWithAD(txn)
	txid := crypto.TransactionIDString(txn.Txn)
	tx := []interface{}{round, intra, txtypeenum, assetid, txid[:], txnbytes, string(jsonbytes)}
	db.txrows = append(db.txrows, tx)
	for _, paddr := range participation {
		txp := []interface{}{paddr, round, intra}
		db.txprows = append(db.txprows, txp)
	}
	return nil
}

func (db *IndexerDb) commitBlock(tx *sql.Tx, round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
	defer tx.Rollback() // ignored if already committed

	addtx, err := tx.Prepare(`COPY txn (round, intra, typeenum, asset, txid, txnbytes, txn) FROM STDIN`)
	if err != nil {
		return fmt.Errorf("COPY txn %v", err)
	}
	defer addtx.Close()
	for _, txr := range db.txrows {
		_, err = addtx.Exec(txr...)
		if err != nil {
			return fmt.Errorf("COPY txn Exec %v", err)
		}
	}
	_, err = addtx.Exec()
	if err != nil {
		db.log.Errorf("CommitBlock failed: %v (%#v)", err, err)
		for _, txr := range db.txrows {
			ntxr := make([]interface{}, len(txr))
			for i, v := range txr {
				switch tv := v.(type) {
				case []byte:
					if utf8.Valid(tv) {
						ntxr[i] = string(tv)
					} else {
						ntxr[i] = v
					}
				default:
					ntxr[i] = v
				}
			}
			db.log.Errorf("txr %#v", ntxr)
		}
		return fmt.Errorf("COPY txn Exec() %v", err)
	}
	err = addtx.Close()
	if err != nil {
		return fmt.Errorf("COPY txn Close %v", err)
	}

	addtxpart, err := tx.Prepare(`COPY txn_participation (addr, round, intra) FROM STDIN`)
	if err != nil {
		return fmt.Errorf("COPY txn part %v", err)
	}
	defer addtxpart.Close()
	for i, txpr := range db.txprows {
		_, err = addtxpart.Exec(txpr...)
		if err != nil {
			//return err
			for _, er := range db.txprows[:i+1] {
				db.log.Printf("%s %d %d", encoding.Base64(er[0].([]byte)), er[1], er[2])
			}
			return fmt.Errorf("%v, around txp row %#v", err, txpr)
		}
	}

	_, err = addtxpart.Exec()
	if err != nil {
		return fmt.Errorf("during addtxp empty exec %v", err)
	}
	err = addtxpart.Close()
	if err != nil {
		return fmt.Errorf("during addtxp close %v", err)
	}

	var blockHeader types.BlockHeader
	err = msgpack.Decode(headerbytes, &blockHeader)
	if err != nil {
		return fmt.Errorf("decode header %v", err)
	}
	headerjson := encoding.EncodeJSON(blockHeader)
	_, err = tx.Exec(`INSERT INTO block_header (round, realtime, rewardslevel, header) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, round, time.Unix(timestamp, 0).UTC(), rewardslevel, headerjson)
	if err != nil {
		return fmt.Errorf("put block_header %v    %#v", err, err)
	}

	return tx.Commit()
}

// CommitBlock is part of idb.IndexerDB
func (db *IndexerDb) CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
	f := func(ctx context.Context, tx *sql.Tx) error {
		return db.commitBlock(tx, round, timestamp, rewardslevel, headerbytes)
	}
	err := db.txWithRetry(context.Background(), serializable, f)

	db.txrows = nil
	db.txprows = nil

	if err != nil {
		return fmt.Errorf("CommitBlock(): %v", err)
	}
	return nil
}

// GetDefaultFrozen get {assetid:default frozen, ...} for all assets, needed by accounting.
// Because Go map[]bool returns false by default, we actually return only a map of the true elements.
func (db *IndexerDb) GetDefaultFrozen() (defaultFrozen map[uint64]bool, err error) {
	rows, err := db.db.Query(`SELECT index FROM asset WHERE (params ->> 'df')::boolean = true`)
	if err != nil {
		return
	}
	defaultFrozen = make(map[uint64]bool)
	for rows.Next() {
		var assetid uint64
		err = rows.Scan(&assetid)
		if err != nil {
			return
		}
		defaultFrozen[assetid] = true
	}
	return
}

// LoadGenesis is part of idb.IndexerDB
func (db *IndexerDb) LoadGenesis(genesis types.Genesis) (err error) {
	tx, err := db.db.BeginTx(context.Background(), &serializable)
	if err != nil {
		return
	}
	defer tx.Rollback() // ignored if .Commit() first

	setAccount, err := tx.Prepare(`INSERT INTO account (addr, microalgos, rewardsbase, account_data, rewards_total, created_at, deleted) VALUES ($1, $2, 0, $3, $4, 0, false)`)
	if err != nil {
		return
	}
	defer setAccount.Close()

	total := uint64(0)
	for ai, alloc := range genesis.Allocation {
		addr, err := sdk_types.DecodeAddress(alloc.Address)
		if err != nil {
			return nil
		}
		if len(alloc.State.AssetParams) > 0 || len(alloc.State.Assets) > 0 {
			return fmt.Errorf("genesis account[%d] has unhandled asset", ai)
		}
		_, err = setAccount.Exec(addr[:], alloc.State.MicroAlgos, encoding.EncodeJSON(alloc.State), 0)
		total += uint64(alloc.State.MicroAlgos)
		if err != nil {
			return fmt.Errorf("error setting genesis account[%d], %v", ai, err)
		}
	}

	nextRound := uint64(0)
	importstate := importState{
		NextRoundToAccount: &nextRound,
	}
	err = db.setImportState(nil, importstate)
	if err != nil {
		return
	}

	err = tx.Commit()
	db.log.Printf("genesis %d accounts %d microalgos, err=%v", len(genesis.Allocation), total, err)
	return err
}

// Returns `idb.ErrorNotInitialized` if uninitialized.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getMetastate(tx *sql.Tx, key string) (string, error) {
	query := `SELECT v FROM metastate WHERE k = $1`

	var row *sql.Row
	if tx == nil {
		row = db.db.QueryRow(query, key)
	} else {
		row = tx.QueryRow(query, key)
	}

	var value string
	err := row.Scan(&value)
	if err == sql.ErrNoRows {
		return "", idb.ErrorNotInitialized
	}
	if err != nil {
		return "", fmt.Errorf("getMetastate() err: %w", err)
	}

	return value, nil
}

const setMetastateUpsert = `INSERT INTO metastate (k, v) VALUES ($1, $2) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`

// If `tx` is nil, use a normal query.
func (db *IndexerDb) setMetastate(tx *sql.Tx, key, jsonStrValue string) (err error) {
	if tx == nil {
		_, err = db.db.Exec(setMetastateUpsert, key, jsonStrValue)
	} else {
		_, err = tx.Exec(setMetastateUpsert, key, jsonStrValue)
	}
	return
}

// Returns idb.ErrorNotInitialized if uninitialized.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getImportState(tx *sql.Tx) (importState, error) {
	importStateJSON, err := db.getMetastate(tx, stateMetastateKey)
	if err == idb.ErrorNotInitialized {
		return importState{}, idb.ErrorNotInitialized
	}
	if err != nil {
		return importState{}, fmt.Errorf("unable to get import state err: %w", err)
	}

	if importStateJSON == "" {
		return importState{}, idb.ErrorNotInitialized
	}

	var state importState
	err = encoding.DecodeJSON([]byte(importStateJSON), &state)
	if err != nil {
		return importState{},
			fmt.Errorf("unable to parse import state v: \"%s\" err: %w", importStateJSON, err)
	}

	return state, nil
}

// If `tx` is nil, use a normal query.
func (db *IndexerDb) setImportState(tx *sql.Tx, state importState) error {
	return db.setMetastate(tx, stateMetastateKey, string(encoding.EncodeJSON(state)))
}

// Returns ErrorNotInitialized if genesis is not loaded.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getNextRoundToAccount(tx *sql.Tx) (uint64, error) {
	state, err := db.getImportState(tx)
	if err == idb.ErrorNotInitialized {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("getNextRoundToAccount() err: %w", err)
	}

	if state.NextRoundToAccount == nil {
		return 0, idb.ErrorNotInitialized
	}
	return *state.NextRoundToAccount, nil
}

// GetNextRoundToAccount is part of idb.IndexerDB
// Returns ErrorNotInitialized if genesis is not loaded.
func (db *IndexerDb) GetNextRoundToAccount() (uint64, error) {
	return db.getNextRoundToAccount(nil)
}

// Returns ErrorNotInitialized if genesis is not loaded.
// If `tx` is nil, use a normal query.
func (db *IndexerDb) getMaxRoundAccounted(tx *sql.Tx) (uint64, error) {
	round, err := db.getNextRoundToAccount(tx)
	if err != nil {
		return 0, err
	}

	if round > 0 {
		round--
	}
	return round, nil
}

// GetNextRoundToLoad is part of idb.IndexerDB
func (db *IndexerDb) GetNextRoundToLoad() (uint64, error) {
	row := db.db.QueryRow(`SELECT max(round) FROM block_header`)

	var nullableRound sql.NullInt64
	err := row.Scan(&nullableRound)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	if !nullableRound.Valid {
		return 0, nil
	}
	return uint64(nullableRound.Int64 + 1), nil
}

// Break the read query so that PostgreSQL doesn't get bogged down
// tracking transactional changes to tables.
const txnQueryBatchSize = 20000

var yieldTxnQuery string

func init() {
	yieldTxnQuery = fmt.Sprintf(`SELECT t.round, t.intra, t.txnbytes, t.extra, t.asset, b.realtime FROM txn t JOIN block_header b ON t.round = b.round WHERE t.round > $1 ORDER BY round, intra LIMIT %d`, txnQueryBatchSize)
}

func (db *IndexerDb) yieldTxnsThread(ctx context.Context, rows *sql.Rows, results chan<- idb.TxnRow) {
	defer rows.Close()

	keepGoing := true
	for keepGoing {
		keepGoing = false
		rounds := make([]uint64, txnQueryBatchSize)
		intras := make([]int, txnQueryBatchSize)
		txnbytess := make([][]byte, txnQueryBatchSize)
		extrajsons := make([][]byte, txnQueryBatchSize)
		creatableids := make([]int64, txnQueryBatchSize)
		roundtimes := make([]time.Time, txnQueryBatchSize)
		pos := 0
		// read from db
		for rows.Next() {
			var round uint64
			var intra int
			var txnbytes []byte
			var extrajson []byte
			var creatableid int64
			var roundtime time.Time
			err := rows.Scan(&round, &intra, &txnbytes, &extrajson, &creatableid, &roundtime)
			if err != nil {
				var row idb.TxnRow
				row.Error = err
				results <- row
				return
			}

			rounds[pos] = round
			intras[pos] = intra
			txnbytess[pos] = txnbytes
			extrajsons[pos] = extrajson
			creatableids[pos] = creatableid
			roundtimes[pos] = roundtime
			pos++

			keepGoing = true
		}
		if err := rows.Err(); err != nil {
			var row idb.TxnRow
			row.Error = err
			results <- row
			return
		}
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
			var row idb.TxnRow
			row.Round = rounds[i]
			row.RoundTime = roundtimes[i]
			row.Intra = intras[i]
			row.TxnBytes = txnbytess[i]
			row.AssetID = uint64(creatableids[i])
			if len(extrajsons[i]) > 0 {
				err := encoding.DecodeJSON(extrajsons[i], &row.Extra)
				if err != nil {
					row.Error = fmt.Errorf("%d:%d decode txn extra, %v", row.Round, row.Intra, err)
					results <- row
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case results <- row:
			}
		}
		if keepGoing {
			var err error
			prevRound := rounds[pos-1]
			rows, err = db.db.QueryContext(ctx, yieldTxnQuery, prevRound)
			if err != nil {
				results <- idb.TxnRow{Error: err}
				break
			}
		}
	}
}

// YieldTxns is part of idb.IndexerDB
func (db *IndexerDb) YieldTxns(ctx context.Context, firstRound uint64) <-chan idb.TxnRow {
	results := make(chan idb.TxnRow, 1)
	rows, err := db.db.QueryContext(ctx, yieldTxnQuery, int64(firstRound)-1)
	if err != nil {
		results <- idb.TxnRow{Error: err}
		close(results)
		return results
	}
	go func() {
		db.yieldTxnsThread(ctx, rows, results)
		close(results)
	}()
	return results
}

// TODO: maybe make a flag to set this, but in case of bug set this to
// debug any asset that isn't working out right:
var debugAsset uint64 = 0

func obs(x interface{}) string {
	return string(encoding.EncodeJSON(x))
}

// StateSchema like go-algorand data/basics/teal.go
type StateSchema struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	NumUint      uint64 `codec:"nui"`
	NumByteSlice uint64 `codec:"nbs"`
}

func (ss *StateSchema) fromBlock(x sdk_types.StateSchema) {
	if x.NumUint != 0 || x.NumByteSlice != 0 {
		ss.NumUint = x.NumUint
		ss.NumByteSlice = x.NumByteSlice
	}
}

// TealType is a teal type
type TealType uint64

// TealValue is a TealValue
type TealValue struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	Type  TealType `codec:"tt"`
	Bytes []byte   `codec:"tb"`
	Uint  uint64   `codec:"ui"`
}

func (tv *TealValue) setFromValueDelta(vd types.ValueDelta) error {
	switch vd.Action {
	case types.SetUintAction:
		tv.Type = TealUintType
		tv.Uint = vd.Uint
	case types.SetBytesAction:
		tv.Type = TealBytesType
		tv.Bytes = vd.Bytes
	default:
		return fmt.Errorf("could not apply ValueDelta %v", vd)
	}
	return nil
}

const (
	// TealBytesType represents the type of a byte slice in a TEAL program
	TealBytesType TealType = 1

	// TealUintType represents the type of a uint in a TEAL program
	TealUintType TealType = 2
)

func (tv TealValue) toModel() models.TealValue {
	switch tv.Type {
	case TealUintType:
		return models.TealValue{Uint: tv.Uint, Type: uint64(tv.Type)}
	case TealBytesType:
		return models.TealValue{Bytes: encoding.Base64(tv.Bytes), Type: uint64(tv.Type)}
	}
	return models.TealValue{}
}

// TODO: These should probably all be moved to the types package.

// KeyTealValue the KeyTealValue struct.
type KeyTealValue struct {
	Key []byte    `codec:"k"`
	Tv  TealValue `codec:"v"`
}

// TealKeyValue the teal key value struct
type TealKeyValue struct {
	They []KeyTealValue
}

func (tkv TealKeyValue) toModel() *models.TealKeyValueStore {
	if len(tkv.They) == 0 {
		return nil
	}
	var out models.TealKeyValueStore = make([]models.TealKeyValue, len(tkv.They))
	pos := 0
	for _, ktv := range tkv.They {
		out[pos].Key = encoding.Base64(ktv.Key)
		out[pos].Value = ktv.Tv.toModel()
		pos++
	}
	return &out
}
func (tkv TealKeyValue) get(key []byte) (TealValue, bool) {
	for _, ktv := range tkv.They {
		if bytes.Equal(ktv.Key, key) {
			return ktv.Tv, true
		}
	}
	return TealValue{}, false
}
func (tkv *TealKeyValue) put(key []byte, tv TealValue) {
	for i, ktv := range tkv.They {
		if bytes.Equal(ktv.Key, key) {
			tkv.They[i].Tv = tv
			return
		}
	}
	tkv.They = append(tkv.They, KeyTealValue{Key: key, Tv: tv})
}
func (tkv *TealKeyValue) delete(key []byte) {
	for i, ktv := range tkv.They {
		if bytes.Equal(ktv.Key, key) {
			last := len(tkv.They) - 1
			if last == 0 {
				tkv.They = nil
				return
			}
			if i < last {
				tkv.They[i] = tkv.They[last]
				tkv.They = tkv.They[:last]
				return
			}
		}
	}
}

// MarshalJSON wraps encoding.EncodeJSON
func (tkv TealKeyValue) MarshalJSON() ([]byte, error) {
	return encoding.EncodeJSON(tkv.They), nil
}

// UnmarshalJSON wraps encoding.DecodeJSON
func (tkv *TealKeyValue) UnmarshalJSON(data []byte) error {
	return encoding.DecodeJSON(data, &tkv.They)
}

// AppParams like go-algorand data/basics/userBalance.go AppParams{}
type AppParams struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	ApprovalProgram   []byte      `codec:"approv"`
	ClearStateProgram []byte      `codec:"clearp"`
	LocalStateSchema  StateSchema `codec:"lsch"`
	GlobalStateSchema StateSchema `codec:"gsch"`
	ExtraProgramPages uint32      `codec:"epp"`

	GlobalState TealKeyValue `codec:"gs,allocbound=-"`
}

// AppLocalState like go-algorand data/basics/userBalance.go AppLocalState{}
type AppLocalState struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	Schema   StateSchema  `codec:"hsch"`
	KeyValue TealKeyValue `codec:"tkv"`
}

type inmemAppLocalState struct {
	AppLocalState

	address  []byte
	appIndex int64
}

// Build a reverse delta and apply the delta to the TealKeyValue state.
func applyKeyValueDelta(state *TealKeyValue, key []byte, vd types.ValueDelta, reverseDelta *idb.AppReverseDelta) (err error) {
	oldValue, ok := state.get(key)
	if ok {
		switch oldValue.Type {
		case TealUintType:
			reverseDelta.SetDelta(key, types.ValueDelta{Action: types.SetUintAction, Uint: oldValue.Uint})
		case TealBytesType:
			reverseDelta.SetDelta(key, types.ValueDelta{Action: types.SetBytesAction, Bytes: oldValue.Bytes})
		default:
			return fmt.Errorf("old value key=%s ov.T=%T ov=%v", key, oldValue, oldValue)
		}
	} else {
		reverseDelta.SetDelta(key, types.ValueDelta{Action: types.DeleteAction})
	}
	newValue := oldValue
	switch vd.Action {
	case types.SetUintAction, types.SetBytesAction:
		newValue.setFromValueDelta(vd)
		state.put(key, newValue)
	case types.DeleteAction:
		state.delete(key)
	default:
		return fmt.Errorf("unknown action action=%d, delta=%v", vd.Action, vd)
	}
	return nil
}

func (db *IndexerDb) getDirtyAppLocalState(addr []byte, appIndex int64, dirty []inmemAppLocalState, getq *sql.Stmt) (localstate inmemAppLocalState, err error) {
	for _, v := range dirty {
		if v.appIndex == appIndex && bytes.Equal(addr, v.address) {
			return v, nil
		}
	}
	var localstatejson []byte
	row := getq.QueryRow(addr, appIndex)
	err = row.Scan(&localstatejson)
	if err == sql.ErrNoRows {
		// ok, no prior data, empty state
		err = nil
	} else if err != nil {
		err = fmt.Errorf("app local get, %v", err)
		return
	} else if len(localstatejson) > 0 {
		err = encoding.DecodeJSON(localstatejson, &localstate.AppLocalState)
		if err != nil {
			err = fmt.Errorf("app local get bad json, %v", err)
		}
	}
	localstate.address = addr
	localstate.appIndex = appIndex
	return
}

// overwrite or append
func setDirtyAppLocalState(dirty []inmemAppLocalState, x inmemAppLocalState) []inmemAppLocalState {
	for i, v := range dirty {
		if v.appIndex == x.appIndex && bytes.Equal(v.address, x.address) {
			dirty[i] = x
			return dirty
		}
	}
	return append(dirty, x)
}

func (db *IndexerDb) commitRoundAccounting(tx *sql.Tx, updates idb.RoundUpdates, round uint64, blockHeader *types.BlockHeader) (err error) {
	defer tx.Rollback() // ignored if .Commit() first

	db.accountingLock.Lock()
	defer db.accountingLock.Unlock()

	any := false
	if len(updates.AlgoUpdates) > 0 {
		any = true
		// account_data json is only used on account creation, otherwise the account data jsonb field is updated from the delta
		upsertalgo, err := tx.Prepare(`INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, created_at, deleted) VALUES ($1, $2, $3, $4, $5, false) ON CONFLICT (addr) DO UPDATE SET microalgos = account.microalgos + EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase, rewards_total = account.rewards_total + EXCLUDED.rewards_total, deleted = false`)
		if err != nil {
			return fmt.Errorf("prepare update algo, %v", err)
		}
		defer upsertalgo.Close()

		// If the account is closing the cumulative rewards field and closed_at needs to be set directly
		// Using an upsert because it's technically allowed to create and close an account in the same round.
		closealgo, err := tx.Prepare(`INSERT INTO account (addr, microalgos, rewardsbase, rewards_total, created_at, closed_at, deleted) VALUES ($1, $2, $3, $4, $5, $6, true) ON CONFLICT (addr) DO UPDATE SET microalgos = account.microalgos + EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase, rewards_total = EXCLUDED.rewards_total, closed_at = EXCLUDED.closed_at, deleted = true, account_data = NULL`)
		if err != nil {
			return fmt.Errorf("prepare reset algo, %v", err)
		}
		defer closealgo.Close()

		for addr, delta := range updates.AlgoUpdates {
			if !delta.Closed {
				_, err = upsertalgo.Exec(addr[:], delta.Balance, blockHeader.RewardsLevel, delta.Rewards, round)
				if err != nil {
					return fmt.Errorf("update algo, %v", err)
				}
			} else {
				_, err = closealgo.Exec(addr[:], delta.Balance, blockHeader.RewardsLevel, delta.Rewards, round, round)
				if err != nil {
					return fmt.Errorf("close algo, %v", err)
				}
			}
		}
	}
	if len(updates.AccountTypes) > 0 {
		any = true
		setat, err := tx.Prepare(`UPDATE account SET keytype = $1 WHERE addr = $2`)
		if err != nil {
			return fmt.Errorf("prepare update account type, %v", err)
		}
		defer setat.Close()
		for addr, kt := range updates.AccountTypes {
			_, err = setat.Exec(kt, addr[:])
			if err != nil {
				return fmt.Errorf("update account type, %v", err)
			}
		}
	}
	if len(updates.AccountDataUpdates) > 0 {
		any = true

		setad, err := tx.Prepare(`UPDATE account SET account_data = coalesce(account_data, '{}'::jsonb) || ($1)::jsonb WHERE addr = $2`)
		if err != nil {
			return fmt.Errorf("prepare keyreg, %v", err)
		}
		defer setad.Close()

		delad, err := tx.Prepare(`UPDATE account SET account_data = coalesce(account_data, '{}'::jsonb) - $1 WHERE addr = $2`)
		if err != nil {
			return fmt.Errorf("prepare keyreg, %v", err)
		}
		defer delad.Close()

		for addr, acctDataUpdates := range updates.AccountDataUpdates {
			set := make(map[string]interface{})

			for key, acctDataUpdate := range acctDataUpdates {
				if acctDataUpdate.Delete {
					_, err = delad.Exec(key, addr[:])
					if err != nil {
						return fmt.Errorf("delete key in account data, %v", err)
					}
				} else {
					set[key] = acctDataUpdate.Value
				}
			}

			jb := encoding.EncodeJSON(set)
			_, err = setad.Exec(jb, addr[:])
			if err != nil {
				return fmt.Errorf("update account data, %v", err)
			}
		}
	}
	if len(updates.AssetUpdates) > 0 && len(updates.AssetUpdates[0]) > 0 {
		any = true

		////////////////
		// Asset Xfer //
		////////////////
		// Create new account_asset, initialize a previously destroyed asset, or apply the balance delta.
		// Setting frozen is complicated for the no-op optin case. It should only be set to default-frozen when the
		// holding is deleted, otherwise it should be left as the original value.
		seta, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen, created_at, deleted) VALUES ($1, $2, $3, $4, $5, false) ON CONFLICT (addr, assetid) DO UPDATE SET amount = account_asset.amount + EXCLUDED.amount, frozen = (EXCLUDED.frozen AND coalesce(account_asset.deleted, false)) OR (account_asset.frozen AND NOT coalesce(account_asset.deleted, false)), deleted = false`)
		if err != nil {
			return fmt.Errorf("prepare set account_asset, %v", err)
		}
		defer seta.Close()

		/////////////////
		// Asset Close //
		/////////////////
		// On asset opt-out attach some extra "apply data" metadata to allow rewinding the asset close if requested.
		acc, err := tx.Prepare(`WITH aaamount AS (SELECT ($1)::bigint as round, ($2)::bigint as intra, x.amount FROM account_asset x WHERE x.addr = $3 AND x.assetid = $4)
UPDATE txn ut SET extra = jsonb_set(coalesce(ut.extra, '{}'::jsonb), '{aca}', to_jsonb(aaamount.amount)) FROM aaamount WHERE ut.round = aaamount.round AND ut.intra = aaamount.intra`)
		if err != nil {
			return fmt.Errorf("prepare asset close0, %v", err)
		}
		defer acc.Close()
		// On asset opt-out update the CloseTo account_asset
		acs, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen, created_at, deleted)
SELECT $1, $2, x.amount, $3, $6, false FROM account_asset x WHERE x.addr = $4 AND x.assetid = $5 AND x.amount <> 0
ON CONFLICT (addr, assetid) DO UPDATE SET amount = account_asset.amount + EXCLUDED.amount, deleted = false`)
		if err != nil {
			return fmt.Errorf("prepare asset close1, %v", err)
		}
		defer acs.Close()
		// On asset opt-out mark the account_asset as closed with zero balance.
		acd, err := tx.Prepare(`UPDATE account_asset SET amount = 0, closed_at = $1, deleted = true WHERE addr = $2 AND assetid = $3`)
		if err != nil {
			return fmt.Errorf("prepare asset close2, %v", err)
		}
		defer acd.Close()

		//////////////////
		// Asset Config //
		//////////////////
		setacfg, err := tx.Prepare(`INSERT INTO asset (index, creator_addr, params, created_at, deleted) VALUES ($1, $2, $3, $4, false) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params, deleted = false`)
		if err != nil {
			return fmt.Errorf("prepare set asset, %v", err)
		}
		defer setacfg.Close()
		getacfg, err := tx.Prepare(`SELECT params FROM asset WHERE index = $1`)
		if err != nil {
			return fmt.Errorf("prepare get asset, %v", err)
		}
		defer getacfg.Close()

		//////////////////
		// Asset Freeze //
		//////////////////
		fr, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen, created_at, deleted) VALUES ($1, $2, 0, $3, $4, false) ON CONFLICT (addr, assetid) DO UPDATE SET frozen = EXCLUDED.frozen, deleted = false`)
		if err != nil {
			return fmt.Errorf("prepare asset freeze, %v", err)
		}
		defer fr.Close()

		for _, subround := range updates.AssetUpdates {
			for addr, aulist := range subround {
				for _, au := range aulist {
					if au.AssetID == debugAsset {
						db.log.Errorf("%d axfer %s %s", round, encoding.Base64(addr[:]), obs(au))
					}

					// Apply deltas
					if au.Transfer != nil {
						if au.Transfer.Delta.IsInt64() {
							// easy case
							delta := au.Transfer.Delta.Int64()
							// don't skip delta == 0; mark opt-in
							_, err = seta.Exec(addr[:], au.AssetID, delta, au.DefaultFrozen, round)
							if err != nil {
								return fmt.Errorf("update account asset, %v", err)
							}
						} else {
							sign := au.Transfer.Delta.Sign()
							var mi big.Int
							var step int64
							if sign > 0 {
								mi.SetInt64(math.MaxInt64)
								step = math.MaxInt64
							} else if sign < 0 {
								mi.SetInt64(math.MinInt64)
								step = math.MinInt64
							} else {
								continue
							}
							for !au.Transfer.Delta.IsInt64() {
								_, err = seta.Exec(addr[:], au.AssetID, step, au.DefaultFrozen, round)
								if err != nil {
									return fmt.Errorf("update account asset, %v", err)
								}
								au.Transfer.Delta.Sub(&au.Transfer.Delta, &mi)
							}
							sign = au.Transfer.Delta.Sign()
							if sign != 0 {
								_, err = seta.Exec(addr[:], au.AssetID, au.Transfer.Delta.Int64(), au.DefaultFrozen, round)
								if err != nil {
									return fmt.Errorf("update account asset, %v", err)
								}
							}
						}
					}

					// Close holding before continuing to next subround.
					if au.Close != nil {
						_, err = acc.Exec(au.Close.Round, au.Close.Offset, au.Close.Sender[:], au.AssetID)
						if err != nil {
							return fmt.Errorf("asset close record amount, %v", err)
						}
						_, err = acs.Exec(au.Close.CloseTo[:], au.AssetID, au.DefaultFrozen, au.Close.Sender[:], au.AssetID, round)
						if err != nil {
							return fmt.Errorf("asset close send, %v", err)
						}
						_, err = acd.Exec(round, au.Close.Sender[:], au.AssetID)
						if err != nil {
							return fmt.Errorf("asset close del, %v", err)
						}
					}

					// Asset Config
					if au.Config != nil {
						var outparams []byte
						if au.Config.IsNew {
							outparams = encoding.EncodeJSON(au.Config.Params)
						} else {
							row := getacfg.QueryRow(au.AssetID)
							var paramjson []byte
							err = row.Scan(&paramjson)
							if err != nil {
								return fmt.Errorf("get acfg %d, %v", au.AssetID, err)
							}
							var old sdk_types.AssetParams
							err = encoding.DecodeJSON(paramjson, &old)
							if err != nil {
								return fmt.Errorf("bad acgf json %d, %v", au.AssetID, err)
							}
							np := types.MergeAssetConfig(old, au.Config.Params)
							outparams = encoding.EncodeJSON(np)
						}
						_, err = setacfg.Exec(au.AssetID, au.Config.Creator[:], outparams, round)
						if err != nil {
							return fmt.Errorf("update asset, %v", err)
						}
					}

					// Asset Freeze
					if au.Freeze != nil {
						if au.AssetID == debugAsset {
							db.log.Errorf("%d %s %s", round, encoding.Base64(addr[:]), obs(au.Freeze))
						}
						_, err = fr.Exec(addr[:], au.AssetID, au.Freeze.Frozen, round)
						if err != nil {
							return fmt.Errorf("update asset freeze, %v", err)
						}
					}
				}
			}
		}
	}
	if len(updates.AssetDestroys) > 0 {
		// Note! leaves `asset` and `account_asset` rows present for historical reference, but deletes all holdings from all accounts
		any = true
		// Update any account_asset holdings which were not previously closed. By now the amount should already be 0.
		ads, err := tx.Prepare(`UPDATE account_asset SET amount = 0, closed_at = $1, deleted = true WHERE addr = (SELECT creator_addr FROM asset WHERE index = $2) AND assetid = $2`)
		if err != nil {
			return fmt.Errorf("prepare asset destroy, %v", err)
		}
		defer ads.Close()
		// Clear out the parameters and set closed_at
		aclear, err := tx.Prepare(`UPDATE asset SET params = 'null'::jsonb, closed_at = $1, deleted = true WHERE index = $2`)
		if err != nil {
			return fmt.Errorf("prepare asset clear, %v", err)
		}
		defer aclear.Close()
		for _, assetID := range updates.AssetDestroys {
			if assetID == debugAsset {
				db.log.Errorf("%d destroy asset %d", round, assetID)
			}
			_, err = ads.Exec(round, assetID)
			if err != nil {
				return fmt.Errorf("asset destroy, %v", err)
			}
			_, err = aclear.Exec(round, assetID)
			if err != nil {
				return fmt.Errorf("asset destroy, %v", err)
			}
		}
	}
	if len(updates.AppGlobalDeltas) > 0 {
		// apps with dirty global state, collection of AppParams as dict
		destroy := make(map[uint64]bool)
		dirty := make(map[uint64]AppParams)
		appCreators := make(map[uint64][]byte)
		getglobal, err := tx.Prepare(`SELECT params FROM app WHERE index = $1`)
		if err != nil {
			return fmt.Errorf("prepare app global get, %v", err)
		}
		defer getglobal.Close()
		// reverseDeltas for txnupglobal below: [][json, round, intra]
		reverseDeltas := make([][]interface{}, 0, len(updates.AppGlobalDeltas))
		for _, adelta := range updates.AppGlobalDeltas {
			state, ok := dirty[uint64(adelta.AppIndex)]
			if !ok {
				row := getglobal.QueryRow(adelta.AppIndex)
				var paramsjson []byte
				err = row.Scan(&paramsjson)
				if err == sql.ErrNoRows {
					// no prior data, empty state
				} else if err != nil {
					return fmt.Errorf("app[%d] global get, %v", adelta.AppIndex, err)
				} else {
					err = encoding.DecodeJSON(paramsjson, &state)
					if err != nil {
						return fmt.Errorf("app[%d] global get json, %v", adelta.AppIndex, err)
					}
				}
			}
			// calculate reverse delta, apply delta to state, save state to dirty
			reverseDelta := idb.AppReverseDelta{
				OnCompletion: adelta.OnCompletion,
			}
			if len(adelta.ApprovalProgram) > 0 {
				reverseDelta.ApprovalProgram = state.ApprovalProgram
				state.ApprovalProgram = adelta.ApprovalProgram
			}
			if len(adelta.ClearStateProgram) > 0 {
				reverseDelta.ClearStateProgram = state.ClearStateProgram
				state.ClearStateProgram = adelta.ClearStateProgram
			}
			state.GlobalStateSchema.fromBlock(adelta.GlobalStateSchema)
			state.LocalStateSchema.fromBlock(adelta.LocalStateSchema)
			for key, vd := range adelta.Delta {
				err = applyKeyValueDelta(&state.GlobalState, []byte(key), vd, &reverseDelta)
				if err != nil {
					return fmt.Errorf("app delta apply err r=%d i=%d app=%d, %v", adelta.Round, adelta.Intra, adelta.AppIndex, err)
				}
			}
			reverseDelta.ExtraProgramPages = state.ExtraProgramPages
			state.ExtraProgramPages = adelta.ExtraProgramPages

			reverseDeltas = append(reverseDeltas, []interface{}{encoding.EncodeJSON(reverseDelta), adelta.Round, adelta.Intra})
			if adelta.OnCompletion == sdk_types.DeleteApplicationOC {
				// clear content but leave row recording that it existed
				state = AppParams{}
				destroy[uint64(adelta.AppIndex)] = true
			} else {
				delete(destroy, uint64(adelta.AppIndex))
			}
			dirty[uint64(adelta.AppIndex)] = state
			if adelta.Creator != nil {
				appCreators[uint64(adelta.AppIndex)] = adelta.Creator
			}
		}

		// update txns with reverse deltas
		// "agr" is "app global reverse"
		txnupglobal, err := tx.Prepare(`UPDATE txn ut SET extra = jsonb_set(coalesce(ut.extra, '{}'::jsonb), '{agr}', $1) WHERE ut.round = $2 AND ut.intra = $3`)
		if err != nil {
			return fmt.Errorf("prepare app global txn up, %v", err)
		}
		defer txnupglobal.Close()
		for _, rd := range reverseDeltas {
			_, err = txnupglobal.Exec(rd...)
			if err != nil {
				return fmt.Errorf("app global txn up, r=%d i=%d, %#v, %v", rd[1], rd[2], string(rd[0].([]byte)), err)
			}
		}
		// apply dirty global state deltas for the round
		putglobal, err := tx.Prepare(`INSERT INTO app (index, creator, params, created_at, deleted) VALUES ($1, $2, $3, $4, false) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params, closed_at = coalesce($5, app.closed_at), deleted = $6`)
		if err != nil {
			return fmt.Errorf("prepare app global put, %v", err)
		}
		defer putglobal.Close()
		for appid, params := range dirty {
			// Nullable closedAt value
			closedAt := sql.NullInt64{
				Int64: int64(round),
				Valid: destroy[appid],
			}
			creator := appCreators[appid]
			paramjson := encoding.EncodeJSON(params)
			_, err = putglobal.Exec(appid, creator, paramjson, round, closedAt, destroy[appid])
			if err != nil {
				return fmt.Errorf("app global put pj=%v, %v", string(paramjson), err)
			}
		}
	}
	if len(updates.AppLocalDeltas) > 0 {
		dirty := make([]inmemAppLocalState, 0, len(updates.AppLocalDeltas))
		getlocal, err := tx.Prepare(`SELECT localstate FROM account_app WHERE addr = $1 AND app = $2`)
		if err != nil {
			return fmt.Errorf("prepare app local get, %v", err)
		}
		defer getlocal.Close()
		// reverseDeltas for txnuplocal below: [][json, round, intra]
		reverseDeltas := make([][]interface{}, 0, len(updates.AppLocalDeltas))
		var droplocals [][]interface{}

		getapp, err := tx.Prepare(`SELECT params FROM app WHERE index = $1`)
		if err != nil {
			return fmt.Errorf("prepare app get (l), %v", err)
		}

		for _, ald := range updates.AppLocalDeltas {
			if ald.OnCompletion == sdk_types.CloseOutOC || ald.OnCompletion == sdk_types.ClearStateOC {
				droplocals = append(droplocals,
					[]interface{}{ald.Address, ald.AppIndex, round},
				)
				continue
			}
			localstate, err := db.getDirtyAppLocalState(ald.Address, ald.AppIndex, dirty, getlocal)
			if err != nil {
				return err
			}
			if ald.OnCompletion == sdk_types.OptInOC {
				row := getapp.QueryRow(ald.AppIndex)
				var paramsjson []byte
				err = row.Scan(&paramsjson)
				if err != nil {
					return fmt.Errorf("app get (l), %v", err)
				}
				var app AppParams
				err = encoding.DecodeJSON(paramsjson, &app)
				if err != nil {
					return fmt.Errorf("app[%d] get json (l), %v", ald.AppIndex, err)
				}
				localstate.Schema = app.LocalStateSchema
			}

			var reverseDelta idb.AppReverseDelta

			for key, vd := range ald.Delta {
				err = applyKeyValueDelta(&localstate.KeyValue, []byte(key), vd, &reverseDelta)
				if err != nil {
					return err
				}
			}
			dirty = setDirtyAppLocalState(dirty, localstate)
			reverseDeltas = append(reverseDeltas, []interface{}{encoding.EncodeJSON(reverseDelta), ald.Round, ald.Intra})
		}

		// update txns with reverse deltas
		// "alr" is "app local reverse"
		if len(reverseDeltas) > 0 {
			txnuplocal, err := tx.Prepare(`UPDATE txn ut SET extra = jsonb_set(coalesce(ut.extra, '{}'::jsonb), '{alr}', $1) WHERE ut.round = $2 AND ut.intra = $3`)
			if err != nil {
				return fmt.Errorf("prepare app local txn up, %v", err)
			}
			defer txnuplocal.Close()
			for _, rd := range reverseDeltas {
				_, err = txnuplocal.Exec(rd...)
				if err != nil {
					return fmt.Errorf("app local txn up, r=%d i=%d %v", rd[1], rd[2], err)
				}
			}
		}

		if len(dirty) > 0 {
			// apply local state deltas for the round
			putglobal, err := tx.Prepare(`INSERT INTO account_app (addr, app, localstate, created_at, deleted) VALUES ($1, $2, $3, $4, false) ON CONFLICT (addr, app) DO UPDATE SET localstate = EXCLUDED.localstate, deleted = false`)
			if err != nil {
				return fmt.Errorf("prepare app local put, %v", err)
			}
			defer putglobal.Close()
			for _, ld := range dirty {
				_, err = putglobal.Exec(ld.address, ld.appIndex, encoding.EncodeJSON(ld.AppLocalState), round)
				if err != nil {
					return fmt.Errorf("app local put, %v", err)
				}
			}
		}

		if len(droplocals) > 0 {
			droplocal, err := tx.Prepare(`UPDATE account_app SET localstate = NULL, closed_at = $3, deleted = true WHERE addr = $1 AND app = $2`)
			if err != nil {
				return fmt.Errorf("prepare app local del, %v", err)
			}
			defer droplocal.Close()
			for _, dl := range droplocals {
				_, err = droplocal.Exec(dl...)
				if err != nil {
					return fmt.Errorf("app local del, %v", err)
				}
			}
		}
	}
	if !any {
		db.log.Debugf("empty round %d", round)
	}

	importstate, err := db.getImportState(tx)
	if err != nil {
		return err
	}

	if importstate.NextRoundToAccount == nil {
		return fmt.Errorf("importstate.AccountRound is nil")
	}

	if uint64(*importstate.NextRoundToAccount) > round {
		return fmt.Errorf(
			"next round to account is %d while trying to write round %d",
			*importstate.NextRoundToAccount, round)
	}

	*importstate.NextRoundToAccount = round + 1
	err = db.setImportState(tx, importstate)
	if err != nil {
		return
	}

	return tx.Commit()
}

// CommitRoundAccounting is part of idb.IndexerDB
func (db *IndexerDb) CommitRoundAccounting(updates idb.RoundUpdates, round uint64, blockHeader *types.BlockHeader) error {
	f := func(ctx context.Context, tx *sql.Tx) (err error) {
		return db.commitRoundAccounting(tx, updates, round, blockHeader)
	}
	if err := db.txWithRetry(context.Background(), serializable, f); err != nil {
		return fmt.Errorf("CommitRoundAccounting(): %v", err)
	}
	return nil
}

// GetBlock is part of idb.IndexerDB
func (db *IndexerDb) GetBlock(ctx context.Context, round uint64, options idb.GetBlockOptions) (blockHeader types.BlockHeader, transactions []idb.TxnRow, err error) {
	tx, err := db.db.BeginTx(ctx, &readonlyRepeatableRead)
	if err != nil {
		return
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `SELECT header FROM block_header WHERE round = $1`, round)
	var blockheaderjson []byte
	err = row.Scan(&blockheaderjson)
	if err != nil {
		return
	}
	err = encoding.DecodeJSON(blockheaderjson, &blockHeader)
	if err != nil {
		return
	}

	if options.Transactions {
		out := make(chan idb.TxnRow, 1)
		query, whereArgs, err := buildTransactionQuery(idb.TransactionFilter{Round: &round})
		if err != nil {
			err = fmt.Errorf("txn query err %v", err)
			out <- idb.TxnRow{Error: err}
			close(out)
			return types.BlockHeader{}, nil, err
		}
		rows, err := tx.QueryContext(ctx, query, whereArgs...)
		if err != nil {
			err = fmt.Errorf("txn query %#v err %v", query, err)
			return types.BlockHeader{}, nil, err
		}

		go func() {
			db.yieldTxnsThreadSimple(ctx, rows, out, nil, nil)
			close(out)
		}()

		results := make([]idb.TxnRow, 0)
		for txrow := range out {
			results = append(results, txrow)
			txrow.Next()
		}
		transactions = results
	}

	return blockHeader, transactions, nil
}

func buildTransactionQuery(tf idb.TransactionFilter) (query string, whereArgs []interface{}, err error) {
	// TODO? There are some combinations of tf params that will
	// yield no results and we could catch that before asking the
	// database. A hopefully rare optimization.
	const maxWhereParts = 30
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs = make([]interface{}, 0, maxWhereParts)
	joinParticipation := false
	partNumber := 1
	if tf.Address != nil {
		whereParts = append(whereParts, fmt.Sprintf("p.addr = $%d", partNumber))
		whereArgs = append(whereArgs, tf.Address)
		partNumber++
		if tf.AddressRole != 0 {
			addrBase64 := encoding.Base64(tf.Address)
			roleparts := make([]string, 0, 8)
			if tf.AddressRole&idb.AddressRoleSender != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'snd' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleReceiver != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'rcv' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleCloseRemainderTo != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'close' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleAssetSender != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'asnd' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleAssetReceiver != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'arcv' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleAssetCloseTo != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'aclose' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&idb.AddressRoleFreeze != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'fadd' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			rolepart := strings.Join(roleparts, " OR ")
			whereParts = append(whereParts, "("+rolepart+")")
		}
		joinParticipation = true
	}
	if tf.MinRound != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.round >= $%d", partNumber))
		whereArgs = append(whereArgs, tf.MinRound)
		partNumber++
	}
	if tf.MaxRound != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.round <= $%d", partNumber))
		whereArgs = append(whereArgs, tf.MaxRound)
		partNumber++
	}
	if !tf.BeforeTime.IsZero() {
		whereParts = append(whereParts, fmt.Sprintf("h.realtime < $%d", partNumber))
		whereArgs = append(whereArgs, tf.BeforeTime)
		partNumber++
	}
	if !tf.AfterTime.IsZero() {
		whereParts = append(whereParts, fmt.Sprintf("h.realtime > $%d", partNumber))
		whereArgs = append(whereArgs, tf.AfterTime)
		partNumber++
	}
	if tf.AssetID != 0 || tf.ApplicationID != 0 {
		var creatableID uint64
		if tf.AssetID != 0 {
			creatableID = tf.AssetID
			if tf.ApplicationID != 0 {
				if tf.AssetID == tf.ApplicationID {
					// this is nonsense, but I'll allow it
				} else {
					return "", nil, fmt.Errorf("cannot search both assetid and appid")
				}
			}
		} else {
			creatableID = tf.ApplicationID
		}
		whereParts = append(whereParts, fmt.Sprintf("t.asset = $%d", partNumber))
		whereArgs = append(whereArgs, creatableID)
		partNumber++
	}
	if tf.AssetAmountGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'aamt')::bigint > $%d", partNumber))
		whereArgs = append(whereArgs, *tf.AssetAmountGT)
		partNumber++
	}
	if tf.AssetAmountLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'aamt')::bigint < $%d", partNumber))
		whereArgs = append(whereArgs, *tf.AssetAmountLT)
		partNumber++
	}
	if tf.TypeEnum != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.typeenum = $%d", partNumber))
		whereArgs = append(whereArgs, tf.TypeEnum)
		partNumber++
	}
	if len(tf.Txid) != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.txid = $%d", partNumber))
		whereArgs = append(whereArgs, tf.Txid)
		partNumber++
	}
	if tf.Round != nil {
		whereParts = append(whereParts, fmt.Sprintf("t.round = $%d", partNumber))
		whereArgs = append(whereArgs, *tf.Round)
		partNumber++
	}
	if tf.Offset != nil {
		whereParts = append(whereParts, fmt.Sprintf("t.intra = $%d", partNumber))
		whereArgs = append(whereArgs, *tf.Offset)
		partNumber++
	}
	if tf.OffsetLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("t.intra < $%d", partNumber))
		whereArgs = append(whereArgs, *tf.OffsetLT)
		partNumber++
	}
	if tf.OffsetGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("t.intra > $%d", partNumber))
		whereArgs = append(whereArgs, *tf.OffsetGT)
		partNumber++
	}
	if len(tf.SigType) != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.txn -> $%d IS NOT NULL", partNumber))
		whereArgs = append(whereArgs, tf.SigType)
		partNumber++
	}
	if len(tf.NotePrefix) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("substring(decode(t.txn -> 'txn' ->> 'note', 'base64') from 1 for %d) = $%d", len(tf.NotePrefix), partNumber))
		whereArgs = append(whereArgs, tf.NotePrefix)
		partNumber++
	}
	if tf.AlgosGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'amt')::bigint > $%d", partNumber))
		whereArgs = append(whereArgs, *tf.AlgosGT)
		partNumber++
	}
	if tf.AlgosLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'amt')::bigint < $%d", partNumber))
		whereArgs = append(whereArgs, *tf.AlgosLT)
		partNumber++
	}
	if tf.EffectiveAmountGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("((t.txn -> 'ca')::bigint + (t.txn -> 'txn' -> 'amt')::bigint) > $%d", partNumber))
		whereArgs = append(whereArgs, *tf.EffectiveAmountGT)
		partNumber++
	}
	if tf.EffectiveAmountLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("((t.txn -> 'ca')::bigint + (t.txn -> 'txn' -> 'amt')::bigint) < $%d", partNumber))
		whereArgs = append(whereArgs, *tf.EffectiveAmountLT)
		partNumber++
	}
	if tf.RekeyTo != nil && (*tf.RekeyTo) {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'rekey') IS NOT NULL"))
	}
	query = "SELECT t.round, t.intra, t.txnbytes, t.extra, t.asset, h.realtime FROM txn t JOIN block_header h ON t.round = h.round"
	if joinParticipation {
		query += " JOIN txn_participation p ON t.round = p.round AND t.intra = p.intra"
	}
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	if joinParticipation {
		// this should match the index on txn_particpation
		query += " ORDER BY p.addr, p.round DESC, p.intra DESC"
	} else {
		// this should explicitly match the primary key on txn (round,intra)
		query += " ORDER BY t.round, t.intra"
	}
	if tf.Limit != 0 {
		query += fmt.Sprintf(" LIMIT %d", tf.Limit)
	}
	return
}

// This function blocks. `tx` must be non-nil.
func (db *IndexerDb) yieldTxns(ctx context.Context, tx *sql.Tx, tf idb.TransactionFilter, out chan<- idb.TxnRow) {
	if len(tf.NextToken) > 0 {
		db.txnsWithNext(ctx, tx, tf, out)
		return
	}

	query, whereArgs, err := buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- idb.TxnRow{Error: err}
		return
	}

	rows, err := tx.Query(query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		return
	}

	db.yieldTxnsThreadSimple(ctx, rows, out, nil, nil)
}

// Transactions is part of idb.IndexerDB
func (db *IndexerDb) Transactions(ctx context.Context, tf idb.TransactionFilter) (<-chan idb.TxnRow, uint64) {
	out := make(chan idb.TxnRow, 1)

	tx, err := db.db.BeginTx(ctx, &readonlyRepeatableRead)
	if err != nil {
		out <- idb.TxnRow{Error: err}
		close(out)
		return out, 0
	}

	round, err := db.getMaxRoundAccounted(tx)
	if err != nil {
		tx.Rollback()
		out <- idb.TxnRow{Error: err}
		close(out)
		return out, round
	}

	go func() {
		db.yieldTxns(ctx, tx, tf, out)
		tx.Rollback()
		close(out)
	}()

	return out, round
}

func (db *IndexerDb) txTransactions(tx *sql.Tx, tf idb.TransactionFilter) <-chan idb.TxnRow {
	out := make(chan idb.TxnRow, 1)
	if len(tf.NextToken) > 0 {
		err := fmt.Errorf("txTransactions incompatible with next")
		out <- idb.TxnRow{Error: err}
		close(out)
		return out
	}
	query, whereArgs, err := buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- idb.TxnRow{Error: err}
		close(out)
		return out
	}
	rows, err := tx.Query(query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		close(out)
		return out
	}
	go func() {
		db.yieldTxnsThreadSimple(context.Background(), rows, out, nil, nil)
		close(out)
	}()
	return out
}

// This function blocks. `tx` must be non-nil.
func (db *IndexerDb) txnsWithNext(ctx context.Context, tx *sql.Tx, tf idb.TransactionFilter, out chan<- idb.TxnRow) {
	nextround, nextintra32, err := idb.DecodeTxnRowNext(tf.NextToken)
	nextintra := uint64(nextintra32)
	if err != nil {
		out <- idb.TxnRow{Error: err}
		return
	}
	origRound := tf.Round
	origOLT := tf.OffsetLT
	origOGT := tf.OffsetGT
	if tf.Address != nil {
		// (round,intra) descending into the past
		if nextround == 0 && nextintra == 0 {
			return
		}
		tf.Round = &nextround
		tf.OffsetLT = &nextintra
	} else {
		// (round,intra) ascending into the future
		tf.Round = &nextround
		tf.OffsetGT = &nextintra
	}
	query, whereArgs, err := buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- idb.TxnRow{Error: err}
		return
	}
	rows, err := tx.Query(query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		return
	}
	count := int(0)
	db.yieldTxnsThreadSimple(ctx, rows, out, &count, &err)
	if err != nil {
		return
	}
	if uint64(count) >= tf.Limit {
		return
	}
	tf.Limit -= uint64(count)
	select {
	case <-ctx.Done():
		return
	default:
	}
	tf.Round = origRound
	if tf.Address != nil {
		// (round,intra) descending into the past
		tf.OffsetLT = origOLT
		if nextround == 0 {
			// NO second query
			return
		}
		tf.MaxRound = nextround - 1
	} else {
		// (round,intra) ascending into the future
		tf.OffsetGT = origOGT
		tf.MinRound = nextround + 1
	}
	query, whereArgs, err = buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- idb.TxnRow{Error: err}
		return
	}
	rows, err = tx.Query(query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		return
	}
	db.yieldTxnsThreadSimple(ctx, rows, out, nil, nil)
}

func (db *IndexerDb) yieldTxnsThreadSimple(ctx context.Context, rows *sql.Rows, results chan<- idb.TxnRow, countp *int, errp *error) {
	defer rows.Close()

	count := 0
	for rows.Next() {
		var round uint64
		var asset uint64
		var intra int
		var txnbytes []byte
		var extraJSON []byte
		var roundtime time.Time
		err := rows.Scan(&round, &intra, &txnbytes, &extraJSON, &asset, &roundtime)
		var row idb.TxnRow
		if err != nil {
			row.Error = err
		} else {
			row.Round = round
			row.Intra = intra
			row.TxnBytes = txnbytes
			row.RoundTime = roundtime
			row.AssetID = asset
			if len(extraJSON) > 0 {
				err = encoding.DecodeJSON(extraJSON, &row.Extra)
				if err != nil {
					row.Error = fmt.Errorf("%d:%d decode txn extra, %v", row.Round, row.Intra, err)
				}
			}
		}
		select {
		case <-ctx.Done():
			goto finish
		case results <- row:
			if err != nil {
				if errp != nil {
					*errp = err
				}
				goto finish
			}
			count++
		}
	}
	if err := rows.Err(); err != nil {
		results <- idb.TxnRow{Error: err}
		if errp != nil {
			*errp = err
		}
	}
finish:
	if countp != nil {
		*countp = count
	}
}

var statusStrings = []string{"Offline", "Online", "NotParticipating"}

const offlineStatusIdx = 0

func (db *IndexerDb) yieldAccountsThread(req *getAccountsRequest) {
	count := uint64(0)
	defer func() {
		req.rows.Close()

		end := time.Now()
		dt := end.Sub(req.start)
		if dt > (1 * time.Second) {
			db.log.Warnf("long query %fs: %s", dt.Seconds(), req.query)
		}
	}()
	for req.rows.Next() {
		var addr []byte
		var microalgos uint64
		var rewardstotal uint64
		var createdat sql.NullInt64
		var closedat sql.NullInt64
		var deleted sql.NullBool
		var rewardsbase uint64
		var keytype *string
		var accountDataJSONStr []byte

		// below are bytes of json serialization

		// holding* are a triplet of lists that should merge together
		var holdingAssetids []byte
		var holdingAmount []byte
		var holdingFrozen []byte
		var holdingCreatedBytes []byte
		var holdingClosedBytes []byte
		var holdingDeletedBytes []byte

		// assetParams* are a pair of lists that should merge together
		var assetParamsIds []byte
		var assetParamsStr []byte
		var assetParamsCreatedBytes []byte
		var assetParamsClosedBytes []byte
		var assetParamsDeletedBytes []byte

		// appParam* are a pair of lists that should merge together
		var appParamIndexes []byte // [appId, ...]
		var appParams []byte       // [{AppParams}, ...]
		var appCreatedBytes []byte
		var appClosedBytes []byte
		var appDeletedBytes []byte

		// localState* are a pair of lists that should merge together
		var localStateAppIds []byte // [appId, ...]
		var localStates []byte      // [{local state}, ...]
		var localStateCreatedBytes []byte
		var localStateClosedBytes []byte
		var localStateDeletedBytes []byte

		var err error

		if req.opts.IncludeAssetHoldings && req.opts.IncludeAssetParams {
			err = req.rows.Scan(
				&addr, &microalgos, &rewardstotal, &createdat, &closedat, &deleted, &rewardsbase, &keytype, &accountDataJSONStr,
				&holdingAssetids, &holdingAmount, &holdingFrozen, &holdingCreatedBytes, &holdingClosedBytes, &holdingDeletedBytes,
				&assetParamsIds, &assetParamsStr, &assetParamsCreatedBytes, &assetParamsClosedBytes, &assetParamsDeletedBytes,
				&appParamIndexes, &appParams, &appCreatedBytes, &appClosedBytes, &appDeletedBytes, &localStateAppIds, &localStates,
				&localStateCreatedBytes, &localStateClosedBytes, &localStateDeletedBytes,
			)
		} else if req.opts.IncludeAssetHoldings {
			err = req.rows.Scan(
				&addr, &microalgos, &rewardstotal, &createdat, &closedat, &deleted, &rewardsbase, &keytype, &accountDataJSONStr,
				&holdingAssetids, &holdingAmount, &holdingFrozen, &holdingCreatedBytes, &holdingClosedBytes, &holdingDeletedBytes,
				&appParamIndexes, &appParams, &appCreatedBytes, &appClosedBytes, &appDeletedBytes, &localStateAppIds, &localStates,
				&localStateCreatedBytes, &localStateClosedBytes, &localStateDeletedBytes,
			)
		} else if req.opts.IncludeAssetParams {
			err = req.rows.Scan(
				&addr, &microalgos, &rewardstotal, &createdat, &closedat, &deleted, &rewardsbase, &keytype, &accountDataJSONStr,
				&assetParamsIds, &assetParamsStr, &assetParamsCreatedBytes, &assetParamsClosedBytes, &assetParamsDeletedBytes,
				&appParamIndexes, &appParams, &appCreatedBytes, &appClosedBytes, &appDeletedBytes, &localStateAppIds, &localStates,
				&localStateCreatedBytes, &localStateClosedBytes, &localStateDeletedBytes,
			)
		} else {
			err = req.rows.Scan(
				&addr, &microalgos, &rewardstotal, &createdat, &closedat, &deleted, &rewardsbase, &keytype, &accountDataJSONStr,
				&appParamIndexes, &appParams, &appCreatedBytes, &appClosedBytes, &appDeletedBytes, &localStateAppIds, &localStates,
				&localStateCreatedBytes, &localStateClosedBytes, &localStateDeletedBytes,
			)
		}
		if err != nil {
			err = fmt.Errorf("account scan err %v", err)
			req.out <- idb.AccountRow{Error: err}
			break
		}

		var account models.Account
		var aaddr sdk_types.Address
		copy(aaddr[:], addr)
		account.Address = aaddr.String()
		account.Round = uint64(req.blockheader.Round)
		account.AmountWithoutPendingRewards = microalgos
		account.Rewards = rewardstotal
		account.CreatedAtRound = nullableInt64Ptr(createdat)
		account.ClosedAtRound = nullableInt64Ptr(closedat)
		account.Deleted = nullableBoolPtr(deleted)
		account.RewardBase = new(uint64)
		*account.RewardBase = rewardsbase
		// default to Offline in there have been no keyreg transactions.
		account.Status = statusStrings[offlineStatusIdx]
		if keytype != nil && *keytype != "" {
			account.SigType = keytype
		}

		if accountDataJSONStr != nil {
			var ad types.AccountData
			err = encoding.DecodeJSON(accountDataJSONStr, &ad)
			if err != nil {
				err = fmt.Errorf("account decode err (%s) %v", accountDataJSONStr, err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			account.Status = statusStrings[ad.Status]
			hasSel := !allZero(ad.SelectionID[:])
			hasVote := !allZero(ad.VoteID[:])
			if hasSel || hasVote {
				part := new(models.AccountParticipation)
				if hasSel {
					part.SelectionParticipationKey = ad.SelectionID[:]
				}
				if hasVote {
					part.VoteParticipationKey = ad.VoteID[:]
				}
				part.VoteFirstValid = uint64(ad.VoteFirstValid)
				part.VoteLastValid = uint64(ad.VoteLastValid)
				part.VoteKeyDilution = ad.VoteKeyDilution
				account.Participation = part
			}

			if !ad.SpendingKey.IsZero() {
				var spendingkey sdk_types.Address
				copy(spendingkey[:], ad.SpendingKey[:])
				account.AuthAddr = stringPtr(spendingkey.String())
			}
		}

		if account.Status == "NotParticipating" {
			account.PendingRewards = 0
		} else {
			// TODO: pending rewards calculation doesn't belong in database layer (this is just the most covenient place which has all the data)
			proto, err := types.Protocol(string(req.blockheader.CurrentProtocol))
			if err != nil {
				err = fmt.Errorf("get protocol err (%s) %v", req.blockheader.CurrentProtocol, err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			rewardsUnits := uint64(0)
			if proto.RewardUnit != 0 {
				rewardsUnits = microalgos / proto.RewardUnit
			}
			rewardsDelta := req.blockheader.RewardsLevel - rewardsbase
			account.PendingRewards = rewardsUnits * rewardsDelta
		}
		account.Amount = microalgos + account.PendingRewards
		// not implemented: account.Rewards sum of all rewards ever

		const nullarraystr = "[null]"

		if len(holdingAssetids) > 0 && string(holdingAssetids) != nullarraystr {
			var haids []uint64
			err = encoding.DecodeJSON(holdingAssetids, &haids)
			if err != nil {
				err = fmt.Errorf("parsing json holding asset ids err %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var hamounts []uint64
			err = encoding.DecodeJSON(holdingAmount, &hamounts)
			if err != nil {
				err = fmt.Errorf("parsing json holding amounts err %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var hfrozen []bool
			err = encoding.DecodeJSON(holdingFrozen, &hfrozen)
			if err != nil {
				err = fmt.Errorf("parsing json holding frozen err %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingCreated []*uint64
			err = encoding.DecodeJSON(holdingCreatedBytes, &holdingCreated)
			if err != nil {
				err = fmt.Errorf("parsing json holding created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingClosed []*uint64
			err = encoding.DecodeJSON(holdingClosedBytes, &holdingClosed)
			if err != nil {
				err = fmt.Errorf("parsing json holding closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingDeleted []*bool
			err = encoding.DecodeJSON(holdingDeletedBytes, &holdingDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json holding deleted ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}

			if len(hamounts) != len(haids) || len(hfrozen) != len(haids) || len(holdingCreated) != len(haids) || len(holdingClosed) != len(haids) || len(holdingDeleted) != len(haids) {
				err = fmt.Errorf("account asset holding unpacking, all should be %d:  %d amounts, %d frozen, %d created, %d closed, %d deleted",
					len(haids), len(hamounts), len(hfrozen), len(holdingCreated), len(holdingClosed), len(holdingDeleted))
				req.out <- idb.AccountRow{Error: err}
				break
			}

			av := make([]models.AssetHolding, 0, len(haids))
			for i, assetid := range haids {
				// SQL can result in cross-product duplication when account has both asset holdings and assets created, de-dup here
				dup := false
				for _, xaid := range haids[:i] {
					if assetid == xaid {
						dup = true
						break
					}
				}
				if dup {
					continue
				}
				tah := models.AssetHolding{
					Amount:          hamounts[i],
					IsFrozen:        hfrozen[i],
					AssetId:         assetid,
					OptedOutAtRound: holdingClosed[i],
					OptedInAtRound:  holdingCreated[i],
					Deleted:         holdingDeleted[i],
				} // TODO: set Creator to asset creator addr string
				av = append(av, tah)
			}
			account.Assets = new([]models.AssetHolding)
			*account.Assets = av
		}
		if len(assetParamsIds) > 0 && string(assetParamsIds) != nullarraystr {
			var assetids []uint64
			err = encoding.DecodeJSON(assetParamsIds, &assetids)
			if err != nil {
				err = fmt.Errorf("parsing json asset param ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetParams []types.AssetParams
			err = encoding.DecodeJSON(assetParamsStr, &assetParams)
			if err != nil {
				err = fmt.Errorf("parsing json asset param string, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetCreated []*uint64
			err = encoding.DecodeJSON(assetParamsCreatedBytes, &assetCreated)
			if err != nil {
				err = fmt.Errorf("parsing json asset created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetClosed []*uint64
			err = encoding.DecodeJSON(assetParamsClosedBytes, &assetClosed)
			if err != nil {
				err = fmt.Errorf("parsing json asset closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetDeleted []*bool
			err = encoding.DecodeJSON(assetParamsDeletedBytes, &assetDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json asset deleted ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}

			if len(assetParams) != len(assetids) || len(assetCreated) != len(assetids) || len(assetClosed) != len(assetids) || len(assetDeleted) != len(assetids) {
				err = fmt.Errorf("account asset unpacking, all should be %d:  %d assetids, %d created, %d closed, %d deleted",
					len(assetParams), len(assetids), len(assetCreated), len(assetClosed), len(assetDeleted))
				req.out <- idb.AccountRow{Error: err}
				break
			}

			cal := make([]models.Asset, 0, len(assetids))
			for i, assetid := range assetids {
				// SQL can result in cross-product duplication when account has both asset holdings and assets created, de-dup here
				dup := false
				for _, xaid := range assetids[:i] {
					if assetid == xaid {
						dup = true
						break
					}
				}
				if dup {
					continue
				}
				ap := assetParams[i]

				tma := models.Asset{
					Index:            assetid,
					CreatedAtRound:   assetCreated[i],
					DestroyedAtRound: assetClosed[i],
					Deleted:          assetDeleted[i],
					Params: models.AssetParams{
						Creator:       account.Address,
						Total:         ap.Total,
						Decimals:      uint64(ap.Decimals),
						DefaultFrozen: boolPtr(ap.DefaultFrozen),
						UnitName:      stringPtr(ap.UnitName),
						Name:          stringPtr(ap.AssetName),
						Url:           stringPtr(ap.URL),
						MetadataHash:  baPtr(ap.MetadataHash[:]),
						Manager:       addrStr(ap.Manager),
						Reserve:       addrStr(ap.Reserve),
						Freeze:        addrStr(ap.Freeze),
						Clawback:      addrStr(ap.Clawback),
					},
				}
				cal = append(cal, tma)
			}
			account.CreatedAssets = new([]models.Asset)
			*account.CreatedAssets = cal
		}

		var totalSchema models.ApplicationStateSchema

		if len(appParamIndexes) > 0 {
			// apps owned by this account
			var appIds []uint64
			err = encoding.DecodeJSON(appParamIndexes, &appIds)
			if err != nil {
				err = fmt.Errorf("parsing json appids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appCreated []*uint64
			err = encoding.DecodeJSON(appCreatedBytes, &appCreated)
			if err != nil {
				err = fmt.Errorf("parsing json app created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appClosed []*uint64
			err = encoding.DecodeJSON(appClosedBytes, &appClosed)
			if err != nil {
				err = fmt.Errorf("parsing json app closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appDeleted []*bool
			err = encoding.DecodeJSON(appDeletedBytes, &appDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json app deleted flags, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}

			var apps []AppParams
			err = encoding.DecodeJSON(appParams, &apps)
			if err != nil {
				err = fmt.Errorf("parsing json appparams, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			if len(appIds) != len(apps) || len(appClosed) != len(apps) || len(appCreated) != len(apps) || len(appDeleted) != len(apps) {
				err = fmt.Errorf("account app unpacking, all should be %d:  %d appids, %d appClosed, %d appCreated, %d appDeleted", len(apps), len(appIds), len(appClosed), len(appCreated), len(appDeleted))
				req.out <- idb.AccountRow{Error: err}
				break
			}

			var totalExtraPages uint64
			aout := make([]models.Application, len(appIds))
			outpos := 0
			for i, appid := range appIds {
				aout[outpos].Id = appid
				aout[outpos].CreatedAtRound = appCreated[i]
				aout[outpos].DeletedAtRound = appClosed[i]
				aout[outpos].Deleted = appDeleted[i]
				aout[outpos].Params.Creator = &account.Address

				// If these are both nil the app was probably deleted, leave out params
				// some "required" fields will be left in the results.
				if apps[i].ApprovalProgram != nil || apps[i].ClearStateProgram != nil {
					aout[outpos].Params.ApprovalProgram = apps[i].ApprovalProgram
					aout[outpos].Params.ClearStateProgram = apps[i].ClearStateProgram
					aout[outpos].Params.GlobalState = apps[i].GlobalState.toModel()
					aout[outpos].Params.GlobalStateSchema = &models.ApplicationStateSchema{
						NumByteSlice: apps[i].GlobalStateSchema.NumByteSlice,
						NumUint:      apps[i].GlobalStateSchema.NumUint,
					}
					aout[outpos].Params.LocalStateSchema = &models.ApplicationStateSchema{
						NumByteSlice: apps[i].LocalStateSchema.NumByteSlice,
						NumUint:      apps[i].LocalStateSchema.NumUint,
					}
				}
				if aout[outpos].Deleted == nil || !*aout[outpos].Deleted {
					totalSchema.NumByteSlice += apps[i].GlobalStateSchema.NumByteSlice
					totalSchema.NumUint += apps[i].GlobalStateSchema.NumUint
					totalExtraPages += uint64(apps[i].ExtraProgramPages)
				}

				outpos++
			}
			if outpos != len(aout) {
				aout = aout[:outpos]
			}
			account.CreatedApps = &aout

			if totalExtraPages != 0 {
				account.AppsTotalExtraPages = &totalExtraPages
			}
		}

		if len(localStateAppIds) > 0 {
			var appIds []uint64
			err = encoding.DecodeJSON(localStateAppIds, &appIds)
			if err != nil {
				err = fmt.Errorf("parsing json local appids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appCreated []*uint64
			err = encoding.DecodeJSON(localStateCreatedBytes, &appCreated)
			if err != nil {
				err = fmt.Errorf("parsing json ls created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appClosed []*uint64
			err = encoding.DecodeJSON(localStateClosedBytes, &appClosed)
			if err != nil {
				err = fmt.Errorf("parsing json ls closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appDeleted []*bool
			err = encoding.DecodeJSON(localStateDeletedBytes, &appDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json ls closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var ls []AppLocalState
			err = encoding.DecodeJSON(localStates, &ls)
			if err != nil {
				err = fmt.Errorf("parsing json local states, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			if len(appIds) != len(ls) || len(appClosed) != len(ls) || len(appCreated) != len(ls) || len(appDeleted) != len(ls) {
				err = fmt.Errorf("account app unpacking, all should be %d:  %d appids, %d appClosed, %d appCreated, %d appDeleted", len(ls), len(appIds), len(appClosed), len(appCreated), len(appDeleted))
				req.out <- idb.AccountRow{Error: err}
				break
			}

			aout := make([]models.ApplicationLocalState, len(ls))
			for i, appid := range appIds {
				aout[i].Id = appid
				aout[i].OptedInAtRound = appCreated[i]
				aout[i].ClosedOutAtRound = appClosed[i]
				aout[i].Deleted = appDeleted[i]
				aout[i].Schema = models.ApplicationStateSchema{
					NumByteSlice: ls[i].Schema.NumByteSlice,
					NumUint:      ls[i].Schema.NumUint,
				}
				aout[i].KeyValue = ls[i].KeyValue.toModel()
				if aout[i].Deleted == nil || !*aout[i].Deleted {
					totalSchema.NumByteSlice += ls[i].Schema.NumByteSlice
					totalSchema.NumUint += ls[i].Schema.NumUint
				}

			}
			account.AppsLocalState = &aout
		}

		if totalSchema != (models.ApplicationStateSchema{}) {
			account.AppsTotalSchema = &totalSchema
		}

		select {
		case req.out <- idb.AccountRow{Account: account}:
			count++
			if req.opts.Limit != 0 && count >= req.opts.Limit {
				return
			}
		case <-req.ctx.Done():
			return
		}
	}
	if err := req.rows.Err(); err != nil {
		err = fmt.Errorf("error reading rows: %v", err)
		req.out <- idb.AccountRow{Error: err}
	}
}

func nullableInt64Ptr(x sql.NullInt64) *uint64 {
	if !x.Valid {
		return nil
	}
	return uint64Ptr(uint64(x.Int64))
}

func nullableBoolPtr(x sql.NullBool) *bool {
	if !x.Valid {
		return nil
	}
	return &x.Bool
}

func uintOrDefault(x *uint64) uint64 {
	if x != nil {
		return *x
	}
	return 0
}

func uint64Ptr(x uint64) *uint64 {
	out := new(uint64)
	*out = x
	return out
}

func boolPtr(x bool) *bool {
	out := new(bool)
	*out = x
	return out
}

func stringPtr(x string) *string {
	if len(x) == 0 {
		return nil
	}
	out := new(string)
	*out = x
	return out
}

func baPtr(x []byte) *[]byte {
	if x == nil || len(x) == 0 {
		return nil
	}
	allzero := true
	for _, b := range x {
		if b != 0 {
			allzero = false
			break
		}
	}
	if allzero {
		return nil
	}
	out := new([]byte)
	*out = x
	return out
}

func allZero(x []byte) bool {
	for _, v := range x {
		if v != 0 {
			return false
		}
	}
	return true
}

func addrStr(addr types.Address) *string {
	if addr.IsZero() {
		return nil
	}
	out := new(string)
	*out = addr.String()
	return out
}

type getAccountsRequest struct {
	ctx         context.Context
	opts        idb.AccountQueryOptions
	blockheader types.BlockHeader
	query       string
	rows        *sql.Rows
	out         chan idb.AccountRow
	start       time.Time
}

// GetAccounts is part of idb.IndexerDB
func (db *IndexerDb) GetAccounts(ctx context.Context, opts idb.AccountQueryOptions) (<-chan idb.AccountRow, uint64) {
	out := make(chan idb.AccountRow, 1)

	if opts.HasAssetID != 0 {
		opts.IncludeAssetHoldings = true
	} else if (opts.AssetGT != nil) || (opts.AssetLT != nil) {
		err := fmt.Errorf("AssetGT=%d, AssetLT=%d, but HasAssetID=%d", uintOrDefault(opts.AssetGT), uintOrDefault(opts.AssetLT), opts.HasAssetID)
		out <- idb.AccountRow{Error: err}
		close(out)
		return out, 0
	}

	// Begin transaction so we get everything at one consistent point in time and round of accounting.
	tx, err := db.db.BeginTx(ctx, &readonlyRepeatableRead)
	if err != nil {
		err = fmt.Errorf("account tx err %v", err)
		out <- idb.AccountRow{Error: err}
		close(out)
		return out, 0
	}

	// Get round number through which accounting has been updated
	round, err := db.getMaxRoundAccounted(tx)
	if err != nil {
		err = fmt.Errorf("account round err %v", err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}

	// Get block header for that round so we know protocol and rewards info
	row := tx.QueryRow(`SELECT header FROM block_header WHERE round = $1`, round)
	var headerjson []byte
	err = row.Scan(&headerjson)
	if err != nil {
		err = fmt.Errorf("account round header %d err %v", round, err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}
	var blockheader types.BlockHeader
	err = encoding.DecodeJSON(headerjson, &blockheader)
	if err != nil {
		err = fmt.Errorf("account round header %d err %v", round, err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}

	// Construct query for fetching accounts...
	query, whereArgs := db.buildAccountQuery(opts)
	req := &getAccountsRequest{
		ctx:         ctx,
		opts:        opts,
		blockheader: blockheader,
		query:       query,
		out:         out,
		start:       time.Now(),
	}
	req.rows, err = tx.Query(query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("account query %#v err %v", query, err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}
	go func() {
		db.yieldAccountsThread(req)
		close(req.out)
		tx.Rollback()
	}()
	return out, round
}

func (db *IndexerDb) buildAccountQuery(opts idb.AccountQueryOptions) (query string, whereArgs []interface{}) {
	// Construct query for fetching accounts...
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs = make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	withClauses := make([]string, 0, maxWhereParts)
	// filter by has-asset or has-app
	if opts.HasAssetID != 0 {
		aq := fmt.Sprintf("SELECT addr FROM account_asset WHERE assetid = $%d", partNumber)
		whereArgs = append(whereArgs, opts.HasAssetID)
		partNumber++
		if opts.AssetGT != nil {
			aq += fmt.Sprintf(" AND amount > $%d", partNumber)
			whereArgs = append(whereArgs, *opts.AssetGT)
			partNumber++
		}
		if opts.AssetLT != nil {
			aq += fmt.Sprintf(" AND amount < $%d", partNumber)
			whereArgs = append(whereArgs, *opts.AssetLT)
			partNumber++
		}
		aq = "qasf AS (" + aq + ")"
		withClauses = append(withClauses, aq)
	}
	if opts.HasAppID != 0 {
		withClauses = append(withClauses, fmt.Sprintf("qapf AS (SELECT addr FROM account_app WHERE app = $%d)", partNumber))
		whereArgs = append(whereArgs, opts.HasAppID)
		partNumber++
	}
	// filters against main account table
	if len(opts.GreaterThanAddress) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.addr > $%d", partNumber))
		whereArgs = append(whereArgs, opts.GreaterThanAddress)
		partNumber++
	}
	if len(opts.EqualToAddress) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.addr = $%d", partNumber))
		whereArgs = append(whereArgs, opts.EqualToAddress)
		partNumber++
	}
	if opts.AlgosGreaterThan != nil {
		whereParts = append(whereParts, fmt.Sprintf("a.microalgos > $%d", partNumber))
		whereArgs = append(whereArgs, *opts.AlgosGreaterThan)
		partNumber++
	}
	if opts.AlgosLessThan != nil {
		whereParts = append(whereParts, fmt.Sprintf("a.microalgos < $%d", partNumber))
		whereArgs = append(whereArgs, *opts.AlgosLessThan)
		partNumber++
	}
	if !opts.IncludeDeleted {
		whereParts = append(whereParts, "coalesce(a.deleted, false) = false")
	}
	if len(opts.EqualToAuthAddr) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("decode(a.account_data ->> 'spend', 'base64') = $%d", partNumber))
		whereArgs = append(whereArgs, opts.EqualToAuthAddr)
		partNumber++
	}
	query = `SELECT a.addr, a.microalgos, a.rewards_total, a.created_at, a.closed_at, a.deleted, a.rewardsbase, a.keytype, a.account_data FROM account a`
	if opts.HasAssetID != 0 {
		// inner join requires match, filtering on presence of asset
		query += " JOIN qasf ON a.addr = qasf.addr"
	}
	if opts.HasAppID != 0 {
		// inner join requires match, filtering on presence of app
		query += " JOIN qapf ON a.addr = qapf.addr"
	}
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY a.addr ASC"
	if opts.Limit != 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	// TODO: asset holdings and asset params are optional, but practically always used. Either make them actually always on, or make app-global and app-local clauses also optional (they are currently always on).
	withClauses = append(withClauses, "qaccounts AS ("+query+")")
	query = "WITH " + strings.Join(withClauses, ", ")
	if opts.IncludeDeleted {
		if opts.IncludeAssetHoldings {
			query += `, qaa AS (SELECT xa.addr, json_agg(aa.assetid) as haid, json_agg(aa.amount) as hamt, json_agg(aa.frozen) as hf, json_agg(aa.created_at) as holding_created_at, json_agg(aa.closed_at) as holding_closed_at, json_agg(coalesce(aa.deleted, false)) as holding_deleted FROM account_asset aa JOIN qaccounts xa ON aa.addr = xa.addr GROUP BY 1)`
		}
		if opts.IncludeAssetParams {
			query += `, qap AS (SELECT ya.addr, json_agg(ap.index) as paid, json_agg(ap.params) as pp, json_agg(ap.created_at) as asset_created_at, json_agg(ap.closed_at) as asset_closed_at, json_agg(ap.deleted) as asset_deleted FROM asset ap JOIN qaccounts ya ON ap.creator_addr = ya.addr GROUP BY 1)`
		}
		// app
		query += `, qapp AS (SELECT app.creator as addr, json_agg(app.index) as papps, json_agg(app.params) as ppa, json_agg(app.created_at) as app_created_at, json_agg(app.closed_at) as app_closed_at, json_agg(app.deleted) as app_deleted FROM app JOIN qaccounts ON qaccounts.addr = app.creator GROUP BY 1)`
		// app localstate
		query += `, qls AS (SELECT la.addr, json_agg(la.app) as lsapps, json_agg(la.localstate) as lsls, json_agg(la.created_at) as ls_created_at, json_agg(la.closed_at) as ls_closed_at, json_agg(la.deleted) as ls_deleted FROM account_app la JOIN qaccounts ON qaccounts.addr = la.addr GROUP BY 1)`
	} else {
		if opts.IncludeAssetHoldings {
			query += `, qaa AS (SELECT xa.addr, json_agg(aa.assetid) as haid, json_agg(aa.amount) as hamt, json_agg(aa.frozen) as hf, json_agg(aa.created_at) as holding_created_at, json_agg(aa.closed_at) as holding_closed_at, json_agg(coalesce(aa.deleted, false)) as holding_deleted FROM account_asset aa JOIN qaccounts xa ON aa.addr = xa.addr WHERE coalesce(aa.deleted, false) = false GROUP BY 1)`
		}
		if opts.IncludeAssetParams {
			query += `, qap AS (SELECT ya.addr, json_agg(ap.index) as paid, json_agg(ap.params) as pp, json_agg(ap.created_at) as asset_created_at, json_agg(ap.closed_at) as asset_closed_at, json_agg(ap.deleted) as asset_deleted FROM asset ap JOIN qaccounts ya ON ap.creator_addr = ya.addr WHERE coalesce(ap.deleted, false) = false GROUP BY 1)`
		}
		// app
		query += `, qapp AS (SELECT app.creator as addr, json_agg(app.index) as papps, json_agg(app.params) as ppa, json_agg(app.created_at) as app_created_at, json_agg(app.closed_at) as app_closed_at, json_agg(app.deleted) as app_deleted FROM app JOIN qaccounts ON qaccounts.addr = app.creator WHERE coalesce(app.deleted, false) = false GROUP BY 1)`
		// app localstate
		query += `, qls AS (SELECT la.addr, json_agg(la.app) as lsapps, json_agg(la.localstate) as lsls, json_agg(la.created_at) as ls_created_at, json_agg(la.closed_at) as ls_closed_at, json_agg(la.deleted) as ls_deleted FROM account_app la JOIN qaccounts ON qaccounts.addr = la.addr WHERE coalesce(la.deleted, false) = false GROUP BY 1)`
	}

	// query results
	query += ` SELECT za.addr, za.microalgos, za.rewards_total, za.created_at, za.closed_at, za.deleted, za.rewardsbase, za.keytype, za.account_data`
	if opts.IncludeAssetHoldings {
		query += `, qaa.haid, qaa.hamt, qaa.hf, qaa.holding_created_at, qaa.holding_closed_at, qaa.holding_deleted`
	}
	if opts.IncludeAssetParams {
		query += `, qap.paid, qap.pp, qap.asset_created_at, qap.asset_closed_at, qap.asset_deleted`
	}
	query += `, qapp.papps, qapp.ppa, qapp.app_created_at, qapp.app_closed_at, qapp.app_deleted, qls.lsapps, qls.lsls, qls.ls_created_at, qls.ls_closed_at, qls.ls_deleted FROM qaccounts za`

	// join everything together
	if opts.IncludeAssetHoldings {
		query += ` LEFT JOIN qaa ON za.addr = qaa.addr`
	}
	if opts.IncludeAssetParams {
		query += ` LEFT JOIN qap ON za.addr = qap.addr`
	}
	query += " LEFT JOIN qapp ON za.addr = qapp.addr LEFT JOIN qls ON qls.addr = za.addr ORDER BY za.addr ASC;"
	return query, whereArgs
}

// Assets is part of idb.IndexerDB
func (db *IndexerDb) Assets(ctx context.Context, filter idb.AssetsQuery) (<-chan idb.AssetRow, uint64) {
	query := `SELECT index, creator_addr, params, created_at, closed_at, deleted FROM asset a`
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if filter.AssetID != 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.index = $%d", partNumber))
		whereArgs = append(whereArgs, filter.AssetID)
		partNumber++
	}
	if filter.AssetIDGreaterThan != 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.index > $%d", partNumber))
		whereArgs = append(whereArgs, filter.AssetIDGreaterThan)
		partNumber++
	}
	if filter.Creator != nil {
		whereParts = append(whereParts, fmt.Sprintf("a.creator_addr = $%d", partNumber))
		whereArgs = append(whereArgs, filter.Creator)
		partNumber++
	}
	if filter.Name != "" {
		whereParts = append(whereParts, fmt.Sprintf("a.params ->> 'an' ILIKE $%d", partNumber))
		whereArgs = append(whereArgs, "%"+filter.Name+"%")
		partNumber++
	}
	if filter.Unit != "" {
		whereParts = append(whereParts, fmt.Sprintf("a.params ->> 'un' ILIKE $%d", partNumber))
		whereArgs = append(whereArgs, "%"+filter.Unit+"%")
		partNumber++
	}
	if filter.Query != "" {
		qs := "%" + filter.Query + "%"
		whereParts = append(whereParts, fmt.Sprintf("(a.params ->> 'un' ILIKE $%d OR a.params ->> 'an' ILIKE $%d)", partNumber, partNumber))
		whereArgs = append(whereArgs, qs)
		partNumber++
	}
	if !filter.IncludeDeleted {
		whereParts = append(whereParts, "coalesce(a.deleted, false) = false")
	}
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY index ASC"
	if filter.Limit != 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	out := make(chan idb.AssetRow, 1)

	tx, err := db.db.BeginTx(ctx, &readonlyRepeatableRead)
	if err != nil {
		out <- idb.AssetRow{Error: err}
		close(out)
		return out, 0
	}

	round, err := db.getMaxRoundAccounted(tx)
	if err != nil {
		out <- idb.AssetRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}

	rows, err := tx.Query(query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("asset query %#v err %v", query, err)
		out <- idb.AssetRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}
	go func() {
		db.yieldAssetsThread(ctx, filter, rows, out)
		close(out)
		tx.Rollback()
	}()
	return out, round
}

func (db *IndexerDb) yieldAssetsThread(ctx context.Context, filter idb.AssetsQuery, rows *sql.Rows, out chan<- idb.AssetRow) {
	defer rows.Close()

	for rows.Next() {
		var index uint64
		var creatorAddr []byte
		var paramsJSONStr []byte
		var created *uint64
		var closed *uint64
		var deleted *bool
		var err error

		err = rows.Scan(&index, &creatorAddr, &paramsJSONStr, &created, &closed, &deleted)
		if err != nil {
			out <- idb.AssetRow{Error: err}
			break
		}
		var params types.AssetParams
		err = encoding.DecodeJSON(paramsJSONStr, &params)
		if err != nil {
			out <- idb.AssetRow{Error: err}
			break
		}
		rec := idb.AssetRow{
			AssetID:      index,
			Creator:      creatorAddr,
			Params:       params,
			CreatedRound: created,
			ClosedRound:  closed,
			Deleted:      deleted,
		}
		select {
		case <-ctx.Done():
			return
		case out <- rec:
		}
	}
	if err := rows.Err(); err != nil {
		out <- idb.AssetRow{Error: err}
	}
}

// AssetBalances is part of idb.IndexerDB
func (db *IndexerDb) AssetBalances(ctx context.Context, abq idb.AssetBalanceQuery) (<-chan idb.AssetBalanceRow, uint64) {
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if abq.AssetID != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.assetid = $%d", partNumber))
		whereArgs = append(whereArgs, abq.AssetID)
		partNumber++
	}
	if abq.AmountGT != nil {
		whereParts = append(whereParts, fmt.Sprintf("aa.amount > $%d", partNumber))
		whereArgs = append(whereArgs, *abq.AmountGT)
		partNumber++
	}
	if abq.AmountLT != nil {
		whereParts = append(whereParts, fmt.Sprintf("aa.amount < $%d", partNumber))
		whereArgs = append(whereArgs, *abq.AmountLT)
		partNumber++
	}
	if len(abq.PrevAddress) != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.addr > $%d", partNumber))
		whereArgs = append(whereArgs, abq.PrevAddress)
		partNumber++
	}
	if !abq.IncludeDeleted {
		whereParts = append(whereParts, "coalesce(aa.deleted, false) = false")
	}
	query := `SELECT addr, assetid, amount, frozen, created_at, closed_at, deleted FROM account_asset aa`
	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += " ORDER BY addr ASC"
	if abq.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", abq.Limit)
	}

	out := make(chan idb.AssetBalanceRow, 1)

	tx, err := db.db.BeginTx(ctx, &readonlyRepeatableRead)
	if err != nil {
		out <- idb.AssetBalanceRow{Error: err}
		close(out)
		return out, 0
	}

	round, err := db.getMaxRoundAccounted(tx)
	if err != nil {
		out <- idb.AssetBalanceRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}

	rows, err := tx.Query(query, whereArgs...)
	if err != nil {
		out <- idb.AssetBalanceRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}
	go func() {
		db.yieldAssetBalanceThread(ctx, rows, out)
		close(out)
		tx.Rollback()
	}()
	return out, round
}

func (db *IndexerDb) yieldAssetBalanceThread(ctx context.Context, rows *sql.Rows, out chan<- idb.AssetBalanceRow) {
	defer rows.Close()

	for rows.Next() {
		var addr []byte
		var assetID uint64
		var amount uint64
		var frozen bool
		var created *uint64
		var closed *uint64
		var deleted *bool
		err := rows.Scan(&addr, &assetID, &amount, &frozen, &created, &closed, &deleted)
		if err != nil {
			out <- idb.AssetBalanceRow{Error: err}
			break
		}
		rec := idb.AssetBalanceRow{
			Address:      addr,
			AssetID:      assetID,
			Amount:       amount,
			Frozen:       frozen,
			ClosedRound:  closed,
			CreatedRound: created,
			Deleted:      deleted,
		}
		select {
		case <-ctx.Done():
			return
		case out <- rec:
		}
	}
	if err := rows.Err(); err != nil {
		out <- idb.AssetBalanceRow{Error: err}
	}
}

// Applications is part of idb.IndexerDB
func (db *IndexerDb) Applications(ctx context.Context, filter *models.SearchForApplicationsParams) (<-chan idb.ApplicationRow, uint64) {
	out := make(chan idb.ApplicationRow, 1)
	if filter == nil {
		out <- idb.ApplicationRow{Error: fmt.Errorf("no arguments provided to application search")}
		close(out)
		return out, 0
	}

	query := `SELECT index, creator, params, created_at, closed_at, deleted FROM app `

	const maxWhereParts = 30
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if filter.ApplicationId != nil {
		whereParts = append(whereParts, fmt.Sprintf("index = $%d", partNumber))
		whereArgs = append(whereArgs, *filter.ApplicationId)
		partNumber++
	}
	if filter.Next != nil {
		whereParts = append(whereParts, fmt.Sprintf("index > $%d", partNumber))
		whereArgs = append(whereArgs, *filter.Next)
		partNumber++
	}
	if filter.IncludeAll == nil || !(*filter.IncludeAll) {
		whereParts = append(whereParts, "coalesce(deleted, false) = false")
	}
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY 1"
	if filter.Limit != nil {
		query += fmt.Sprintf(" LIMIT %d", *filter.Limit)
	}

	tx, err := db.db.BeginTx(ctx, &readonlyRepeatableRead)
	if err != nil {
		out <- idb.ApplicationRow{Error: err}
		close(out)
		return out, 0
	}

	round, err := db.getMaxRoundAccounted(tx)
	if err != nil {
		out <- idb.ApplicationRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}

	rows, err := tx.Query(query, whereArgs...)
	if err != nil {
		out <- idb.ApplicationRow{Error: err}
		close(out)
		tx.Rollback()
		return out, round
	}

	go func() {
		db.yieldApplicationsThread(ctx, rows, out)
		close(out)
		tx.Rollback()
	}()
	return out, round
}

func (db *IndexerDb) yieldApplicationsThread(ctx context.Context, rows *sql.Rows, out chan idb.ApplicationRow) {
	defer rows.Close()

	for rows.Next() {
		var index uint64
		var creator []byte
		var paramsjson []byte
		var created *uint64
		var closed *uint64
		var deleted *bool
		err := rows.Scan(&index, &creator, &paramsjson, &created, &closed, &deleted)
		if err != nil {
			out <- idb.ApplicationRow{Error: err}
			break
		}
		var rec idb.ApplicationRow
		rec.Application.Id = index
		rec.Application.CreatedAtRound = created
		rec.Application.DeletedAtRound = closed
		rec.Application.Deleted = deleted
		var ap AppParams
		err = encoding.DecodeJSON(paramsjson, &ap)
		if err != nil {
			rec.Error = fmt.Errorf("app=%d json err, %v", index, err)
			out <- rec
			break
		}
		rec.Application.Params.ApprovalProgram = ap.ApprovalProgram
		rec.Application.Params.ClearStateProgram = ap.ClearStateProgram
		rec.Application.Params.Creator = new(string)

		var aaddr sdk_types.Address
		copy(aaddr[:], creator)
		rec.Application.Params.Creator = new(string)
		*(rec.Application.Params.Creator) = aaddr.String()
		rec.Application.Params.GlobalState = ap.GlobalState.toModel()
		rec.Application.Params.GlobalStateSchema = &models.ApplicationStateSchema{
			NumByteSlice: ap.GlobalStateSchema.NumByteSlice,
			NumUint:      ap.GlobalStateSchema.NumUint,
		}
		rec.Application.Params.LocalStateSchema = &models.ApplicationStateSchema{
			NumByteSlice: ap.LocalStateSchema.NumByteSlice,
			NumUint:      ap.LocalStateSchema.NumUint,
		}

		if ap.ExtraProgramPages != 0 {
			rec.Application.Params.ExtraProgramPages = new(uint64)
			*rec.Application.Params.ExtraProgramPages = uint64(ap.ExtraProgramPages)
		}

		out <- rec
	}
	if err := rows.Err(); err != nil {
		out <- idb.ApplicationRow{Error: err}
	}
}

// Health is part of idb.IndexerDB
func (db *IndexerDb) Health() (idb.Health, error) {
	migrationRequired := false
	migrating := false
	blocking := false
	errString := ""
	var data = make(map[string]interface{})

	if db.readonly {
		data["read-only-mode"] = true
	}

	if db.migration != nil {
		state := db.migration.GetStatus()

		if state.Err != nil {
			errString = state.Err.Error()
		}
		if state.Status != "" {
			data["migration-status"] = state.Status
		}

		migrationRequired = state.Running
		migrating = state.Running
		blocking = state.Blocking
	} else {
		state, err := db.getMigrationState()
		if err != nil {
			return idb.Health{}, err
		}

		blocking = migrationStateBlocked(state)
		migrationRequired = needsMigration(state)
	}

	data["migration-required"] = migrationRequired

	round, err := db.getMaxRoundAccounted(nil)

	// We'll just have to set the round to 0
	if err == idb.ErrorNotInitialized {
		err = nil
		round = 0
	}

	return idb.Health{
		Data:        &data,
		Round:       round,
		IsMigrating: migrating,
		DBAvailable: !blocking,
		Error:       errString,
	}, err
}

// GetSpecialAccounts is part of idb.IndexerDB
func (db *IndexerDb) GetSpecialAccounts() (idb.SpecialAccounts, error) {
	cache, err := db.getMetastate(nil, specialAccountsMetastateKey)
	if err != nil {
		if err != idb.ErrorNotInitialized {
			return idb.SpecialAccounts{}, fmt.Errorf("GetSpecialAccounts() err: %w", err)
		}

		// Initialize specialAccountsMetastateKey
		blockHeader, _, err := db.GetBlock(context.Background(), 0, idb.GetBlockOptions{})
		if err != nil {
			err = fmt.Errorf(
				"GetSpecialAccounts() problem looking up special accounts from genesis "+
					"block, err: %w", err)
			return idb.SpecialAccounts{}, err
		}

		accounts := idb.SpecialAccounts{
			FeeSink:     blockHeader.FeeSink,
			RewardsPool: blockHeader.RewardsPool,
		}

		cache := encoding.EncodeJSON(accounts)
		err = db.setMetastate(nil, specialAccountsMetastateKey, string(cache))
		if err != nil {
			return idb.SpecialAccounts{}, fmt.Errorf("problem saving metastate: %v", err)
		}

		return accounts, nil
	}

	var accounts idb.SpecialAccounts
	err = encoding.DecodeJSON([]byte(cache), &accounts)
	if err != nil {
		err = fmt.Errorf(
			"GetSpecialAccounts() problem decoding, cache: '%s' err: %w", cache, err)
		return idb.SpecialAccounts{}, err
	}

	return accounts, nil
}
