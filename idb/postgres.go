// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of the Algorand Indexer
//
// Algorand Indexer is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// Algorand Indexer is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with Algorand Indexer.  If not, see <https://www.gnu.org/licenses/>.

// You can build without postgres by `go build --tags nopostgres` but it's on by default
// +build !nopostgres

package idb

// import text to contstant setup_postgres_sql
//go:generate go run ../cmd/texttosource/main.go idb setup_postgres.sql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/client/algod/models"
	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"
	_ "github.com/lib/pq"

	"github.com/algorand/indexer/types"
)

func OpenPostgres(connection string) (pdb *PostgresIndexerDb, err error) {
	db, err := sql.Open("postgres", connection)
	if err != nil {
		return nil, err
	}
	pdb = &PostgresIndexerDb{
		db:         db,
		protoCache: make(map[string]types.ConsensusParams, 20),
	}
	// e.g. a user named "readonly" is in the connection string
	if !strings.Contains(connection, "readonly") {
		err = pdb.init()
	}
	return
}

type PostgresIndexerDb struct {
	db *sql.DB
	tx *sql.Tx

	protoCache map[string]types.ConsensusParams
}

func (db *PostgresIndexerDb) init() (err error) {
	_, err = db.db.Exec(setup_postgres_sql)

	// TODO: Schema-migration/Upgrade. Select upgrade state from database and compare to code, apply upgrades from code to database state.
	// upgradeJson, err := db.GetMetastate("upgrade-state")
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
	db.tx, err = db.db.BeginTx(context.Background(), nil)
	return
}

