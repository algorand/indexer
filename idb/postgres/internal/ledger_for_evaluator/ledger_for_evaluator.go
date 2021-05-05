package postgres

import (
	"database/sql"
	"fmt"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
)

const blockHeaderQuery = "SELECT header FROM block_header WHERE round = $1"
const assetCreatorQuery = "SELECT creator_addr FROM asset " +
	"WHERE index = $1 AND NOT deleted"
const appCreatorQuery = "SELECT creator FROM app WHERE index = $1 AND NOT deleted"
const accountQuery = "SELECT microalgos, rewardsbase, rewards_total, account_data " +
	"FROM account WHERE addr = $1 AND NOT deleted"
const assetHoldingsQuery = "SELECT assetid, amount, frozen FROM account_asset " +
	"WHERE addr = $1 AND NOT deleted"
const assetParamsQuery = "SELECT index, params FROM asset " +
	"WHERE creator_addr = $1 AND NOT deleted"
const appParamsQuery = "SELECT index, params FROM app WHERE creator = $1 AND NOT deleted"
const appLocalStatesQuery = "SELECT app, localstate FROM account_app " +
	"WHERE addr = $1 AND NOT deleted"

// Implements `ledgerForEvaluator` interface from go-algorand and is used for accounting.
type LedgerForEvaluator struct {
	tx          *sql.Tx
	genesisHash crypto.Digest
	// Indexer currently does not store the balance of the rewards pool account, but
	// go-algorand's eval checks that it satisfies the minimum balance. We thus return
	// a fake amount. TODO: remove.
	rewardsPoolAddress basics.Address

	blockHeaderStmt    *sql.Stmt
	assetCreatorStmt   *sql.Stmt
	appCreatorStmt     *sql.Stmt
	accountStmt        *sql.Stmt
	assetHoldingsStmt  *sql.Stmt
	assetParamsStmt    *sql.Stmt
	appParamsStmt      *sql.Stmt
	appLocalStatesStmt *sql.Stmt
}

func MakeLedgerForEvaluator(tx *sql.Tx, genesisHash crypto.Digest, rewardsPoolAddress basics.Address) (LedgerForEvaluator, error) {
	l := LedgerForEvaluator{
		tx:                 tx,
		genesisHash:        genesisHash,
		rewardsPoolAddress: rewardsPoolAddress,
	}

	var err error

	l.blockHeaderStmt, err = tx.Prepare(blockHeaderQuery)
	if err != nil {
		return LedgerForEvaluator{},
			fmt.Errorf("MakeLedgerForEvaluator(): prepare block header stmt err: %w", err)
	}
	l.assetCreatorStmt, err = tx.Prepare(assetCreatorQuery)
	if err != nil {
		return LedgerForEvaluator{},
			fmt.Errorf("MakeLedgerForEvaluator(): prepare asset creator stmt err: %w", err)
	}
	l.appCreatorStmt, err = tx.Prepare(appCreatorQuery)
	if err != nil {
		return LedgerForEvaluator{},
			fmt.Errorf("MakeLedgerForEvaluator(): prepare app creator stmt err: %w", err)
	}
	l.accountStmt, err = tx.Prepare(accountQuery)
	if err != nil {
		return LedgerForEvaluator{},
			fmt.Errorf("MakeLedgerForEvaluator(): prepare account stmt err: %w", err)
	}
	l.assetHoldingsStmt, err = tx.Prepare(assetHoldingsQuery)
	if err != nil {
		return LedgerForEvaluator{},
			fmt.Errorf("MakeLedgerForEvaluator(): prepare asset holdings stmt err: %w", err)
	}
	l.assetParamsStmt, err = tx.Prepare(assetParamsQuery)
	if err != nil {
		return LedgerForEvaluator{},
			fmt.Errorf("MakeLedgerForEvaluator(): prepare asset params stmt err: %w", err)
	}
	l.appParamsStmt, err = tx.Prepare(appParamsQuery)
	if err != nil {
		return LedgerForEvaluator{},
			fmt.Errorf("MakeLedgerForEvaluator(): prepare app params stmt err: %w", err)
	}
	l.appLocalStatesStmt, err = tx.Prepare(appLocalStatesQuery)
	if err != nil {
		return LedgerForEvaluator{},
			fmt.Errorf("MakeLedgerForEvaluator(): prepare app local states stmt err: %w", err)
	}

	return l, nil
}

