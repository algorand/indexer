// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package idb

// import text to contstant setup_postgres_sql
//go:generate go run ../cmd/texttosource/main.go idb setup_postgres.sql

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
	models "github.com/algorand/indexer/api/generated/v2"
	_ "github.com/lib/pq"

	"github.com/algorand/indexer/types"
)

func OpenPostgres(connection string, opts *IndexerDbOptions) (pdb *PostgresIndexerDb, err error) {
	db, err := sql.Open("postgres", connection)
	if err != nil {
		return nil, err
	}
	pdb = &PostgresIndexerDb{
		db:         db,
		protoCache: make(map[string]types.ConsensusParams, 20),
	}
	// e.g. a user named "readonly" is in the connection string
	readonly := ((opts != nil) && opts.ReadOnly) || strings.Contains(connection, "readonly")
	if !readonly {
		err = pdb.init()
	}
	return
}

type PostgresIndexerDb struct {
	db *sql.DB

	// state for StartBlock/AddTransaction/CommitBlock
	tx        *sql.Tx
	addtx     *sql.Stmt
	addtxpart *sql.Stmt

	txrows  [][]interface{}
	txprows [][]interface{}

	protoCache map[string]types.ConsensusParams
}

func (db *PostgresIndexerDb) init() (err error) {
	accountingStateJson, _ := db.GetMetastate("state")
	hasAccounting := len(accountingStateJson) > 0
	migrationStateJson, _ := db.GetMetastate("migration")
	hasMigration := len(migrationStateJson) > 0

	if hasMigration || hasAccounting {
		// see postgres_migrations.go
		return db.runAvailableMigrations(migrationStateJson)
	}

	// new database, run setup
	_, err = db.db.Exec(setup_postgres_sql)
	if err != nil {
		return
	}
	err = db.markMigrationsAsDone()
	return
}

func (db *PostgresIndexerDb) AlreadyImported(path string) (imported bool, err error) {
	row := db.db.QueryRow(`SELECT COUNT(path) FROM imported WHERE path = $1`, path)
	numpath := 0
	err = row.Scan(&numpath)
	return numpath == 1, err
}

func (db *PostgresIndexerDb) MarkImported(path string) (err error) {
	_, err = db.db.Exec(`INSERT INTO imported (path) VALUES ($1)`, path)
	return err
}

func (db *PostgresIndexerDb) StartBlock() (err error) {
	db.txrows = make([][]interface{}, 0, 6000)
	db.txprows = make([][]interface{}, 0, 10000)
	return nil
}

func (db *PostgresIndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error {
	txnbytes := msgpack.Encode(txn)
	var jsonbytes []byte
	jsonbytes, err := msgpackToJson(txnbytes)
	if err != nil {
		return err
	}
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

func (db *PostgresIndexerDb) CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
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
			fmt.Printf("txr %#v\n", ntxr)
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
				fmt.Printf("%s %d %d\n", base64.StdEncoding.EncodeToString(er[0].([]byte)), er[1], er[2])
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
func (db *PostgresIndexerDb) GetAsset(assetid uint64) (asset types.AssetParams, err error) {
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
func (db *PostgresIndexerDb) GetDefaultFrozen() (defaultFrozen map[uint64]bool, err error) {
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

func (db *PostgresIndexerDb) LoadGenesis(genesis types.Genesis) (err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback() // ignored if .Commit() first

	setAccount, err := tx.Prepare(`INSERT INTO account (addr, microalgos, rewardsbase, account_data) VALUES ($1, $2, 0, $3)`)
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
		_, err = setAccount.Exec(addr[:], alloc.State.MicroAlgos, string(json.Encode(alloc.State)))
		total += uint64(alloc.State.MicroAlgos)
		if err != nil {
			return fmt.Errorf("error setting genesis account[%d], %v", ai, err)
		}
	}
	err = tx.Commit()
	fmt.Printf("genesis %d accounts %d microalgos, err=%v\n", len(genesis.Allocation), total, err)
	return err

}

func (db *PostgresIndexerDb) SetProto(version string, proto types.ConsensusParams) (err error) {
	pj := json.Encode(proto)
	if err != nil {
		return err
	}
	_, err = db.db.Exec(`INSERT INTO protocol (version, proto) VALUES ($1, $2) ON CONFLICT (version) DO UPDATE SET proto = EXCLUDED.proto`, version, pj)
	return err
}

func (db *PostgresIndexerDb) GetProto(version string) (proto types.ConsensusParams, err error) {
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

func (db *PostgresIndexerDb) GetMetastate(key string) (jsonStrValue string, err error) {
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

func (db *PostgresIndexerDb) SetMetastate(key, jsonStrValue string) (err error) {
	_, err = db.db.Exec(setMetastateUpsert, key, jsonStrValue)
	return
}

func (db *PostgresIndexerDb) GetMaxRound() (round uint64, err error) {
	row := db.db.QueryRow(`SELECT max(round) FROM block_header`)
	err = row.Scan(&round)
	return
}

// Break the read query so that PostgreSQL doesn't get bogged down
// tracking transactional changes to tables.
const txnQueryBatchSize = 20000

var yieldTxnQuery string

func init() {
	yieldTxnQuery = fmt.Sprintf(`SELECT t.round, t.intra, t.txnbytes, t.extra, t.asset, b.realtime FROM txn t JOIN block_header b ON t.round = b.round WHERE t.round > $1 ORDER BY round, intra LIMIT %d`, txnQueryBatchSize)
}

func (db *PostgresIndexerDb) yieldTxnsThread(ctx context.Context, rows *sql.Rows, results chan<- TxnRow) {
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
				var row TxnRow
				row.Error = err
				results <- row
				rows.Close()
				close(results)
				return
			} else {
				rounds[pos] = round
				intras[pos] = intra
				txnbytess[pos] = txnbytes
				extrajsons[pos] = extrajson
				creatableids[pos] = creatableid
				roundtimes[pos] = roundtime
				pos++
			}
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
			var row TxnRow
			row.Round = rounds[i]
			row.RoundTime = roundtimes[i]
			row.Intra = intras[i]
			row.TxnBytes = txnbytess[i]
			row.AssetId = uint64(creatableids[i])
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
				results <- TxnRow{Error: err}
				break
			}
		}
	}
	close(results)
}

func (db *PostgresIndexerDb) YieldTxns(ctx context.Context, prevRound int64) <-chan TxnRow {
	results := make(chan TxnRow, 1)
	rows, err := db.db.QueryContext(ctx, yieldTxnQuery, prevRound)
	if err != nil {
		results <- TxnRow{Error: err}
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

// like go-algorand data/basics/teal.go
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

type TealType uint64
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

type KeyTealValue struct {
	Key []byte    `codec:"k"`
	Tv  TealValue `codec:"v"`
}

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
			if i < last {
				tkv.They[i] = tkv.They[last]
				tkv.They = tkv.They[:last]
				return
			}
		}
	}
}

