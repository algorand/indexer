// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package postgres

// import text to contstant setup_postgres_sql
//go:generate go run ../../cmd/texttosource/main.go postgres setup_postgres.sql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"

	// Load the postgres sql.DB implementation
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/migration"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
)

const stateMetastateKey = "state"
const migrationMetastateKey = "migration"
const specialAccountsMetastateKey = "accounts"

// OpenPostgres is available for creating test instances of postgres.IndexerDb
func OpenPostgres(connection string, opts *idb.IndexerDbOptions, log *log.Logger) (pdb *IndexerDb, err error) {
	db, err := sql.Open("postgres", connection)

	if err != nil {
		return nil, err
	}

	if strings.Contains(connection, "readonly") {
		if opts == nil {
			opts = &idb.IndexerDbOptions{}
		}
		opts.ReadOnly = true
	}

	return openPostgres(db, opts, log)
}

// Allow tests to inject a DB
func openPostgres(db *sql.DB, opts *idb.IndexerDbOptions, logger *log.Logger) (pdb *IndexerDb, err error) {
	pdb = &IndexerDb{
		log:        logger,
		db:         db,
		protoCache: make(map[string]types.ConsensusParams, 20),
	}

	if pdb.log == nil {
		pdb.log = log.New()
		pdb.log.SetFormatter(&log.JSONFormatter{})
		pdb.log.SetOutput(os.Stdout)
		pdb.log.SetLevel(log.TraceLevel)
	}

	// e.g. a user named "readonly" is in the connection string
	readonly := (opts != nil) && opts.ReadOnly
	if !readonly {
		err = pdb.init()
	}
	return
}

// IndexerDb is an idb.IndexerDB implementation
type IndexerDb struct {
	log *log.Logger

	db *sql.DB

	// state for StartBlock/AddTransaction/CommitBlock
	tx        *sql.Tx
	addtx     *sql.Stmt
	addtxpart *sql.Stmt

	txrows  [][]interface{}
	txprows [][]interface{}

	protoCache map[string]types.ConsensusParams

	migration *migration.Migration
}

func (db *IndexerDb) init() (err error) {
	accountingStateJSON, _ := db.GetMetastate(stateMetastateKey)
	hasAccounting := len(accountingStateJSON) > 0
	migrationStateJSON, _ := db.GetMetastate(migrationMetastateKey)
	hasMigration := len(migrationStateJSON) > 0

	db.GetSpecialAccounts()

	if hasMigration || hasAccounting {
		// see postgres_migrations.go
		return db.runAvailableMigrations(migrationStateJSON)
	}

	// new database, run setup
	_, err = db.db.Exec(setup_postgres_sql)
	if err != nil {
		return
	}
	err = db.markMigrationsAsDone()
	return
}

// AlreadyImported is part of idb.IndexerDB
func (db *IndexerDb) AlreadyImported(path string) (imported bool, err error) {
	row := db.db.QueryRow(`SELECT COUNT(path) FROM imported WHERE path = $1`, path)
	numpath := 0
	err = row.Scan(&numpath)
	return numpath == 1, err
}

// MarkImported is part of idb.IndexerDB
func (db *IndexerDb) MarkImported(path string) (err error) {
	_, err = db.db.Exec(`INSERT INTO imported (path) VALUES ($1)`, path)
	return err
}

// StartBlock is part of idb.IndexerDB
func (db *IndexerDb) StartBlock() (err error) {
	db.txrows = make([][]interface{}, 0, 6000)
	db.txprows = make([][]interface{}, 0, 10000)
	return nil
}

// For App apply data, convert "string" keys which are secretly []byte blobs to their base64 representation so that JSON systems that require strings to be utf8 don't panic.
func stxnToJSON(txn types.SignedTxnWithAD) []byte {
	jt := txn
	if len(jt.EvalDelta.GlobalDelta) > 0 {
		gd := make(map[string]types.ValueDelta, len(jt.EvalDelta.GlobalDelta))
		for k, v := range jt.EvalDelta.GlobalDelta {
			gd[b64([]byte(k))] = v
		}
		jt.EvalDelta.GlobalDelta = gd
	}
	if len(jt.EvalDelta.LocalDeltas) > 0 {
		ldout := make(map[uint64]types.StateDelta, len(jt.EvalDelta.LocalDeltas))
		for i, ld := range jt.EvalDelta.LocalDeltas {
			nld := make(map[string]types.ValueDelta, len(ld))
			for k, v := range ld {
				nld[b64([]byte(k))] = v
			}
			ldout[i] = nld
		}
		jt.EvalDelta.LocalDeltas = ldout
	}
	return idb.JSONOneLine(jt)
}