func (l *LedgerForEvaluator) Close() {
	l.blockHeaderStmt.Close()
	l.assetCreatorStmt.Close()
	l.appCreatorStmt.Close()
	l.accountStmt.Close()
	l.assetHoldingsStmt.Close()
	l.assetParamsStmt.Close()
	l.appParamsStmt.Close()
	l.appLocalStatesStmt.Close()
}

func (l LedgerForEvaluator) BlockHdr(round basics.Round) (bookkeeping.BlockHeader, error) {
	row := l.blockHeaderStmt.QueryRow(uint64(round))

	var header []byte
	err := row.Scan(&header)
	if err != nil {
		return bookkeeping.BlockHeader{}, fmt.Errorf("BlockHdr() scan row err: %w", err)
	}

	res, err := encoding.DecodeBlockHeader(header)
	if err != nil {
		return bookkeeping.BlockHeader{}, fmt.Errorf("BlockHdr() decode header err: %w", err)
	}

	return res, nil
}

func (l LedgerForEvaluator) CheckDup(config.ConsensusParams, basics.Round, basics.Round, basics.Round, transactions.Txid, ledger.TxLease) error {
	// This function is not used by evaluator.
	return nil
}

func (l *LedgerForEvaluator) readAccountTable(address basics.Address) (basics.AccountData, bool /*exists*/, error) {
	row := l.accountStmt.QueryRow(address[:])

	var microalgos uint64
	var rewardsbase uint64
	var rewardsTotal uint64
	var accountData []byte

	err := row.Scan(&microalgos, &rewardsbase, &rewardsTotal, &accountData)
	if err == sql.ErrNoRows {
		return basics.AccountData{}, false, nil
	}
	if err != nil {
		return basics.AccountData{}, false, fmt.Errorf("readAccountTable() scan row err: %w", err)
	}

	res := basics.AccountData{}
	if accountData != nil {
		res, err = encoding.DecodeAccountData(accountData)
		if err != nil {
			return basics.AccountData{}, false,
				fmt.Errorf("readAccountTable() decode account data err: %w", err)
		}
	}

	res.MicroAlgos = basics.MicroAlgos{Raw: microalgos}
	res.RewardsBase = rewardsbase
	res.RewardedMicroAlgos = basics.MicroAlgos{Raw: rewardsTotal}

	return res, true, nil
}

func (l *LedgerForEvaluator) readAccountAssetTable(address basics.Address) (map[basics.AssetIndex]basics.AssetHolding, error) {
	rows, err := l.assetHoldingsStmt.Query(address[:])
	if err != nil {
		return nil, fmt.Errorf("readAccountAssetTable() query err: %w", err)
	}

	res := make(map[basics.AssetIndex]basics.AssetHolding)

	var assetid uint64
	var amount uint64
	var frozen bool

	for rows.Next() {
		err = rows.Scan(&assetid, &amount, &frozen)
		if err != nil {
			return nil, fmt.Errorf("readAccountAssetTable() scan row err: %w", err)
		}

		res[basics.AssetIndex(assetid)] = basics.AssetHolding{
			Amount: amount,
			Frozen: frozen,
		}
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("readAccountAssetTable() scan end err: %w", err)
	}

	return res, nil
}

func (l *LedgerForEvaluator) readAssetTable(address basics.Address) (map[basics.AssetIndex]basics.AssetParams, error) {
	rows, err := l.assetParamsStmt.Query(address[:])
	if err != nil {
		return nil, fmt.Errorf("readAssetTable() query err: %w", err)
	}

	res := make(map[basics.AssetIndex]basics.AssetParams)

	var index uint64
	var params []byte

	for rows.Next() {
		err = rows.Scan(&index, &params)
		if err != nil {
			return nil, fmt.Errorf("readAssetTable() scan row err: %w", err)
		}

		res[basics.AssetIndex(index)], err = encoding.DecodeAssetParams(params)
		if err != nil {
			return nil, fmt.Errorf("readAssetTable() decode params err: %w", err)
		}
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("readAssetTable() scan end err: %w", err)
	}

	return res, nil
}