func (tkv TealKeyValue) MarshalJSON() ([]byte, error) {
	return json.Encode(tkv.They), nil
}
func (tkv *TealKeyValue) UnmarshalJSON(data []byte) error {
	return json.Decode(data, &tkv.They)
}

// like go-algorand data/basics/userBalance.go AppParams{}
type AppParams struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	ApprovalProgram   []byte      `codec:"approv"`
	ClearStateProgram []byte      `codec:"clearp"`
	LocalStateSchema  StateSchema `codec:"lsch"`
	GlobalStateSchema StateSchema `codec:"gsch"`

	GlobalState TealKeyValue `codec:"gs,allocbound=-"`
}

// like go-algorand data/basics/userBalance.go AppLocalState{}
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
func applyKeyValueDelta(state *TealKeyValue, key []byte, vd types.ValueDelta, reverseDelta *AppReverseDelta) (err error) {
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

func (db *PostgresIndexerDb) getDirtyAppLocalState(addr []byte, appIndex int64, dirty []inmemAppLocalState, getq *sql.Stmt) (localstate inmemAppLocalState, err error) {
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
	} else {
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

func (db *PostgresIndexerDb) CommitRoundAccounting(updates RoundUpdates, round, rewardsBase uint64) (err error) {
	any := false
	tx, err := db.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback() // ignored if .Commit() first

	if len(updates.AlgoUpdates) > 0 {
		any = true
		// account_data json is only used on account creation, otherwise the account data jsonb field is updated from the delta
		setalgo, err := tx.Prepare(`INSERT INTO account (addr, microalgos, rewardsbase) VALUES ($1, $2, $3) ON CONFLICT (addr) DO UPDATE SET microalgos = account.microalgos + EXCLUDED.microalgos, rewardsbase = EXCLUDED.rewardsbase`)
		if err != nil {
			return fmt.Errorf("prepare update algo, %v", err)
		}
		defer setalgo.Close()
		for addr, delta := range updates.AlgoUpdates {
			_, err = setalgo.Exec(addr[:], delta, rewardsBase)
			if err != nil {
				return fmt.Errorf("update algo, %v", err)
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
		// TODO: fix according to this comment:
		// if asset is new, set.
		// if new config is empty, set empty. -- handled by AssetDestroys
		// else, update.
		any = true
		setacfg, err := tx.Prepare(`INSERT INTO asset (index, creator_addr, params) VALUES ($1, $2, $3) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params`)
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
			if au.AssetId == debugAsset {
				fmt.Fprintf(os.Stderr, "%d acfg %s %s\n", round, b64(au.Creator[:]), obs(au))
			}
			var outparams string
			if au.IsNew {
				outparams = string(json.Encode(au.Params))
			} else {
				row := getacfg.QueryRow(au.AssetId)
				var paramjson []byte
				err = row.Scan(&paramjson)
				if err != nil {
					return fmt.Errorf("get acfg %d, %v", au.AssetId, err)
				}
				var old atypes.AssetParams
				err = json.Decode(paramjson, &old)
				if err != nil {
					return fmt.Errorf("bad acgf json %d, %v", au.AssetId, err)
				}
				np := types.MergeAssetConfig(old, au.Params)
				outparams = string(json.Encode(np))
			}
			_, err = setacfg.Exec(au.AssetId, au.Creator[:], outparams)
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
			_, err = uta.Exec(tau.AssetId, tau.Round, tau.Offset)
			if err != nil {
				return fmt.Errorf("update txn.asset, %v", err)
			}
		}
		defer uta.Close()
	}
	if len(updates.AssetUpdates) > 0 {
		any = true
		seta, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen) VALUES ($1, $2, $3, $4) ON CONFLICT (addr, assetid) DO UPDATE SET amount = account_asset.amount + EXCLUDED.amount`)
		if err != nil {
			return fmt.Errorf("prepare set account_asset, %v", err)
		}
		defer seta.Close()
		for addr, aulist := range updates.AssetUpdates {
			for _, au := range aulist {
				if au.AssetId == debugAsset {
					fmt.Fprintf(os.Stderr, "%d axfer %s %s\n", round, b64(addr[:]), obs(au))
				}
				if au.Delta.IsInt64() {
					// easy case
					delta := au.Delta.Int64()
					// don't skip delta == 0; mark opt-in
					_, err = seta.Exec(addr[:], au.AssetId, delta, au.DefaultFrozen)
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
						_, err = seta.Exec(addr[:], au.AssetId, step, au.DefaultFrozen)
						if err != nil {
							return fmt.Errorf("update account asset, %v", err)
						}
						au.Delta.Sub(&au.Delta, &mi)
					}
					sign = au.Delta.Sign()
					if sign != 0 {
						_, err = seta.Exec(addr[:], au.AssetId, au.Delta.Int64(), au.DefaultFrozen)
						if err != nil {
							return fmt.Errorf("update account asset, %v", err)
						}
					}
				}
			}
		}
	}
	if len(updates.FreezeUpdates) > 0 {
		any = true
		fr, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen) VALUES ($1, $2, 0, $3) ON CONFLICT (addr, assetid) DO UPDATE SET frozen = EXCLUDED.frozen`)
		if err != nil {
			return fmt.Errorf("prepare asset freeze, %v", err)
		}
		defer fr.Close()
		for _, fs := range updates.FreezeUpdates {
			if fs.AssetId == debugAsset {
				fmt.Fprintf(os.Stderr, "%d %s %s\n", round, b64(fs.Addr[:]), obs(fs))
			}
			_, err = fr.Exec(fs.Addr[:], fs.AssetId, fs.Frozen)
			if err != nil {
				return fmt.Errorf("update asset freeze, %v", err)
			}
		}
	}
	if len(updates.AssetCloses) > 0 {
		any = true
		acc, err := tx.Prepare(`WITH aaamount AS (SELECT ($1)::bigint as round, ($2)::bigint as intra, x.amount FROM account_asset x WHERE x.addr = $3 AND x.assetid = $4)
UPDATE txn ut SET extra = jsonb_set(coalesce(ut.extra, '{}'::jsonb), '{aca}', to_jsonb(aaamount.amount)) FROM aaamount WHERE ut.round = aaamount.round AND ut.intra = aaamount.intra`)
		if err != nil {
			return fmt.Errorf("prepare asset close0, %v", err)
		}
		defer acc.Close()
		acs, err := tx.Prepare(`INSERT INTO account_asset (addr, assetid, amount, frozen)
SELECT $1, $2, x.amount, $3 FROM account_asset x WHERE x.addr = $4 AND x.assetid = $5
ON CONFLICT (addr, assetid) DO UPDATE SET amount = account_asset.amount + EXCLUDED.amount`)
		if err != nil {
			return fmt.Errorf("prepare asset close1, %v", err)
		}
		defer acs.Close()
		acd, err := tx.Prepare(`DELETE FROM account_asset WHERE addr = $1 AND assetid = $2`)
		if err != nil {
			return fmt.Errorf("prepare asset close2, %v", err)
		}
		defer acd.Close()
		for _, ac := range updates.AssetCloses {
			if ac.AssetId == debugAsset {
				fmt.Fprintf(os.Stderr, "%d close %s\n", round, obs(ac))
			}
			_, err = acc.Exec(ac.Round, ac.Offset, ac.Sender[:], ac.AssetId)
			if err != nil {
				return fmt.Errorf("asset close record amount, %v", err)
			}
			_, err = acs.Exec(ac.CloseTo[:], ac.AssetId, ac.DefaultFrozen, ac.Sender[:], ac.AssetId)
			if err != nil {
				return fmt.Errorf("asset close send, %v", err)
			}
			_, err = acd.Exec(ac.Sender[:], ac.AssetId)
			if err != nil {
				return fmt.Errorf("asset close del, %v", err)
			}
		}
	}
	if len(updates.AssetDestroys) > 0 {
		any = true
		// Note! leaves `asset` row present for historical reference, but deletes all holdings from all accounts
		ads, err := tx.Prepare(`DELETE FROM account_asset WHERE assetid = $1`)
		if err != nil {
			return fmt.Errorf("prepare asset destroy, %v", err)
		}
		defer ads.Close()
		aclear, err := tx.Prepare(`UPDATE asset SET params = 'null'::jsonb WHERE index = $1`)
		if err != nil {
			return fmt.Errorf("prepare asset clear, %v", err)
		}
		defer aclear.Close()
		for _, assetId := range updates.AssetDestroys {
			if assetId == debugAsset {
				fmt.Fprintf(os.Stderr, "%d destroy asset %d\n", round, assetId)
			}
			_, err = ads.Exec(assetId)
			if err != nil {
				return fmt.Errorf("asset destroy, %v", err)
			}
			_, err = aclear.Exec(assetId)
			if err != nil {
				return fmt.Errorf("asset destroy, %v", err)
			}
		}
	}
	if len(updates.AppGlobalDeltas) > 0 {
		// apps with dirty global state, collection of AppParams as dict
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
			reverseDelta := AppReverseDelta{

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
		putglobal, err := tx.Prepare(`INSERT INTO app (index, creator, params) VALUES ($1, $2, $3) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params`)
		if err != nil {
			return fmt.Errorf("prepare app global put, %v", err)
		}
		defer putglobal.Close()
		for appid, params := range dirty {
			creator := appCreators[appid]
			paramjson := json.Encode(params)
			_, err = putglobal.Exec(appid, creator, paramjson)
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
					[]interface{}{ald.Address, ald.AppIndex},
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

			var reverseDelta AppReverseDelta

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
			putglobal, err := tx.Prepare(`INSERT INTO account_app (addr, app, localstate) VALUES ($1, $2, $3) ON CONFLICT (addr, app) DO UPDATE SET localstate = EXCLUDED.localstate`)
			if err != nil {
				return fmt.Errorf("prepare app local put, %v", err)
			}
			defer putglobal.Close()
			for _, ld := range dirty {
				_, err = putglobal.Exec(ld.address, ld.appIndex, json.Encode(ld.AppLocalState))
				if err != nil {
					return fmt.Errorf("app local put, %v", err)
				}
			}
		}

		if len(droplocals) > 0 {
			droplocal, err := tx.Prepare(`DELETE FROM account_app WHERE addr = $1 AND app = $2`)
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
		fmt.Printf("empty round %d\n", round)
	}
	var istate ImportState
	staterow := tx.QueryRow(`SELECT v FROM metastate WHERE k = 'state'`)
	var stateJsonStr string
	err = staterow.Scan(&stateJsonStr)
	if err == sql.ErrNoRows {
		// ok
	} else if err != nil {
		return
	} else {
		err = json.Decode([]byte(stateJsonStr), &istate)
		if err != nil {
			return
		}
	}
	istate.AccountRound = int64(round)
	sjs := string(json.Encode(istate))
	_, err = tx.Exec(setMetastateUpsert, "state", sjs)
	if err != nil {
		return
	}
	return tx.Commit()
}

func (db *PostgresIndexerDb) GetBlock(round uint64) (block types.Block, err error) {
	row := db.db.QueryRow(`SELECT header FROM block_header WHERE round = $1`, round)
	var blockheaderjson []byte
	err = row.Scan(&blockheaderjson)
	if err != nil {
		return
	}
	err = json.Decode(blockheaderjson, &block)
	return
}

func buildTransactionQuery(tf TransactionFilter) (query string, whereArgs []interface{}, err error) {
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
			if tf.AddressRole&AddressRoleSender != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'snd' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&AddressRoleReceiver != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'rcv' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&AddressRoleCloseRemainderTo != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'close' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&AddressRoleAssetSender != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'asnd' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&AddressRoleAssetReceiver != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'arcv' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&AddressRoleAssetCloseTo != 0 {
				roleparts = append(roleparts, fmt.Sprintf("t.txn -> 'txn' ->> 'aclose' = $%d", partNumber))
				whereArgs = append(whereArgs, addrBase64)
				partNumber++
			}
			if tf.AddressRole&AddressRoleFreeze != 0 {
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
	if tf.AssetId != 0 || tf.ApplicationId != 0 {
		var creatableId uint64
		if tf.AssetId != 0 {
			creatableId = tf.AssetId
			if tf.ApplicationId != 0 {
				if tf.AssetId == tf.ApplicationId {
					// this is nonsense, but I'll allow it
				} else {
					return "", nil, fmt.Errorf("cannot search both assetid and appid")
				}
			}
		} else {
			creatableId = tf.ApplicationId
		}
		whereParts = append(whereParts, fmt.Sprintf("t.asset = $%d", partNumber))
		whereArgs = append(whereArgs, creatableId)
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

func (db *PostgresIndexerDb) Transactions(ctx context.Context, tf TransactionFilter) <-chan TxnRow {
	out := make(chan TxnRow, 1)
	if len(tf.NextToken) > 0 {
		go db.txnsWithNext(ctx, tf, out)
		return out
	}
	query, whereArgs, err := buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- TxnRow{Error: err}
		close(out)
		return out
	}
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- TxnRow{Error: err}
		close(out)
		return out
	}
	go db.yieldTxnsThreadSimple(ctx, rows, out, true, nil, nil)
	return out
}

func (db *PostgresIndexerDb) txTransactions(tx *sql.Tx, tf TransactionFilter) <-chan TxnRow {
	out := make(chan TxnRow, 1)
	if len(tf.NextToken) > 0 {
		err := fmt.Errorf("txTransactions incompatible with next")
		out <- TxnRow{Error: err}
		close(out)
		return out
	}
	query, whereArgs, err := buildTransactionQuery(tf)
	if err != nil {
		err = fmt.Errorf("txn query err %v", err)
		out <- TxnRow{Error: err}
		close(out)
		return out
	}
	rows, err := tx.Query(query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- TxnRow{Error: err}
		close(out)
		return out
	}
	go db.yieldTxnsThreadSimple(context.Background(), rows, out, true, nil, nil)
	return out
}

func (db *PostgresIndexerDb) txnsWithNext(ctx context.Context, tf TransactionFilter, out chan<- TxnRow) {
	nextround, nextintra32, err := DecodeTxnRowNext(tf.NextToken)
	nextintra := uint64(nextintra32)
	if err != nil {
		out <- TxnRow{Error: err}
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
		out <- TxnRow{Error: err}
		close(out)
		return
	}
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- TxnRow{Error: err}
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
		out <- TxnRow{Error: err}
		close(out)
		return
	}
	rows, err = db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- TxnRow{Error: err}
		close(out)
		return
	}
	db.yieldTxnsThreadSimple(ctx, rows, out, true, nil, nil)
}

func (db *PostgresIndexerDb) yieldTxnsThreadSimple(ctx context.Context, rows *sql.Rows, results chan<- TxnRow, doClose bool, countp *int, errp *error) {
	count := 0
	for rows.Next() {
		var round uint64
		var asset uint64
		var intra int
		var txnbytes []byte
		var extraJson []byte
		var roundtime time.Time
		err := rows.Scan(&round, &intra, &txnbytes, &extraJson, &asset, &roundtime)
		var row TxnRow
		if err != nil {
			row.Error = err
		} else {
			row.Round = round
			row.Intra = intra
			row.TxnBytes = txnbytes
			row.RoundTime = roundtime
			row.AssetId = asset
			if len(extraJson) > 0 {
				err = json.Decode(extraJson, &row.Extra)
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

func (db *PostgresIndexerDb) yieldAccountsThread(ctx context.Context, opts AccountQueryOptions, rows *sql.Rows, tx *sql.Tx, blockheader types.Block, out chan<- AccountRow) {
	defer tx.Rollback()
	count := uint64(0)
	for rows.Next() {
		var addr []byte
		var microalgos uint64
		var rewardsbase uint64
		var keytype *string
		var accountDataJsonStr []byte

		// below are bytes of json serialization

		// holding* are a triplet of lists that should merge together
		var holdingAssetid []byte
		var holdingAmount []byte
		var holdingFrozen []byte

		// assetParams* are a pair of lists that should merge together
		var assetParamsIds []byte
		var assetParamsStr []byte

		// appParam* are a pair of lists that should merge together
		var appParamIndexes []byte // [appId, ...]
		var appParams []byte       // [{AppParams}, ...]

		// localState* are a pair of lists that should merge together
		var localStateAppIds []byte // [appId, ...]
		var localStates []byte      // [{local state}, ...]

		var err error

		if opts.IncludeAssetHoldings {
			if opts.IncludeAssetParams {
				err = rows.Scan(
					&addr, &microalgos, &rewardsbase, &keytype, &accountDataJsonStr,
					&holdingAssetid, &holdingAmount, &holdingFrozen,
					&assetParamsIds, &assetParamsStr,
					&appParamIndexes, &appParams, &localStateAppIds, &localStates,
				)
			} else {
				err = rows.Scan(
					&addr, &microalgos, &rewardsbase, &keytype, &accountDataJsonStr,
					&holdingAssetid, &holdingAmount, &holdingFrozen,
					&appParamIndexes, &appParams, &localStateAppIds, &localStates,
				)
			}
		} else if opts.IncludeAssetParams {
			err = rows.Scan(
				&addr, &microalgos, &rewardsbase, &keytype, &accountDataJsonStr,
				&assetParamsIds, &assetParamsStr,
				&appParamIndexes, &appParams, &localStateAppIds, &localStates,
			)
		} else {
			err = rows.Scan(
				&addr, &microalgos, &rewardsbase, &keytype, &accountDataJsonStr,
				&appParamIndexes, &appParams, &localStateAppIds, &localStates,
			)
		}
		if err != nil {
			out <- AccountRow{Error: err}
			break
		}

		var account models.Account
		var aaddr atypes.Address
		copy(aaddr[:], addr)
		account.Address = aaddr.String()
		account.Round = uint64(blockheader.Round)
		account.AmountWithoutPendingRewards = microalgos
		account.RewardBase = new(uint64)
		*account.RewardBase = rewardsbase
		// default to Offline in there have been no keyreg transactions.
		account.Status = statusStrings[offlineStatusIdx]
		if keytype != nil && *keytype != "" {
			account.SigType = keytype
		}

		if accountDataJsonStr != nil {
			var ad types.AccountData
			err = json.Decode(accountDataJsonStr, &ad)
			if err != nil {
				out <- AccountRow{Error: err}
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
			proto, err := db.GetProto(string(blockheader.CurrentProtocol))
			if err != nil {
				out <- AccountRow{Error: err}
				break
			}
			rewardsUnits := uint64(0)
			if proto.RewardUnit != 0 {
				rewardsUnits = microalgos / proto.RewardUnit
			}
			rewardsDelta := blockheader.RewardsLevel - rewardsbase
			account.PendingRewards = rewardsUnits * rewardsDelta
		}
		account.Amount = microalgos + account.PendingRewards
		// not implemented: account.Rewards sum of all rewards ever

		const nullarraystr = "[null]"
		reject := opts.HasAssetId != 0

		if len(holdingAssetid) > 0 && string(holdingAssetid) != nullarraystr {
			var haids []uint64
			err = json.Decode(holdingAssetid, &haids)
			if err != nil {
				out <- AccountRow{Error: err}
				break
			}
			var hamounts []uint64
			err = json.Decode(holdingAmount, &hamounts)
			if err != nil {
				out <- AccountRow{Error: err}
				break
			}
			var hfrozen []bool
			err = json.Decode(holdingFrozen, &hfrozen)
			if err != nil {
				out <- AccountRow{Error: err}
				break
			}
			av := make([]models.AssetHolding, 0, len(haids))
			for i, assetid := range haids {
				// SQL can result in cross-product duplication when account has bothe asset holdings and assets created, de-dup here
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
				if assetid == opts.HasAssetId {
					if opts.AssetGT != 0 {
						if hamounts[i] > opts.AssetGT {
							reject = false
						}
					} else if opts.AssetLT != 0 {
						if hamounts[i] < opts.AssetLT {
							reject = false
						}
					} else {
						reject = false
					}
				}
				tah := models.AssetHolding{Amount: hamounts[i], IsFrozen: hfrozen[i], AssetId: assetid} // TODO: set Creator to asset creator addr string
				av = append(av, tah)
			}
			account.Assets = new([]models.AssetHolding)
			*account.Assets = av
		}
		if reject {
			continue
		}
		if len(assetParamsIds) > 0 && string(assetParamsIds) != nullarraystr {
			var assetids []uint64
			err = json.Decode(assetParamsIds, &assetids)
			if err != nil {
				out <- AccountRow{Error: err}
				break
			}
			var assetParams []types.AssetParams
			err = json.Decode(assetParamsStr, &assetParams)
			if err != nil {
				out <- AccountRow{Error: err}
				break
			}
			cal := make([]models.Asset, 0, len(assetids))
			for i, assetid := range assetids {
				// SQL can result in cross-product duplication when account has bothe asset holdings and assets created, de-dup here
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
				if ap == (types.AssetParams{}) {
					// remnant of deleted asset
					continue
				}
				tma := models.Asset{
					Index: assetid,
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
				out <- AccountRow{Error: err}
				break
			}
			var apps []AppParams
			err = json.Decode(appParams, &apps)
			if err != nil {
				err = fmt.Errorf("parsing json appparams, %v", err)
				out <- AccountRow{Error: err}
				break
			}
			if len(appIds) != len(apps) {
				err = fmt.Errorf("account app unpacking got %d appids but %d apps", len(appIds), len(apps))
				out <- AccountRow{Error: err}
				break
			}

			aout := make([]models.Application, len(appIds))
			outpos := 0
			for i, appid := range appIds {
				if apps[i].ApprovalProgram == nil && apps[i].ClearStateProgram == nil {
					// app was deleted, but a record remains that it _did_ exist, skip it here
					continue
				}
				aout[outpos].Id = appid
				aout[outpos].Params.ApprovalProgram = apps[i].ApprovalProgram
				aout[outpos].Params.ClearStateProgram = apps[i].ClearStateProgram
				aout[outpos].Params.Creator = &account.Address
				aout[outpos].Params.GlobalState = apps[i].GlobalState.toModel()
				aout[outpos].Params.GlobalStateSchema = &models.ApplicationStateSchema{
					NumByteSlice: apps[i].GlobalStateSchema.NumByteSlice,
					NumUint:      apps[i].GlobalStateSchema.NumUint,
				}
				aout[outpos].Params.LocalStateSchema = &models.ApplicationStateSchema{
					NumByteSlice: apps[i].LocalStateSchema.NumByteSlice,
					NumUint:      apps[i].LocalStateSchema.NumUint,
				}
				outpos++
			}
			if outpos != len(aout) {
				aout = aout[:outpos]
			}
			account.CreatedApps = &aout
		}

		reject = opts.HasAppId != 0
		if len(localStateAppIds) > 0 {
			var appIds []uint64
			err = json.Decode(localStateAppIds, &appIds)
			if err != nil {
				err = fmt.Errorf("parsing json local appids, %v", err)
				out <- AccountRow{Error: err}
				break
			}
			var ls []AppLocalState
			err = json.Decode(localStates, &ls)
			if err != nil {
				err = fmt.Errorf("parsing json local states, %v", err)
				out <- AccountRow{Error: err}
				break
			}
			if len(appIds) != len(ls) {
				err = fmt.Errorf("account app unpacking got %d appids but %d appls", len(appIds), len(ls))
				out <- AccountRow{Error: err}
				break
			}
			aout := make([]models.ApplicationLocalState, len(appIds))
			for i, appid := range appIds {
				aout[i].Id = appid
				aout[i].Schema = models.ApplicationStateSchema{
					NumByteSlice: ls[i].Schema.NumByteSlice,
					NumUint:      ls[i].Schema.NumUint,
				}
				aout[i].KeyValue = ls[i].KeyValue.toModel()
				if appid == opts.HasAppId {
					reject = false
				}
			}
			account.AppsLocalState = &aout
		}
		if reject {
			continue
		}

		select {
		case out <- AccountRow{Account: account}:
			count++
			if count >= opts.Limit {
				close(out)
				return
			}
		case <-ctx.Done():
			close(out)
			return
		}
	}
	close(out)
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

func (db *PostgresIndexerDb) GetAccounts(ctx context.Context, opts AccountQueryOptions) <-chan AccountRow {
	out := make(chan AccountRow, 1)

	if opts.HasAssetId != 0 {
		opts.IncludeAssetHoldings = true
	} else if (opts.AssetGT != 0) || (opts.AssetLT != 0) {
		err := fmt.Errorf("AssetGT=%d, AssetLT=%d, but HasAssetId=%d", opts.AssetGT, opts.AssetLT, opts.HasAssetId)
		out <- AccountRow{Error: err}
		close(out)
		return out
	}

	// Begin transaction so we get everything at one consistent point in time and round of accounting.
	tx, err := db.db.BeginTx(ctx, &readOnlyTx)
	if err != nil {
		err = fmt.Errorf("account tx err %v", err)
		out <- AccountRow{Error: err}
		close(out)
		return out
	}

	// Get round number through which accounting has been updated
	row := tx.QueryRow(`SELECT (v -> 'account_round')::bigint as account_round FROM metastate WHERE k = 'state'`)
	var accountRound uint64
	err = row.Scan(&accountRound)
	if err != nil {
		err = fmt.Errorf("account_round err %v", err)
		out <- AccountRow{Error: err}
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
		out <- AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out
	}
	var blockheader types.Block
	err = json.Decode(headerjson, &blockheader)
	if err != nil {
		err = fmt.Errorf("account round header %d err %v", accountRound, err)
		out <- AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out
	}

	// Construct query for fetching accounts...
	query, whereArgs := db.buildAccountQuery(opts)
	rows, err := tx.Query(query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("account query %#v err %v", query, err)
		out <- AccountRow{Error: err}
		close(out)
		tx.Rollback()
		return out
	}
	go db.yieldAccountsThread(ctx, opts, rows, tx, blockheader, out)
	return out
}

func (db *PostgresIndexerDb) buildAccountQuery(opts AccountQueryOptions) (query string, whereArgs []interface{}) {
	// Construct query for fetching accounts...
	query = `SELECT a.addr, a.microalgos, a.rewardsbase, a.keytype, a.account_data FROM account a`
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs = make([]interface{}, 0, maxWhereParts)
	partNumber := 1
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
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	query += " ORDER BY a.addr ASC"
	if opts.Limit != 0 && opts.HasAssetId == 0 && opts.HasAppId == 0 {
		// sql limit gets disabled when we filter client side
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	// TODO: asset holdings and asset params are optional, but practically always used. Either make them actually always on, or make app-global and app-local clauses also optional (they are currently always on).
	query = "WITH qaccounts AS (" + query + ")"
	if opts.IncludeAssetHoldings {
		query += `, qaa AS (SELECT xa.addr, json_agg(aa.assetid) as haid, json_agg(aa.amount) as hamt, json_agg(aa.frozen) as hf FROM account_asset aa JOIN qaccounts xa ON aa.addr = xa.addr GROUP BY 1)`
	}
	if opts.IncludeAssetParams {
		query += `, qap AS (SELECT ya.addr, json_agg(ap.index) as paid, json_agg(ap.params) as pp FROM asset ap JOIN qaccounts ya ON ap.creator_addr = ya.addr GROUP BY 1)`
	}
	query += `, qapp AS (SELECT app.creator as addr, json_agg(app.index) as papps, json_agg(app.params) as ppa FROM app JOIN qaccounts ON qaccounts.addr = app.creator GROUP BY 1), qls AS (SELECT la.addr, json_agg(la.app) as lsapps, json_agg(la.localstate) as lsls FROM account_app la JOIN qaccounts ON qaccounts.addr = la.addr GROUP BY 1)`
	query += ` SELECT za.addr, za.microalgos, za.rewardsbase, za.keytype, za.account_data`
	if opts.IncludeAssetHoldings {
		query += `, qaa.haid, qaa.hamt, qaa.hf`
	}
	if opts.IncludeAssetParams {
		query += `, qap.paid, qap.pp`
	}
	query += `, qapp.papps, qapp.ppa, qls.lsapps, qls.lsls FROM qaccounts za`
	if opts.IncludeAssetHoldings {
		query += ` LEFT JOIN qaa ON za.addr = qaa.addr`
	}
	if opts.IncludeAssetParams {
		query += ` LEFT JOIN qap ON za.addr = qap.addr`
	}
	query += " LEFT JOIN qapp ON za.addr = qapp.addr LEFT JOIN qls ON qls.addr = za.addr ORDER BY za.addr ASC;"
	return query, whereArgs
}

func (db *PostgresIndexerDb) Assets(ctx context.Context, filter AssetsQuery) <-chan AssetRow {
	query := `SELECT index, creator_addr, params FROM asset a`
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if filter.AssetId != 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.index = $%d", partNumber))
		whereArgs = append(whereArgs, filter.AssetId)
		partNumber++
	}
	if filter.AssetIdGreaterThan != 0 {
		whereParts = append(whereParts, fmt.Sprintf("a.index > $%d", partNumber))
		whereArgs = append(whereArgs, filter.AssetIdGreaterThan)
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
	out := make(chan AssetRow, 1)
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("asset query %#v err %v", query, err)
		out <- AssetRow{Error: err}
		close(out)
		return out
	}
	go db.yieldAssetsThread(ctx, filter, rows, out)
	return out
}

func (db *PostgresIndexerDb) yieldAssetsThread(ctx context.Context, filter AssetsQuery, rows *sql.Rows, out chan<- AssetRow) {
	for rows.Next() {
		var index uint64
		var creator_addr []byte
		var paramsJsonStr []byte
		var err error

		err = rows.Scan(&index, &creator_addr, &paramsJsonStr)
		if err != nil {
			out <- AssetRow{Error: err}
			break
		}
		var params types.AssetParams
		err = json.Decode(paramsJsonStr, &params)
		if err != nil {
			out <- AssetRow{Error: err}
			break
		}
		rec := AssetRow{
			AssetId: index,
			Creator: creator_addr,
			Params:  params,
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

func (db *PostgresIndexerDb) AssetBalances(ctx context.Context, abq AssetBalanceQuery) <-chan AssetBalanceRow {
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
	partNumber := 1
	if abq.AssetId != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.assetid = $%d", partNumber))
		whereArgs = append(whereArgs, abq.AssetId)
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
	query := `SELECT addr, assetid, amount, frozen FROM account_asset aa`
	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += " ORDER BY addr ASC"
	if abq.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", abq.Limit)
	}
	rows, err = db.db.QueryContext(ctx, query, whereArgs...)
	out := make(chan AssetBalanceRow, 1)
	if err != nil {
		out <- AssetBalanceRow{Error: err}
		close(out)
		return out
	}
	go db.yieldAssetBalanceThread(ctx, rows, out)
	return out
}

func (db *PostgresIndexerDb) yieldAssetBalanceThread(ctx context.Context, rows *sql.Rows, out chan<- AssetBalanceRow) {
	for rows.Next() {
		var addr []byte
		var assetId uint64
		var amount uint64
		var frozen bool
		err := rows.Scan(&addr, &assetId, &amount, &frozen)
		if err != nil {
			out <- AssetBalanceRow{Error: err}
			break
		}
		rec := AssetBalanceRow{
			Address: addr,
			AssetId: assetId,
			Amount:  amount,
			Frozen:  frozen,
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

func (db *PostgresIndexerDb) Applications(ctx context.Context, filter *models.SearchForApplicationsParams) <-chan ApplicationRow {
	query := `SELECT index, creator, params FROM app `

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
	out := make(chan ApplicationRow, 1)
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)

	if err != nil {
		out <- ApplicationRow{Error: err}
		close(out)
		return out
	}
	go db.yieldApplicationsThread(ctx, rows, out)
	return out
}

func (db *PostgresIndexerDb) yieldApplicationsThread(ctx context.Context, rows *sql.Rows, out chan ApplicationRow) {
	for rows.Next() {
		var index int64
		var creator []byte
		var paramsjson []byte
		err := rows.Scan(&index, &creator, &paramsjson)
		if err != nil {
			out <- ApplicationRow{Error: err}
			break
		}
		var rec ApplicationRow
		rec.Application.Id = uint64(index)
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

type postgresFactory struct {
}

func (df postgresFactory) Name() string {
	return "postgres"
}
func (df postgresFactory) Build(arg string, opts *IndexerDbOptions) (IndexerDb, error) {
	return OpenPostgres(arg, opts)
}

func init() {
	indexerFactories = append(indexerFactories, &postgresFactory{})
}

type ImportState struct {
	AccountRound int64 `codec:"account_round"`
}

func ParseImportState(js string) (istate ImportState, err error) {
	err = json.Decode([]byte(js), &istate)
	return
}