func (db *PostgresIndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error {
	var err error
	txnbytes := msgpack.Encode(txn)
	txid := crypto.TransactionIDString(txn.Txn)
	_, err = db.tx.Exec(`INSERT INTO txn (round, intra, typeenum, asset, txid, txnbytes, txn) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`, round, intra, txtypeenum, assetid, txid[:], txnbytes, string(json.Encode(txn)))
	if err != nil {
		return err
	}
	stmt, err := db.tx.Prepare(`INSERT INTO txn_participation (addr, round, intra) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}
	for _, paddr := range participation {
		_, err = stmt.Exec(paddr, round, intra)
		if err != nil {
			return err
		}
	}
	return err
}
func (db *PostgresIndexerDb) CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
	var err error
	var block types.Block
	err = msgpack.Decode(headerbytes, &block)
	if err != nil {
		return err
	}
	headerjson := json.Encode(block)
	_, err = db.tx.Exec(`INSERT INTO block_header (round, realtime, rewardslevel, header) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, round, time.Unix(timestamp, 0), rewardslevel, string(headerjson))
	if err != nil {
		return err
	}
	err = db.tx.Commit()
	db.tx = nil
	return err
}

func (db *PostgresIndexerDb) GetBlockHeader(round uint64) (block types.Block, err error) {
	row := db.db.QueryRow(`SELECT header FROM block_header WHERE round = $1`, round)
	var blockbytes []byte
	err = row.Scan(&blockbytes)
	if err != nil {
		return
	}
	err = json.Decode(blockbytes, &block)
	return
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
	fmt.Printf("genesis %d accounts %d microalgos, %v\n", len(genesis.Allocation), total, err)
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
	return
}

func (db *PostgresIndexerDb) SetMetastate(key, jsonStrValue string) (err error) {
	_, err = db.db.Exec(`INSERT INTO metastate (k, v) VALUES ($1, $2) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`, key, jsonStrValue)
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
	yieldTxnQuery = fmt.Sprintf(`SELECT t.round, t.intra, t.txnbytes, t.extra, b.realtime FROM txn t JOIN block_header b ON t.round = b.round WHERE t.round > $1 ORDER BY round, intra LIMIT %d`, txnQueryBatchSize)
}

func (db *PostgresIndexerDb) yieldTxnsThread(ctx context.Context, rows *sql.Rows, results chan<- TxnRow) {
	keepGoing := true
	for keepGoing {
		keepGoing = false
		rounds := make([]uint64, txnQueryBatchSize)
		intras := make([]int, txnQueryBatchSize)
		txnbytess := make([][]byte, txnQueryBatchSize)
		extrajsons := make([][]byte, txnQueryBatchSize)
		roundtimes := make([]time.Time, txnQueryBatchSize)
		pos := 0
		// read from db
		for rows.Next() {
			var round uint64
			var intra int
			var txnbytes []byte
			var extrajson []byte
			var roundtime time.Time
			err := rows.Scan(&round, &intra, &txnbytes, &extrajson, &roundtime)
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
		fmt.Fprintf(os.Stderr, "got batch of %d txns round %d-%d\n", pos, rounds[0], rounds[pos-1])
		// yield to chan
		for i := 0; i < pos; i++ {
			var row TxnRow
			row.Round = rounds[i]
			row.RoundTime = roundtimes[i]
			row.Intra = intras[i]
			row.TxnBytes = txnbytess[i]
			if len(extrajsons[i]) > 0 {
				json.Decode(extrajsons[i], &row.Extra)
			}
			select {
			case <-ctx.Done():
				break
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

// this gets overlaid onto account.account_data jsonb, replacing just these fields, same as types.AccountData
type AccountDataKeyregPart struct {
	Status byte `codec:"onl"`

	VoteID      types.OneTimeSignatureVerifier `codec:"vote"`
	SelectionID types.VRFVerifier              `codec:"sel"`

	VoteFirstValid  types.Round `codec:"voteFst"`
	VoteLastValid   types.Round `codec:"voteLst"`
	VoteKeyDilution uint64      `codec:"voteKD"`
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
	if len(updates.KeyregUpdates) > 0 {
		any = true
		setkeyreg, err := tx.Prepare(`UPDATE account SET account_data = coalesce(account_data, '{}'::jsonb) || ($1)::jsonb WHERE addr = $2`)
		if err != nil {
			return fmt.Errorf("prepare keyreg, %v", err)
		}
		defer setkeyreg.Close()
		for _, kr := range updates.KeyregUpdates {
			part := AccountDataKeyregPart{
				Status:          byte(kr.Status),
				VoteID:          kr.VoteID,
				SelectionID:     kr.SelectionID,
				VoteFirstValid:  types.Round(kr.VoteFirstValid),
				VoteLastValid:   types.Round(kr.VoteLastValid),
				VoteKeyDilution: kr.VoteKeyDilution,
			}
			jb := json.Encode(part)
			_, err = setkeyreg.Exec(jb, kr.Addr[:])
			if err != nil {
				return fmt.Errorf("update keyreg, %v", err)
			}
		}
	}
	if len(updates.AcfgUpdates) > 0 {
		any = true
		setacfg, err := tx.Prepare(`INSERT INTO asset (index, creator_addr, params) VALUES ($1, $2, $3) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params`)
		if err != nil {
			return fmt.Errorf("prepare set asset, %v", err)
		}
		defer setacfg.Close()
		for _, au := range updates.AcfgUpdates {
			if au.AssetId == debugAsset {
				fmt.Fprintf(os.Stderr, "%d acfg %s %s\n", round, b64(au.Creator[:]), obs(au))
			}
			_, err = setacfg.Exec(au.AssetId, au.Creator[:], string(json.Encode(au.Params)))
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
				if au.Delta == 0 {
					continue
				}
				if au.AssetId == debugAsset {
					fmt.Fprintf(os.Stderr, "%d axfer %s %s\n", round, b64(addr[:]), obs(au))
				}
				_, err = seta.Exec(addr[:], au.AssetId, au.Delta, au.DefaultFrozen)
				if err != nil {
					return fmt.Errorf("update account asset, %v", err)
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
SELECT $1, $2, x.amount, $3 FROM account_asset x WHERE x.addr = $4
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
			_, err = acs.Exec(ac.CloseTo[:], ac.AssetId, ac.DefaultFrozen, ac.Sender[:])
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
		for _, assetId := range updates.AssetDestroys {
			if assetId == debugAsset {
				fmt.Fprintf(os.Stderr, "%d destroy asset %d\n", round, assetId)
			}
			ads.Exec(assetId)
			if err != nil {
				return fmt.Errorf("asset destroy, %v", err)
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
	_, err = tx.Exec(`INSERT INTO metastate (k, v) VALUES ('state', $1) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`, sjs)
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

func (db *PostgresIndexerDb) Transactions(ctx context.Context, tf TransactionFilter) <-chan TxnRow {
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
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
	if tf.AssetId != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.asset = $%d", partNumber))
		whereArgs = append(whereArgs, tf.AssetId)
		partNumber++
	}
	if tf.MinAssetAmount != 0 {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'aamt')::bigint >= $%d", partNumber))
		whereArgs = append(whereArgs, tf.MinAssetAmount)
		partNumber++
	}
	if tf.MaxAssetAmount != 0 {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'aamt')::bigint <= $%d", partNumber))
		whereArgs = append(whereArgs, tf.MaxAssetAmount)
		partNumber++
	}
	if tf.TypeEnum != 0 {
		whereParts = append(whereParts, fmt.Sprintf("t.typeenum = $%d", partNumber))
		whereArgs = append(whereArgs, tf.TypeEnum)
		partNumber++
	}
	if tf.Txid != nil {
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
	if tf.MinAlgos != 0 {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'amt')::bigint >= $%d", partNumber))
		whereArgs = append(whereArgs, tf.MinAlgos)
		partNumber++
	}
	if tf.MaxAlgos != 0 {
		whereParts = append(whereParts, fmt.Sprintf("(t.txn -> 'txn' -> 'amt')::bigint <= $%d", partNumber))
		whereArgs = append(whereArgs, tf.MaxAlgos)
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
	query := "SELECT t.round, t.intra, t.txnbytes, t.extra, h.realtime FROM txn t JOIN block_header h ON t.round = h.round"
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
	out := make(chan TxnRow, 1)
	rows, err := db.db.QueryContext(ctx, query, whereArgs...)
	if err != nil {
		err = fmt.Errorf("txn query %#v err %v", query, err)
		out <- TxnRow{Error: err}
		close(out)
		return out
	}
	go db.yieldTxnsThreadSimple(ctx, rows, out)
	return out
}

func (db *PostgresIndexerDb) yieldTxnsThreadSimple(ctx context.Context, rows *sql.Rows, results chan<- TxnRow) {
	for rows.Next() {
		var round uint64
		var intra int
		var txnbytes []byte
		var extraJson []byte
		var roundtime time.Time
		err := rows.Scan(&round, &intra, &txnbytes, &extraJson, &roundtime)
		var row TxnRow
		if err != nil {
			row.Error = err
		} else {
			row.Round = round
			row.Intra = intra
			row.TxnBytes = txnbytes
			row.RoundTime = roundtime
			if len(extraJson) > 0 {
				json.Decode(extraJson, &row.Extra)
			}
		}
		select {
		case <-ctx.Done():
			break
		case results <- row:
			if err != nil {
				break
			}
		}
	}
	close(results)
}

const maxAccountsLimit = 1000

func (db *PostgresIndexerDb) yieldAccountsThread(ctx context.Context, opts AccountQueryOptions, rows *sql.Rows, tx *sql.Tx, blockheader types.Block, out chan<- AccountRow) {
	defer tx.Rollback()
	for rows.Next() {
		var addr []byte
		var microalgos uint64
		var rewardsbase uint64
		var accountDataJsonStr *string

		// these are bytes of json serialization
		var holdingAssetid []byte
		var holdingAmount []byte
		var holdingFrozen []byte

		// these are bytes of json serialization
		var assetParamsIds []byte
		var assetParamsStr []byte

		var err error

		if opts.IncludeAssetHoldings {
			if opts.IncludeAssetParams {
				err = rows.Scan(
					&addr, &microalgos, &rewardsbase, &accountDataJsonStr,
					&holdingAssetid, &holdingAmount, &holdingFrozen,
					&assetParamsIds, &assetParamsStr,
				)
			} else {
				err = rows.Scan(
					&addr, &microalgos, &rewardsbase, &accountDataJsonStr,
					&holdingAssetid, &holdingAmount, &holdingFrozen,
				)
			}
		} else if opts.IncludeAssetParams {
			err = rows.Scan(
				&addr, &microalgos, &rewardsbase, &accountDataJsonStr,
				&assetParamsIds, &assetParamsStr,
			)
		} else {
			err = rows.Scan(&addr, &microalgos, &rewardsbase, &accountDataJsonStr)
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

		// TODO: pending rewards calculation doesn't belong in database layer (this is just the most covenient place which has all the data)
		proto, err := db.GetProto(string(blockheader.CurrentProtocol))
		rewardsUnits := uint64(0)
		if proto.RewardUnit != 0 {
			rewardsUnits = microalgos / proto.RewardUnit
		}
		rewardsDelta := blockheader.RewardsLevel - rewardsbase
		account.PendingRewards = rewardsUnits * rewardsDelta
		account.Amount = microalgos + account.PendingRewards
		// not implemented: account.Rewards sum of all rewards ever

		const nullarraystr = "[null]"

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
			account.Assets = make(map[uint64]models.AssetHolding, len(haids))
			for i, assetid := range haids {
				account.Assets[assetid] = models.AssetHolding{Amount: hamounts[i], Frozen: hfrozen[i]}
			}
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
			account.AssetParams = make(map[uint64]models.AssetParams, len(assetids))
			for i, assetid := range assetids {
				ap := assetParams[i]
				account.AssetParams[assetid] = models.AssetParams{
					Creator:       account.Address,
					Total:         ap.Total,
					Decimals:      ap.Decimals,
					DefaultFrozen: ap.DefaultFrozen,
					UnitName:      ap.UnitName,
					AssetName:     ap.AssetName,
					URL:           ap.URL,
					MetadataHash:  ap.MetadataHash[:],
					ManagerAddr:   addrStr(ap.Manager[:]),
					ReserveAddr:   addrStr(ap.Reserve[:]),
					FreezeAddr:    addrStr(ap.Freeze[:]),
					ClawbackAddr:  addrStr(ap.Clawback[:]),
				}

			}
		}
		select {
		case out <- AccountRow{Account: account}:
		case <-ctx.Done():
			break
		}
	}
	close(out)
}

func addrStr(addr []byte) string {
	if len(addr) == 0 {
		return ""
	}
	allZero := true
	for _, bv := range addr {
		if bv != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return ""
	}
	var aa atypes.Address
	copy(aa[:], addr)
	return aa.String()
}

func (db *PostgresIndexerDb) GetAccounts(ctx context.Context, opts AccountQueryOptions) <-chan AccountRow {
	out := make(chan AccountRow, 1)

	// Begin transaction so we get everything at one consistent point in time and round of accounting.
	tx, err := db.db.BeginTx(ctx, nil)
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
	query := `SELECT a.addr, a.microalgos, a.rewardsbase, a.account_data`
	if opts.IncludeAssetHoldings {
		query += `, json_agg(aa.assetid) as haid, json_agg(aa.amount) as hamt, json_agg(aa.frozen) as hf`
	}
	if opts.IncludeAssetParams {
		query += `, json_agg(ap.index) as paid, json_agg(ap.params) as pp`
	}
	query += ` FROM account a`
	if opts.IncludeAssetHoldings {
		query += ` LEFT JOIN account_asset aa ON a.addr = aa.addr`
	}
	if opts.IncludeAssetParams {
		query += ` LEFT JOIN asset ap ON a.addr = ap.creator_addr`
	}
	const maxWhereParts = 14
	whereParts := make([]string, 0, maxWhereParts)
	whereArgs := make([]interface{}, 0, maxWhereParts)
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
	if len(whereParts) > 0 {
		whereStr := strings.Join(whereParts, " AND ")
		query += " WHERE " + whereStr
	}
	if opts.IncludeAssetHoldings || opts.IncludeAssetParams {
		query += " GROUP BY 1,2,3,4"
	}
	if opts.Limit != 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
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
		whereParts = append(whereParts, fmt.Sprintf("a.params ->> 'an' = $%d", partNumber))
		whereArgs = append(whereArgs, filter.Name)
		partNumber++
	}
	if filter.Unit != "" {
		whereParts = append(whereParts, fmt.Sprintf("a.params ->> 'un' = $%d", partNumber))
		whereArgs = append(whereArgs, filter.Unit)
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
			break
		case out <- rec:
			break
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
	if abq.MinAmount != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.amount >= $%d", partNumber))
		whereArgs = append(whereArgs, abq.MinAmount)
		partNumber++
	}
	if abq.MaxAmount != 0 {
		whereParts = append(whereParts, fmt.Sprintf("aa.amount <= $%d", partNumber))
		whereArgs = append(whereArgs, abq.MaxAmount)
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
			break
		case out <- rec:
			continue
		}
	}
	close(out)
}

type postgresFactory struct {
}

func (df postgresFactory) Name() string {
	return "postgres"
}
func (df postgresFactory) Build(arg string) (IndexerDb, error) {
	return OpenPostgres(arg)
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