// AddTransaction is part of idb.IndexerDB
func (db *IndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error {
	txnbytes := msgpack.Encode(txn)
	jsonbytes := stxnToJSON(txn)
	txid := crypto.TransactionIDString(txn.Txn)
	tx := []interface{}{round, intra, txtypeenum, assetid, txid[:], txnbytes, jsonbytes}
	db.txrows = append(db.txrows, tx)
	for _, paddr := range participation {
		seen := false
		if !seen {
			txp := []interface{}{paddr, round, intra}
			db.txprows = append(db.txprows, txp)
		}
	}
	return nil
}

// CommitBlock is part of idb.IndexerDB
func (db *IndexerDb) CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
	tx, err := db.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("BeginTx %v", err)
	}
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
				db.log.Printf("%s %d %d", base64.StdEncoding.EncodeToString(er[0].([]byte)), er[1], er[2])
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

	var block types.Block
	err = msgpack.Decode(headerbytes, &block)
	if err != nil {
		return fmt.Errorf("decode header %v", err)
	}
	headerjson := json.Encode(block)
	_, err = tx.Exec(`INSERT INTO block_header (round, realtime, rewardslevel, header) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, round, time.Unix(timestamp, 0).UTC(), rewardslevel, string(headerjson))
	if err != nil {
		return fmt.Errorf("put block_header %v", err)
	}

	err = tx.Commit()
	db.txrows = nil
	db.txprows = nil
	if err != nil {
		return fmt.Errorf("on commit, %v", err)
	}
	return err
}

// GetAsset return AssetParams about an asset
func (db *IndexerDb) GetAsset(assetid uint64) (asset types.AssetParams, err error) {
	row := db.db.QueryRow(`SELECT params FROM asset WHERE index = $1`, assetid)
	var assetjson string
	err = row.Scan(&assetjson)
	if err != nil {
		return
	}
	err = json.Decode([]byte(assetjson), &asset)
	return
}

// GetDefaultFrozen get {assetid:default frozen, ...} for all assets
func (db *IndexerDb) GetDefaultFrozen() (defaultFrozen map[uint64]bool, err error) {
	rows, err := db.db.Query(`SELECT index, params -> 'df' FROM asset a`)
	if err != nil {
		return
	}
	defaultFrozen = make(map[uint64]bool)
	for rows.Next() {
		var assetid uint64
		var frozen bool
		err = rows.Scan(&assetid, &frozen)
		if err != nil {
			return
		}
		defaultFrozen[assetid] = frozen
	}
	return
}

// LoadGenesis is part of idb.IndexerDB
func (db *IndexerDb) LoadGenesis(genesis types.Genesis) (err error) {
	tx, err := db.db.Begin()
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
		addr, err := atypes.DecodeAddress(alloc.Address)
		if len(alloc.State.AssetParams) > 0 || len(alloc.State.Assets) > 0 {
			return fmt.Errorf("genesis account[%d] has unhandled asset", ai)
		}
		_, err = setAccount.Exec(addr[:], alloc.State.MicroAlgos, string(json.Encode(alloc.State)), 0)
		total += uint64(alloc.State.MicroAlgos)
		if err != nil {
			return fmt.Errorf("error setting genesis account[%d], %v", ai, err)
		}
	}
	err = tx.Commit()
	db.log.Printf("genesis %d accounts %d microalgos, err=%v", len(genesis.Allocation), total, err)
	return err

}

// SetProto is part of idb.IndexerDB
func (db *IndexerDb) SetProto(version string, proto types.ConsensusParams) (err error) {
	pj := json.Encode(proto)
	if err != nil {
		return err
	}
	_, err = db.db.Exec(`INSERT INTO protocol (version, proto) VALUES ($1, $2) ON CONFLICT (version) DO UPDATE SET proto = EXCLUDED.proto`, version, pj)
	return err
}

// GetProto is part of idb.IndexerDB
func (db *IndexerDb) GetProto(version string) (proto types.ConsensusParams, err error) {
	proto, hit := db.protoCache[version]
	if hit {
		return
	}
	row := db.db.QueryRow(`SELECT proto FROM protocol WHERE version = $1`, version)
	var protostr string
	err = row.Scan(&protostr)
	if err != nil {
		return
	}
	err = json.Decode([]byte(protostr), &proto)
	if err == nil {
		db.protoCache[version] = proto
	}
	return
}

// GetMetastate is part of idb.IndexerDB
func (db *IndexerDb) GetMetastate(key string) (jsonStrValue string, err error) {
	row := db.db.QueryRow(`SELECT v FROM metastate WHERE k = $1`, key)
	err = row.Scan(&jsonStrValue)
	if err == sql.ErrNoRows {
		err = nil
	}
	if err != nil {
		jsonStrValue = ""
	}
	return
}

const setMetastateUpsert = `INSERT INTO metastate (k, v) VALUES ($1, $2) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`

// SetMetastate is part of idb.IndexerDB
func (db *IndexerDb) SetMetastate(key, jsonStrValue string) (err error) {
	_, err = db.db.Exec(setMetastateUpsert, key, jsonStrValue)
	return
}

// GetMaxRound is part of idb.IndexerDB
func (db *IndexerDb) GetMaxRound() (round uint64, err error) {
	var nullableRound sql.NullInt64
	round = 0
	row := db.db.QueryRow(`SELECT max(round) FROM block_header`)
	err = row.Scan(&nullableRound)

	if err == nil && nullableRound.Valid {
		round = uint64(nullableRound.Int64)
	}

	return
}

// Break the read query so that PostgreSQL doesn't get bogged down
// tracking transactional changes to tables.
const txnQueryBatchSize = 20000

var yieldTxnQuery string

func init() {
	yieldTxnQuery = fmt.Sprintf(`SELECT t.round, t.intra, t.txnbytes, t.extra, t.asset, b.realtime FROM txn t JOIN block_header b ON t.round = b.round WHERE t.round > $1 ORDER BY round, intra LIMIT %d`, txnQueryBatchSize)
}

func (db *IndexerDb) yieldTxnsThread(ctx context.Context, rows *sql.Rows, results chan<- idb.TxnRow) {
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
				rows.Close()
				close(results)
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
			var row idb.TxnRow
			row.Round = rounds[i]
			row.RoundTime = roundtimes[i]
			row.Intra = intras[i]
			row.TxnBytes = txnbytess[i]
			row.AssetID = uint64(creatableids[i])
			if len(extrajsons[i]) > 0 {
				err := json.Decode(extrajsons[i], &row.Extra)
				if err != nil {
					row.Error = fmt.Errorf("%d:%d decode txn extra, %v", row.Round, row.Intra, err)
					results <- row
					close(results)
					return
				}
			}
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
			rows, err = db.db.QueryContext(ctx, yieldTxnQuery, prevRound)
			if err != nil {
				results <- idb.TxnRow{Error: err}
				break
			}
		}
	}
	close(results)
}

// YieldTxns is part of idb.IndexerDB
func (db *IndexerDb) YieldTxns(ctx context.Context, prevRound int64) <-chan idb.TxnRow {
	results := make(chan idb.TxnRow, 1)
	rows, err := db.db.QueryContext(ctx, yieldTxnQuery, prevRound)
	if err != nil {
		results <- idb.TxnRow{Error: err}
		close(results)
		return results
	}
	go db.yieldTxnsThread(ctx, rows, results)
	return results
}

// TODO: maybe make a flag to set this, but in case of bug set this to
// debug any asset that isn't working out right:
var debugAsset uint64 = 0

func b64(addr []byte) string {
	return base64.StdEncoding.EncodeToString(addr)
}

func obs(x interface{}) string {
	return string(json.Encode(x))
}

// StateSchema like go-algorand data/basics/teal.go
type StateSchema struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	NumUint      uint64 `codec:"nui"`
	NumByteSlice uint64 `codec:"nbs"`
}

func (ss *StateSchema) fromBlock(x atypes.StateSchema) {
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
		return models.TealValue{Bytes: b64(tv.Bytes), Type: uint64(tv.Type)}
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
		out[pos].Key = b64(ktv.Key)
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

// MarshalJSON wraps json.Encode
func (tkv TealKeyValue) MarshalJSON() ([]byte, error) {
	return json.Encode(tkv.They), nil
}

// UnmarshalJSON wraps json.Decode
func (tkv *TealKeyValue) UnmarshalJSON(data []byte) error {
	return json.Decode(data, &tkv.They)
}

// AppParams like go-algorand data/basics/userBalance.go AppParams{}
type AppParams struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	ApprovalProgram   []byte      `codec:"approv"`
	ClearStateProgram []byte      `codec:"clearp"`
	LocalStateSchema  StateSchema `codec:"lsch"`
	GlobalStateSchema StateSchema `codec:"gsch"`

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
		err = json.Decode(localstatejson, &localstate.AppLocalState)
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

// CommitRoundAccounting is part of idb.IndexerDB
func (db *IndexerDb) CommitRoundAccounting(updates idb.RoundUpdates, round, rewardsBase uint64) (err error) {
	any := false
	tx, err := db.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback() // ignored if .Commit() first

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
				_, err = upsertalgo.Exec(addr[:], delta.Balance, rewardsBase, delta.Rewards, round)
				if err != nil {
					return fmt.Errorf("update algo, %v", err)
				}
			} else {
				_, err = closealgo.Exec(addr[:], delta.Balance, rewardsBase, delta.Rewards, round, round)
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
		setkeyreg, err := tx.Prepare(`UPDATE account SET account_data = coalesce(account_data, '{}'::jsonb) || ($1)::jsonb WHERE addr = $2`)
		if err != nil {
			return fmt.Errorf("prepare keyreg, %v", err)
		}
		defer setkeyreg.Close()
		for addr, adu := range updates.AccountDataUpdates {
			jb := json.Encode(adu)
			_, err = setkeyreg.Exec(jb, addr[:])
			if err != nil {
				return fmt.Errorf("update keyreg, %v", err)
			}
		}
	}
	if len(updates.AcfgUpdates) > 0 {
		any = true
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
		for _, au := range updates.AcfgUpdates {
			if au.AssetID == debugAsset {
				db.log.Errorf("%d acfg %s %s", round, b64(au.Creator[:]), obs(au))
			}
			var outparams string
			if au.IsNew {
				outparams = string(json.Encode(au.Params))
			} else {
				row := getacfg.QueryRow(au.AssetID)
				var paramjson []byte
				err = row.Scan(&paramjson)
				if err != nil {
					return fmt.Errorf("get acfg %d, %v", au.AssetID, err)
				}
				var old atypes.AssetParams
				err = json.Decode(paramjson, &old)
				if err != nil {
					return fmt.Errorf("bad acgf json %d, %v", au.AssetID, err)
				}
				np := types.MergeAssetConfig(old, au.Params)
				outparams = string(json.Encode(np))
			}
			_, err = setacfg.Exec(au.AssetID, au.Creator[:], outparams, round)
			if err != nil {
				return fmt.Errorf("update asset, %v", err)
			}
		}
	}
	if len(updates.TxnAssetUpdates) > 0 {
		any = true
		uta, err := tx.Prepare(`UPDATE txn SET asset = $1 WHERE round = $2 AND intra = $3`)
		if err != nil {
			return fmt.Errorf("prepare update txn.asset, %v", err)
		}
		for _, tau := range updates.TxnAssetUpdates {
			_, err = uta.Exec(tau.AssetID, tau.Round, tau.Offset)
			if err != nil {
				return fmt.Errorf("update txn.asset, %v", err)
			}
		}
		defer uta.Close()
	}
	if len(updates.AssetUpdates) > 0 && len(updates.AssetUpdates[0]) > 0 {
		any = true
		// Create new account_asset, or initialize a previously destroyed asset.
		seta, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen, created_at, deleted) VALUES ($1, $2, $3, $4, $5, false) ON CONFLICT (addr, assetid) DO UPDATE SET amount = account_asset.amount + EXCLUDED.amount, deleted = false`)
		if err != nil {
			return fmt.Errorf("prepare set account_asset, %v", err)
		}
		defer seta.Close()

		// On asset opt-out attach some extra "apply data" metadata to allow rewinding the asset close if requested.
		acc, err := tx.Prepare(`WITH aaamount AS (SELECT ($1)::bigint as round, ($2)::bigint as intra, x.amount FROM account_asset x WHERE x.addr = $3 AND x.assetid = $4)
UPDATE txn ut SET extra = jsonb_set(coalesce(ut.extra, '{}'::jsonb), '{aca}', to_jsonb(aaamount.amount)) FROM aaamount WHERE ut.round = aaamount.round AND ut.intra = aaamount.intra`)
		if err != nil {
			return fmt.Errorf("prepare asset close0, %v", err)
		}
		defer acc.Close()
		// On asset opt-out update the CloseTo account_asset
		acs, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen, created_at, deleted)