func (l *LedgerForEvaluator) readAppTable(address basics.Address) (map[basics.AppIndex]basics.AppParams, error) {
	rows, err := l.appParamsStmt.Query(address[:])
	if err != nil {
		return nil, fmt.Errorf("readAppTable() query err: %w", err)
	}

	res := make(map[basics.AppIndex]basics.AppParams)

	var index uint64
	var params []byte

	for rows.Next() {
		err = rows.Scan(&index, &params)
		if err != nil {
			return nil, fmt.Errorf("readAppTable() scan row err: %w", err)
		}

		res[basics.AppIndex(index)], err = encoding.DecodeAppParams(params)
		if err != nil {
			return nil, fmt.Errorf("readAppTable() decode params err: %w", err)
		}
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("readAppTable() scan end err: %w", err)
	}

	return res, nil
}

func (l *LedgerForEvaluator) readAccountAppTable(address basics.Address) (map[basics.AppIndex]basics.AppLocalState, error) {
	rows, err := l.appLocalStatesStmt.Query(address[:])
	if err != nil {
		return nil, fmt.Errorf("readAccountAppTable() query err: %w", err)
	}

	res := make(map[basics.AppIndex]basics.AppLocalState)

	var app uint64
	var localstate []byte

	for rows.Next() {
		err = rows.Scan(&app, &localstate)
		if err != nil {
			return nil, fmt.Errorf("readAccountAppTable() scan row err: %w", err)
		}

		res[basics.AppIndex(app)], err = encoding.DecodeAppLocalState(localstate)
		if err != nil {
			return nil, fmt.Errorf("readAccountAppTable() decode local state err: %w", err)
		}
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("readAccountAppTable() scan end err: %w", err)
	}

	return res, nil
}

func (l LedgerForEvaluator) LookupWithoutRewards(round basics.Round, address basics.Address) (basics.AccountData, basics.Round, error) {
	// The rewards pool balance must pass the minimum balance check in go-algorand's
	// eval(), so return a sufficiently large balance.
	if address == l.rewardsPoolAddress {
		var balance uint64 = 1000 * 1000 * 1000 * 1000 * 1000
		accountData := basics.AccountData{
			MicroAlgos: basics.MicroAlgos{Raw: balance},
		}
		return accountData, round, nil
	}

	accountData, exists, err := l.readAccountTable(address)
	if err != nil {
		return basics.AccountData{}, basics.Round(0), err
	}
	if !exists {
		return basics.AccountData{}, round, nil
	}

	accountData.Assets, err = l.readAccountAssetTable(address)
	if err != nil {
		return basics.AccountData{}, basics.Round(0), err
	}

	accountData.AssetParams, err = l.readAssetTable(address)
	if err != nil {
		return basics.AccountData{}, basics.Round(0), err
	}

	accountData.AppParams, err = l.readAppTable(address)
	if err != nil {
		return basics.AccountData{}, basics.Round(0), err
	}

	accountData.AppLocalStates, err = l.readAccountAppTable(address)
	if err != nil {
		return basics.AccountData{}, basics.Round(0), err
	}

	return accountData, round, nil
}

func (l LedgerForEvaluator) GetCreatorForRound(_ basics.Round, cindex basics.CreatableIndex, ctype basics.CreatableType) (basics.Address, bool, error) {
	var row *sql.Row

	switch ctype {
	case basics.AssetCreatable:
		row = l.assetCreatorStmt.QueryRow(uint64(cindex))
	case basics.AppCreatable:
		row = l.appCreatorStmt.QueryRow(uint64(cindex))
	default:
		panic("unknown creatable type")
	}

	var buf []byte
	err := row.Scan(&buf)
	if err == sql.ErrNoRows {
		return basics.Address{}, false, nil
	}
	if err != nil {
		return basics.Address{}, false, nil
	}

	var address basics.Address
	copy(address[:], buf)

	return address, true, nil
}

func (l LedgerForEvaluator) GenesisHash() crypto.Digest {
	return l.genesisHash
}

func (l LedgerForEvaluator) Totals(round basics.Round) (ledgercore.AccountTotals, error) {
	// The evaluator uses totals only for recomputing the rewards pool balance. Indexer
	// does not currently compute the this balance, and we can returns an empty struct
	// here.
	return ledgercore.AccountTotals{}, nil
}

func (l LedgerForEvaluator) CompactCertVoters(basics.Round) (*ledger.VotersForRound, error) {
	// This function is not used by evaluator.
	return nil, nil
}