SELECT $1, $2, x.amount, $3, $6, false FROM account_asset x WHERE x.addr = $4 AND x.assetid = $5
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


		for _, subround := range updates.AssetUpdates {
			for addr, aulist := range subround {
				for _, au := range aulist {
					if au.AssetID == debugAsset {
						db.log.Errorf("%d axfer %s %s", round, b64(addr[:]), obs(au))
					}

					// Apply deltas
					if au.Delta.IsInt64() {
						// easy case
						delta := au.Delta.Int64()
						// don't skip delta == 0; mark opt-in
						_, err = seta.Exec(addr[:], au.AssetID, delta, au.DefaultFrozen, round)
						if err != nil {
							return fmt.Errorf("update account asset, %v", err)
						}
					} else {
						sign := au.Delta.Sign()
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
						for !au.Delta.IsInt64() {
							_, err = seta.Exec(addr[:], au.AssetID, step, au.DefaultFrozen, round)
							if err != nil {
								return fmt.Errorf("update account asset, %v", err)
							}
							au.Delta.Sub(&au.Delta, &mi)
						}
						sign = au.Delta.Sign()
						if sign != 0 {
							_, err = seta.Exec(addr[:], au.AssetID, au.Delta.Int64(), au.DefaultFrozen, round)
							if err != nil {
								return fmt.Errorf("update account asset, %v", err)
							}
						}
					}

					// Close holding before continuing to next subround.
					if au.Closed != nil {
						_, err = acc.Exec(au.Closed.Round, au.Closed.Offset, au.Closed.Sender[:], au.Closed.AssetID)
						if err != nil {
							return fmt.Errorf("asset close record amount, %v", err)
						}
						_, err = acs.Exec(au.Closed.CloseTo[:], au.Closed.AssetID, au.Closed.DefaultFrozen, au.Closed.Sender[:], au.Closed.AssetID, round)
						if err != nil {
							return fmt.Errorf("asset close send, %v", err)
						}
						_, err = acd.Exec(round, au.Closed.Sender[:], au.Closed.AssetID)
						if err != nil {
							return fmt.Errorf("asset close del, %v", err)
						}
					}
				}
			}
		}
	}
	if len(updates.FreezeUpdates) > 0 {
		any = true
		fr, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen, created_at, deleted) VALUES ($1, $2, 0, $3, $4, false) ON CONFLICT (addr, assetid) DO UPDATE SET frozen = EXCLUDED.frozen, deleted = false`)
		if err != nil {
			return fmt.Errorf("prepare asset freeze, %v", err)
		}
		defer fr.Close()
		for _, fs := range updates.FreezeUpdates {
			if fs.AssetID == debugAsset {
				db.log.Errorf("%d %s %s", round, b64(fs.Addr[:]), obs(fs))
			}
			_, err = fr.Exec(fs.Addr[:], fs.AssetID, fs.Frozen, round)
			if err != nil {
				return fmt.Errorf("update asset freeze, %v", err)
			}
		}
	}
	if len(updates.AssetDestroys) > 0 {
		// Note! leaves `asset` and `account_asset` rows present for historical reference, but deletes all holdings from all accounts
		any = true
		// Update any account_asset holdings which were not previously closed. By now the amount should already be 0.
		ads, err := tx.Prepare(`UPDATE account_asset SET amount = 0, closed_at = $1, deleted = true WHERE assetid = $2 AND amount != 0`)
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
					err = json.Decode(paramsjson, &state)
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
			reverseDeltas = append(reverseDeltas, []interface{}{json.Encode(reverseDelta), adelta.Round, adelta.Intra})
			if adelta.OnCompletion == atypes.DeleteApplicationOC {
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
			paramjson := json.Encode(params)
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
			if ald.OnCompletion == atypes.CloseOutOC || ald.OnCompletion == atypes.ClearStateOC {
				droplocals = append(droplocals,
					[]interface{}{ald.Address, ald.AppIndex, round},
				)
				continue
			}
			localstate, err := db.getDirtyAppLocalState(ald.Address, ald.AppIndex, dirty, getlocal)
			if err != nil {
				return err
			}
			if ald.OnCompletion == atypes.OptInOC {
				row := getapp.QueryRow(ald.AppIndex)
				var paramsjson []byte
				err = row.Scan(&paramsjson)
				if err != nil {
					return fmt.Errorf("app get (l), %v", err)
				}
				var app AppParams
				err = json.Decode(paramsjson, &app)
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
			reverseDeltas = append(reverseDeltas, []interface{}{json.Encode(reverseDelta), ald.Round, ald.Intra})
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
				_, err = putglobal.Exec(ld.address, ld.appIndex, json.Encode(ld.AppLocalState), round)
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
		db.log.Printf("empty round %d", round)
	}
	var istate importer.ImportState
	staterow := tx.QueryRow(`SELECT v FROM metastate WHERE k = 'state'`)
	var stateJSONStr string
	err = staterow.Scan(&stateJSONStr)
	if err == sql.ErrNoRows {
		// ok
	} else if err != nil {
		return
	} else {
		err = json.Decode([]byte(stateJSONStr), &istate)
		if err != nil {
			return
		}
	}
	istate.AccountRound = int64(round)
	sjs := string(json.Encode(istate))
	_, err = tx.Exec(setMetastateUpsert, stateMetastateKey, sjs)
	if err != nil {
		return
	}
	return tx.Commit()
}

// GetBlock is part of idb.IndexerDB
func (db *IndexerDb) GetBlock(round uint64) (block types.Block, err error) {
	row := db.db.QueryRow(`SELECT header FROM block_header WHERE round = $1`, round)
	var blockheaderjson []byte
	err = row.Scan(&blockheaderjson)
	if err != nil {
		return
	}
	err = json.Decode(blockheaderjson, &block)
	return
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
			addrBase64 := base64.StdEncoding.EncodeToString(tf.Address)
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
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'afrz' = $%d", partNumber))
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
	if tf.AssetAmountGT != 0 {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'aamt')::bigint > $%d", partNumber))
		whereArgs = append(whereArgs, tf.AssetAmountGT)
		partNumber++
	}
	if tf.AssetAmountLT != 0 {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'aamt')::bigint < $%d", partNumber))
		whereArgs = append(whereArgs, tf.AssetAmountLT)
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
	if tf.AlgosGT != 0 {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'amt')::bigint > $%d", partNumber))
		whereArgs = append(whereArgs, tf.AlgosGT)
		partNumber++
	}
	if tf.AlgosLT != 0 {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'amt')::bigint < $%d", partNumber))
		whereArgs = append(whereArgs, tf.AlgosLT)
		partNumber++
	}
	if tf.EffectiveAmountGt != 0 {
		whereParts = append(whereParts, fmt.Sprintf("((t.txn -> 'ca')::bigint + (t.txn -> 'txn' -> 'amt')::bigint) > $%d", partNumber))
		whereArgs = append(whereArgs, tf.EffectiveAmountGt)
		partNumber++
	}
	if tf.EffectiveAmountLt != 0 {
		whereParts = append(whereParts, fmt.Sprintf("((t.txn -> 'ca')::bigint + (t.txn -> 'txn' -> 'amt')::bigint) < $%d", partNumber))
		whereArgs = append(whereArgs, tf.EffectiveAmountLt)
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

// Transactions is part of idb.IndexerDB
func (db *IndexerDb) Transactions(ctx context.Context, tf idb.TransactionFilter) <-chan idb.TxnRow {
	out := make(chan idb.TxnRow, 1)
	if len(tf.NextToken) > 0 {
		go db.txnsWithNext(ctx, tf, out)
		return out
	}
	query, whereArgs, err := buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- idb.TxnRow{Error: err}
		close(out)
		return out
	}
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		close(out)
		return out
	}
	go db.yieldTxnsThreadSimple(ctx, rows, out, true, nil, nil)
	return out
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
	go db.yieldTxnsThreadSimple(context.Background(), rows, out, true, nil, nil)
	return out
}

func (db *IndexerDb) txnsWithNext(ctx context.Context, tf idb.TransactionFilter, out chan<- idb.TxnRow) {
	nextround, nextintra32, err := idb.DecodeTxnRowNext(tf.NextToken)
	nextintra := uint64(nextintra32)
	if err != nil {
		out <- idb.TxnRow{Error: err}
		close(out)
	}
	origRound := tf.Round
	origOLT := tf.OffsetLT
	origOGT := tf.OffsetGT
	if tf.Address != nil {
		// (round,intra) descending into the past
		if nextround == 0 && nextintra == 0 {
			close(out)
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
		close(out)
		return
	}
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		close(out)
		return
	}
	count := int(0)
	db.yieldTxnsThreadSimple(ctx, rows, out, false, &count, &err)
	if err != nil {
		close(out)
		return
	}
	if uint64(count) >= tf.Limit {
		close(out)
		return
	}
	tf.Limit -= uint64(count)
	select {
	case <-ctx.Done():
		close(out)
		return
	default:
	}
	tf.Round = origRound
	if tf.Address != nil {
		// (round,intra) descending into the past
		tf.OffsetLT = origOLT
		if nextround == 0 {
			// NO second query
			close(out)
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
		close(out)
		return
	}
	rows, err = db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- idb.TxnRow{Error: err}
		close(out)
		return
	}
	db.yieldTxnsThreadSimple(ctx, rows, out, true, nil, nil)
}

func (db *IndexerDb) yieldTxnsThreadSimple(ctx context.Context, rows *sql.Rows, results chan<- idb.TxnRow, doClose bool, countp *int, errp *error) {
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
				err = json.Decode(extraJSON, &row.Extra)
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
finish:
	if doClose {
		close(results)
	}
	if countp != nil {
		*countp = count
	}
}

const maxAccountsLimit = 1000

var statusStrings = []string{"Offline", "Online", "NotParticipating"}

const offlineStatusIdx = 0

func (db *IndexerDb) yieldAccountsThread(req *getAccountsRequest) {
	defer req.tx.Rollback()
	count := uint64(0)
	defer func() {
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
			req.out <- idb.AccountRow{Error: err}
			break
		}

		var account models.Account
		var aaddr atypes.Address
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
			err = json.Decode(accountDataJSONStr, &ad)
			if err != nil {
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
				var spendingkey atypes.Address
				copy(spendingkey[:], ad.SpendingKey[:])
				account.AuthAddr = stringPtr(spendingkey.String())
			}
		}

		if account.Status == "NotParticipating" {
			account.PendingRewards = 0
		} else {
			// TODO: pending rewards calculation doesn't belong in database layer (this is just the most covenient place which has all the data)
			proto, err := db.GetProto(string(req.blockheader.CurrentProtocol))
			if err != nil {
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
			err = json.Decode(holdingAssetids, &haids)
			if err != nil {
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var hamounts []uint64
			err = json.Decode(holdingAmount, &hamounts)
			if err != nil {
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var hfrozen []bool
			err = json.Decode(holdingFrozen, &hfrozen)
			if err != nil {
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingCreated []*uint64
			err = json.Decode(holdingCreatedBytes, &holdingCreated)
			if err != nil {
				err = fmt.Errorf("parsing json holding created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingClosed []*uint64
			err = json.Decode(holdingClosedBytes, &holdingClosed)
			if err != nil {
				err = fmt.Errorf("parsing json holding closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var holdingDeleted []*bool
			err = json.Decode(holdingDeletedBytes, &holdingDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json holding deleted ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}

			if len(hamounts) != len(haids) || len(hfrozen) != len(haids) || len(holdingCreated) != len(haids) || len(holdingClosed) != len(haids) || len(holdingDeleted) != len(haids){
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
			err = json.Decode(assetParamsIds, &assetids)
			if err != nil {
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetParams []types.AssetParams
			err = json.Decode(assetParamsStr, &assetParams)
			if err != nil {
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetCreated []*uint64
			err = json.Decode(assetParamsCreatedBytes, &assetCreated)
			if err != nil {
				err = fmt.Errorf("parsing json asset created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetClosed []*uint64
			err = json.Decode(assetParamsClosedBytes, &assetClosed)
			if err != nil {
				err = fmt.Errorf("parsing json asset closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var assetDeleted []*bool
			err = json.Decode(assetParamsDeletedBytes, &assetDeleted)
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

		if len(appParamIndexes) > 0 {
			// apps owned by this account
			var appIds []uint64
			err = json.Decode(appParamIndexes, &appIds)
			if err != nil {
				err = fmt.Errorf("parsing json appids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appCreated []*uint64
			err = json.Decode(appCreatedBytes, &appCreated)
			if err != nil {
				err = fmt.Errorf("parsing json app created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appClosed []*uint64
			err = json.Decode(appClosedBytes, &appClosed)
			if err != nil {
				err = fmt.Errorf("parsing json app closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appDeleted []*bool
			err = json.Decode(appDeletedBytes, &appDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json app deleted flags, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}

			var apps []AppParams
			str := string(appParams)
			fmt.Println(str)
			err = json.Decode(appParams, &apps)
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

				outpos++
			}
			if outpos != len(aout) {
				aout = aout[:outpos]
			}
			account.CreatedApps = &aout
		}

		if len(localStateAppIds) > 0 {
			var appIds []uint64
			err = json.Decode(localStateAppIds, &appIds)
			if err != nil {
				err = fmt.Errorf("parsing json local appids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appCreated []*uint64
			err = json.Decode(localStateCreatedBytes, &appCreated)
			if err != nil {
				err = fmt.Errorf("parsing json ls created ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appClosed []*uint64
			err = json.Decode(localStateClosedBytes, &appClosed)
			if err != nil {
				err = fmt.Errorf("parsing json ls closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var appDeleted []*bool
			err = json.Decode(localStateDeletedBytes, &appDeleted)
			if err != nil {
				err = fmt.Errorf("parsing json ls closed ids, %v", err)
				req.out <- idb.AccountRow{Error: err}
				break
			}
			var ls []AppLocalState
			err = json.Decode(localStates, &ls)
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
			}
			account.AppsLocalState = &aout
		}

		// Sometimes the migration state effects what data should be returned.
		db.processAccount(&account)

		select {
		case req.out <- idb.AccountRow{Account: account}:
			count++
			if req.opts.Limit != 0 && count >= req.opts.Limit {
				close(req.out)
				return
			}
		case <-req.ctx.Done():
			close(req.out)
			return
		}
	}
	close(req.out)
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

var emptyString = ""

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

func bytesStr(addr []byte) *string {
	if len(addr) == 0 {
		return nil
	}
	if allZero(addr) {
		return nil
	}
	out := new(string)
	*out = b64(addr)
	return out
}

var readOnlyTx = sql.TxOptions{ReadOnly: true}

type getAccountsRequest struct {
	ctx         context.Context
	opts        idb.AccountQueryOptions
	tx          *sql.Tx
	blockheader types.Block
	query       string
	rows        *sql.Rows
	out         chan idb.AccountRow
	start       time.Time
}

// GetAccounts is part of idb.IndexerDB
func (db *IndexerDb) GetAccounts(ctx context.Context, opts idb.AccountQueryOptions) <-chan idb.AccountRow {
	out := make(chan idb.AccountRow, 1)

	if opts.HasAssetID != 0 {
		opts.IncludeAssetHoldings = true
	} else if (opts.AssetGT != 0) || (opts.AssetLT != 0) {
		err := fmt.Errorf("AssetGT=%d, AssetLT=%d, but HasAssetID=%d", opts.AssetGT, opts.AssetLT, opts.HasAssetID)
		out <- idb.AccountRow{Error: err}
		close(out)
		return out
	}

	// Begin transaction so we get everything at one consistent point in time and round of accounting.
	tx, err := db.db.BeginTx(ctx, &readOnlyTx)
	if err != nil {
		err = fmt.Errorf("account tx err %v", err)
		out <- idb.AccountRow{Error: err}
		close(out)
		return out
	}

	// Get round number through which accounting has been updated
	row := tx.QueryRow(`SELECT (v -> 'account_round')::bigint as account_round FROM metastate WHERE k = 'state'`)
	var accountRound uint64
	err = row.Scan(&accountRound)
	if err != nil {
		err = fmt.Errorf("account_round err %v", err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out
	}

	// Get block header for that round so we know protocol and rewards info
	row = tx.QueryRow(`SELECT header FROM block_header WHERE round = $1`, accountRound)
	var headerjson []byte
	err = row.Scan(&headerjson)
	if err != nil {
		err = fmt.Errorf("account round header %d err %v", accountRound, err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out
	}
	var blockheader types.Block
	err = json.Decode(headerjson, &blockheader)
	if err != nil {
		err = fmt.Errorf("account round header %d err %v", accountRound, err)
		out <- idb.AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out
	}

	// Construct query for fetching accounts...
	query, whereArgs := db.buildAccountQuery(opts)
	req := &getAccountsRequest{
		ctx:         ctx,
		opts:        opts,
		tx:          tx,
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
		return out
	}
	go db.yieldAccountsThread(req)
	return out
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
		if opts.AssetGT != 0 {
			aq += fmt.Sprintf(" AND amount > $%d", partNumber)
			whereArgs = append(whereArgs, opts.AssetGT)
			partNumber++
		}
		if opts.AssetLT != 0 {
			aq += fmt.Sprintf(" AND amount < $%d", partNumber)
			whereArgs = append(whereArgs, opts.AssetLT)
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
	if opts.AlgosGreaterThan != 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.microalgos > $%d", partNumber))
		whereArgs = append(whereArgs, opts.AlgosGreaterThan)
		partNumber++
	}
	if opts.AlgosLessThan != 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.microalgos < $%d", partNumber))
		whereArgs = append(whereArgs, opts.AlgosLessThan)
		partNumber++
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
	if opts.IncludeAssetHoldings {
		query += `, qaa AS (SELECT xa.addr, json_agg(aa.assetid) as haid, json_agg(aa.amount) as hamt, json_agg(aa.frozen) as hf, json_agg(aa.created_at) as holding_created_at, json_agg(aa.closed_at) as holding_closed_at, json_agg(aa.deleted) as holding_deleted FROM account_asset aa JOIN qaccounts xa ON aa.addr = xa.addr GROUP BY 1)`
	}
	if opts.IncludeAssetParams {
		query += `, qap AS (SELECT ya.addr, json_agg(ap.index) as paid, json_agg(ap.params) as pp, json_agg(ap.created_at) as asset_created_at, json_agg(ap.closed_at) as asset_closed_at, json_agg(ap.deleted) as asset_deleted FROM asset ap JOIN qaccounts ya ON ap.creator_addr = ya.addr GROUP BY 1)`
	}
	// app
	query += `, qapp AS (SELECT app.creator as addr, json_agg(app.index) as papps, json_agg(app.params) as ppa, json_agg(app.created_at) as app_created_at, json_agg(app.closed_at) as app_closed_at, json_agg(app.deleted) as app_deleted FROM app JOIN qaccounts ON qaccounts.addr = app.creator GROUP BY 1)`
	// app localstate
	query += `, qls AS (SELECT la.addr, json_agg(la.app) as lsapps, json_agg(la.localstate) as lsls, json_agg(la.created_at) as ls_created_at, json_agg(la.closed_at) as ls_closed_at, json_agg(la.deleted) as ls_deleted FROM account_app la JOIN qaccounts ON qaccounts.addr = la.addr GROUP BY 1)`

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
func (db *IndexerDb) Assets(ctx context.Context, filter idb.AssetsQuery) <-chan idb.AssetRow {
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
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY index ASC"
	if filter.Limit != 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	out := make(chan idb.AssetRow, 1)
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("asset query %#v err %v", query, err)
		out <- idb.AssetRow{Error: err}
		close(out)
		return out
	}
	go db.yieldAssetsThread(ctx, filter, rows, out)
	return out
}

func (db *IndexerDb) yieldAssetsThread(ctx context.Context, filter idb.AssetsQuery, rows *sql.Rows, out chan<- idb.AssetRow) {
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
		err = json.Decode(paramsJSONStr, &params)
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
			close(out)
			return
		case out <- rec:
		}
	}
	close(out)
}

// AssetBalances is part of idb.IndexerDB
func (db *IndexerDb) AssetBalances(ctx context.Context, abq idb.AssetBalanceQuery) <-chan idb.AssetBalanceRow {
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if abq.AssetID != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.assetid = $%d", partNumber))
		whereArgs = append(whereArgs, abq.AssetID)
		partNumber++
	}
	if abq.AmountGT != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.amount > $%d", partNumber))
		whereArgs = append(whereArgs, abq.AmountGT)
		partNumber++
	}
	if abq.AmountLT != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.amount < $%d", partNumber))
		whereArgs = append(whereArgs, abq.AmountLT)
		partNumber++
	}
	if len(abq.PrevAddress) != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.addr > $%d", partNumber))
		whereArgs = append(whereArgs, abq.PrevAddress)
		partNumber++
	}
	var rows *sql.Rows
	var err error
	query := `SELECT addr, assetid, amount, frozen, created_at, closed_at, deleted FROM account_asset aa`
	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += " ORDER BY addr ASC"
	if abq.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", abq.Limit)
	}
	rows, err = db.db.QueryContext(ctx, query, whereArgs...)
	out := make(chan idb.AssetBalanceRow, 1)
	if err != nil {
		out <- idb.AssetBalanceRow{Error: err}
		close(out)
		return out
	}
	go db.yieldAssetBalanceThread(ctx, rows, out)
	return out
}

func (db *IndexerDb) yieldAssetBalanceThread(ctx context.Context, rows *sql.Rows, out chan<- idb.AssetBalanceRow) {
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
			close(out)
			return
		case out <- rec:
		}
	}
	close(out)
}

// Applications is part of idb.IndexerDB
func (db *IndexerDb) Applications(ctx context.Context, filter *models.SearchForApplicationsParams) <-chan idb.ApplicationRow {
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
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY 1"
	if filter.Limit != nil {
		query += fmt.Sprintf(" LIMIT %d", *filter.Limit)
	}
	out := make(chan idb.ApplicationRow, 1)
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)

	if err != nil {
		out <- idb.ApplicationRow{Error: err}
		close(out)
		return out
	}
	go db.yieldApplicationsThread(ctx, rows, out)
	return out
}

func (db *IndexerDb) yieldApplicationsThread(ctx context.Context, rows *sql.Rows, out chan idb.ApplicationRow) {
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
		err = json.Decode(paramsjson, &ap)
		if err != nil {
			rec.Error = fmt.Errorf("app=%d json err, %v", index, err)
			out <- rec
			break
		}
		rec.Application.Params.ApprovalProgram = ap.ApprovalProgram
		rec.Application.Params.ClearStateProgram = ap.ClearStateProgram
		rec.Application.Params.Creator = new(string)

		var aaddr atypes.Address
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
		out <- rec
	}
	close(out)
}

// Health is part of idb.IndexerDB
func (db *IndexerDb) Health() (idb.Health, error) {
	migrationRequired := false
	migrating := false
	blocking := false
	var data = make(map[string]interface{})

	// If we are not in read-only mode, there will be a migration object.
	if db.migration != nil {
		state := db.migration.GetStatus()

		if state.Err != nil {
			data["migration-error"] = state.Err.Error()
		}
		if state.Status != "" {
			data["migration-status"] = state.Status
		}

		migrationRequired = state.Running
		migrating = state.Running
		blocking = state.Blocking
	} else {
		data["read-only-node"] = true
		state, err := db.getMigrationState()
		if err == nil {
			blocking = migrationStateBlocked(*state)
			migrationRequired = needsMigration(*state)
		}
	}

	data["migration-required"] = migrationRequired

	round, err := db.GetMaxRound()
	return idb.Health{
		Data:        &data,
		Round:       round,
		IsMigrating: migrating,
		DBAvailable: !blocking,
	}, err
}

// GetSpecialAccounts is part of idb.IndexerDB
func (db *IndexerDb) GetSpecialAccounts() (accounts idb.SpecialAccounts, err error) {
	var cache string
	cache, err = db.GetMetastate(specialAccountsMetastateKey)
	if err != nil || cache == "" {
		// Initialize specialAccountsMetastateKey
		var block types.Block
		block, err = db.GetBlock(0)
		if err != nil {
			return idb.SpecialAccounts{}, fmt.Errorf("problem looking up special accounts from genesis block: %v", err)
		}

		accounts = idb.SpecialAccounts{
			FeeSink:     block.FeeSink,
			RewardsPool: block.RewardsPool,
		}

		cache := string(json.Encode(accounts))
		err = db.SetMetastate(specialAccountsMetastateKey, cache)
		if err != nil {
			return idb.SpecialAccounts{}, fmt.Errorf("problem saving metastate: %v", err)
		}

		return
	}

	err = json.Decode([]byte(cache), &accounts)
	if err != nil {
		err = fmt.Errorf("problem decoding cache '%s': %v", cache, err)
	}
	return
}

type postgresFactory struct {
}

func (df postgresFactory) Name() string {
	return "postgres"
}
func (df postgresFactory) Build(arg string, opts *idb.IndexerDbOptions, log *log.Logger) (idb.IndexerDb, error) {
	return OpenPostgres(arg, opts, log)
}

func init() {
	idb.RegisterFactory("postgres", &postgresFactory{})
}
