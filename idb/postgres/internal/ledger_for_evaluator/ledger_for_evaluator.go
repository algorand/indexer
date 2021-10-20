package ledgerforevaluator

import (
	"context"
	"fmt"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/jackc/pgx/v4"

	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/idb/postgres/internal/schema"
)

const (
	blockHeaderStmtName    = "block_header"
	assetCreatorStmtName   = "asset_creator"
	appCreatorStmtName     = "app_creator"
	accountStmtName        = "account"
	assetHoldingsStmtName  = "asset_holdings"
	assetParamsStmtName    = "asset_params"
	appParamsStmtName      = "app_params"
	appLocalStatesStmtName = "app_local_states"
	accountTotalsStmtName  = "account_totals"
)

var statements = map[string]string{
	blockHeaderStmtName: "SELECT header FROM block_header WHERE round = $1",
	assetCreatorStmtName: "SELECT creator_addr FROM asset " +
		"WHERE index = $1 AND NOT deleted",
	appCreatorStmtName: "SELECT creator FROM app WHERE index = $1 AND NOT deleted",
	accountStmtName: "SELECT microalgos, rewardsbase, rewards_total, account_data " +
		"FROM account WHERE addr = $1 AND NOT deleted",
	assetHoldingsStmtName: "SELECT assetid, amount, frozen FROM account_asset " +
		"WHERE addr = $1 AND NOT deleted",
	assetParamsStmtName: "SELECT index, params FROM asset " +
		"WHERE creator_addr = $1 AND NOT deleted",
	appParamsStmtName: "SELECT index, params FROM app WHERE creator = $1 AND NOT deleted",
	appLocalStatesStmtName: "SELECT app, localstate FROM account_app " +
		"WHERE addr = $1 AND NOT deleted",
	accountTotalsStmtName: `SELECT v FROM metastate WHERE k = '` +
		schema.AccountTotals + `'`,
}

// LedgerForEvaluator implements the indexerLedgerForEval interface from
// go-algorand ledger/eval.go and is used for accounting.
type LedgerForEvaluator struct {
	tx          pgx.Tx
	latestRound basics.Round
}

// MakeLedgerForEvaluator creates a LedgerForEvaluator object.
func MakeLedgerForEvaluator(tx pgx.Tx, latestRound basics.Round) (LedgerForEvaluator, error) {
	l := LedgerForEvaluator{
		tx:          tx,
		latestRound: latestRound,
	}

	for name, query := range statements {
		_, err := tx.Prepare(context.Background(), name, query)
		if err != nil {
			return LedgerForEvaluator{},
				fmt.Errorf("MakeLedgerForEvaluator() prepare statement err: %w", err)
		}
	}

	return l, nil
}

// Close shuts down LedgerForEvaluator.
func (l *LedgerForEvaluator) Close() {
	for name := range statements {
		l.tx.Conn().Deallocate(context.Background(), name)
	}
}

// LatestBlockHdr is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LatestBlockHdr() (bookkeeping.BlockHeader, error) {
	row := l.tx.QueryRow(context.Background(), blockHeaderStmtName, uint64(l.latestRound))

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

func (l *LedgerForEvaluator) parseAccountTable(row pgx.Row) (basics.AccountData, bool /*exists*/, error) {
	var microalgos uint64
	var rewardsbase uint64
	var rewardsTotal uint64
	var accountData []byte

	err := row.Scan(&microalgos, &rewardsbase, &rewardsTotal, &accountData)
	if err == pgx.ErrNoRows {
		return basics.AccountData{}, false, nil
	}
	if err != nil {
		return basics.AccountData{}, false, fmt.Errorf("parseAccountTable() scan row err: %w", err)
	}

	res := basics.AccountData{}
	if accountData != nil {
		res, err = encoding.DecodeTrimmedAccountData(accountData)
		if err != nil {
			return basics.AccountData{}, false,
				fmt.Errorf("parseAccountTable() decode account data err: %w", err)
		}
	}

	res.MicroAlgos = basics.MicroAlgos{Raw: microalgos}
	res.RewardsBase = rewardsbase
	res.RewardedMicroAlgos = basics.MicroAlgos{Raw: rewardsTotal}

	return res, true, nil
}

func (l *LedgerForEvaluator) parseAccountAssetTable(rows pgx.Rows) (map[basics.AssetIndex]basics.AssetHolding, error) {
	defer rows.Close()
	var res map[basics.AssetIndex]basics.AssetHolding

	var assetid uint64
	var amount uint64
	var frozen bool

	for rows.Next() {
		err := rows.Scan(&assetid, &amount, &frozen)
		if err != nil {
			return nil, fmt.Errorf("parseAccountAssetTable() scan row err: %w", err)
		}

		if res == nil {
			res = make(map[basics.AssetIndex]basics.AssetHolding)
		}
		res[basics.AssetIndex(assetid)] = basics.AssetHolding{
			Amount: amount,
			Frozen: frozen,
		}
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("parseAccountAssetTable() scan end err: %w", err)
	}

	return res, nil
}

func (l *LedgerForEvaluator) parseAssetTable(rows pgx.Rows) (map[basics.AssetIndex]basics.AssetParams, error) {
	defer rows.Close()
	var res map[basics.AssetIndex]basics.AssetParams

	var index uint64
	var params []byte

	for rows.Next() {
		err := rows.Scan(&index, &params)
		if err != nil {
			return nil, fmt.Errorf("parseAssetTable() scan row err: %w", err)
		}

		if res == nil {
			res = make(map[basics.AssetIndex]basics.AssetParams)
		}
		res[basics.AssetIndex(index)], err = encoding.DecodeAssetParams(params)
		if err != nil {
			return nil, fmt.Errorf("parseAssetTable() decode params err: %w", err)
		}
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("parseAssetTable() scan end err: %w", err)
	}

	return res, nil
}

func (l *LedgerForEvaluator) parseAppTable(rows pgx.Rows) (map[basics.AppIndex]basics.AppParams, error) {
	defer rows.Close()
	var res map[basics.AppIndex]basics.AppParams

	var index uint64
	var params []byte

	for rows.Next() {
		err := rows.Scan(&index, &params)
		if err != nil {
			return nil, fmt.Errorf("parseAppTable() scan row err: %w", err)
		}

		if res == nil {
			res = make(map[basics.AppIndex]basics.AppParams)
		}
		res[basics.AppIndex(index)], err = encoding.DecodeAppParams(params)
		if err != nil {
			return nil, fmt.Errorf("parseAppTable() decode params err: %w", err)
		}
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("parseAppTable() scan end err: %w", err)
	}

	return res, nil
}

func (l *LedgerForEvaluator) parseAccountAppTable(rows pgx.Rows) (map[basics.AppIndex]basics.AppLocalState, error) {
	defer rows.Close()
	var res map[basics.AppIndex]basics.AppLocalState

	var app uint64
	var localstate []byte

	for rows.Next() {
		err := rows.Scan(&app, &localstate)
		if err != nil {
			return nil, fmt.Errorf("parseAccountAppTable() scan row err: %w", err)
		}

		if res == nil {
			res = make(map[basics.AppIndex]basics.AppLocalState)
		}
		res[basics.AppIndex(app)], err = encoding.DecodeAppLocalState(localstate)
		if err != nil {
			return nil, fmt.Errorf("parseAccountAppTable() decode local state err: %w", err)
		}
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("parseAccountAppTable() scan end err: %w", err)
	}

	return res, nil
}

// Load rows from the account table for the given addresses except the special accounts.
// nil is stored for those accounts that were not found. Uses batching.
func (l *LedgerForEvaluator) loadAccountTable(addresses map[basics.Address]struct{}) (map[basics.Address]*basics.AccountData, error) {
	addressesArr := make([]basics.Address, 0, len(addresses))
	for address := range addresses {
		addressesArr = append(addressesArr, address)
	}

	var batch pgx.Batch
	for i := range addressesArr {
		batch.Queue(accountStmtName, addressesArr[i][:])
	}

	results := l.tx.SendBatch(context.Background(), &batch)
	defer results.Close()

	res := make(map[basics.Address]*basics.AccountData, len(addresses))
	for _, address := range addressesArr {
		row := results.QueryRow()

		accountData := new(basics.AccountData)
		var exists bool
		var err error

		*accountData, exists, err = l.parseAccountTable(row)
		if err != nil {
			return nil, fmt.Errorf("loadAccountTable() err: %w", err)
		}

		if exists {
			res[address] = accountData
		} else {
			res[address] = nil
		}
	}

	err := results.Close()
	if err != nil {
		return nil, fmt.Errorf("loadAccountTable() close results err: %w", err)
	}

	return res, nil
}

// Load all creatables for the non-nil account data from the provided map into that
// account data. Uses batching.
func (l *LedgerForEvaluator) loadCreatables(accountDataMap *map[basics.Address]*basics.AccountData) error {
	var batch pgx.Batch

	existingAddresses := make([]basics.Address, 0, len(*accountDataMap))
	for address, accountData := range *accountDataMap {
		if accountData != nil {
			existingAddresses = append(existingAddresses, address)
		}
	}

	for i := range existingAddresses {
		batch.Queue(assetHoldingsStmtName, existingAddresses[i][:])
	}
	for i := range existingAddresses {
		batch.Queue(assetParamsStmtName, existingAddresses[i][:])
	}
	for i := range existingAddresses {
		batch.Queue(appParamsStmtName, existingAddresses[i][:])
	}
	for i := range existingAddresses {
		batch.Queue(appLocalStatesStmtName, existingAddresses[i][:])
	}

	results := l.tx.SendBatch(context.Background(), &batch)
	defer results.Close()

	for _, address := range existingAddresses {
		rows, err := results.Query()
		if err != nil {
			return fmt.Errorf("loadCreatables() query asset holdings err: %w", err)
		}
		(*accountDataMap)[address].Assets, err = l.parseAccountAssetTable(rows)
		if err != nil {
			return fmt.Errorf("loadCreatables() err: %w", err)
		}
	}
	for _, address := range existingAddresses {
		rows, err := results.Query()
		if err != nil {
			return fmt.Errorf("loadCreatables() query asset params err: %w", err)
		}
		(*accountDataMap)[address].AssetParams, err = l.parseAssetTable(rows)
		if err != nil {
			return fmt.Errorf("loadCreatables() err: %w", err)
		}
	}
	for _, address := range existingAddresses {
		rows, err := results.Query()
		if err != nil {
			return fmt.Errorf("loadCreatables() query app params err: %w", err)
		}
		(*accountDataMap)[address].AppParams, err = l.parseAppTable(rows)
		if err != nil {
			return fmt.Errorf("loadCreatables() err: %w", err)
		}
	}
	for _, address := range existingAddresses {
		rows, err := results.Query()
		if err != nil {
			return fmt.Errorf("loadCreatables() query app local states err: %w", err)
		}
		(*accountDataMap)[address].AppLocalStates, err = l.parseAccountAppTable(rows)
		if err != nil {
			return fmt.Errorf("loadCreatables() err: %w", err)
		}
	}

	err := results.Close()
	if err != nil {
		return fmt.Errorf("loadCreatables() close results err: %w", err)
	}

	return nil
}

// LookupWithoutRewards is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LookupWithoutRewards(addresses map[basics.Address]struct{}) (map[basics.Address]*basics.AccountData, error) {
	res, err := l.loadAccountTable(addresses)
	if err != nil {
		return nil, fmt.Errorf("loadAccounts() err: %w", err)
	}

	err = l.loadCreatables(&res)
	if err != nil {
		return nil, fmt.Errorf("loadAccounts() err: %w", err)
	}

	return res, nil
}

func (l *LedgerForEvaluator) parseAddress(row pgx.Row) (basics.Address, bool /*exists*/, error) {
	var buf []byte
	err := row.Scan(&buf)
	if err == pgx.ErrNoRows {
		return basics.Address{}, false, nil
	}
	if err != nil {
		return basics.Address{}, false, fmt.Errorf("parseAddress() err: %w", err)
	}

	var address basics.Address
	copy(address[:], buf)

	return address, true, nil
}

// GetAssetCreator is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) GetAssetCreator(indices map[basics.AssetIndex]struct{}) (map[basics.AssetIndex]ledger.FoundAddress, error) {
	indicesArr := make([]basics.AssetIndex, 0, len(indices))
	for index := range indices {
		indicesArr = append(indicesArr, index)
	}

	var batch pgx.Batch
	for _, index := range indicesArr {
		batch.Queue(assetCreatorStmtName, uint64(index))
	}

	results := l.tx.SendBatch(context.Background(), &batch)
	res := make(map[basics.AssetIndex]ledger.FoundAddress, len(indices))
	for _, index := range indicesArr {
		row := results.QueryRow()

		address, exists, err := l.parseAddress(row)
		if err != nil {
			return nil, fmt.Errorf("GetAssetCreator() err: %w", err)
		}

		res[index] = ledger.FoundAddress{Address: address, Exists: exists}
	}
	results.Close()

	return res, nil
}

// GetAppCreator is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) GetAppCreator(indices map[basics.AppIndex]struct{}) (map[basics.AppIndex]ledger.FoundAddress, error) {
	indicesArr := make([]basics.AppIndex, 0, len(indices))
	for index := range indices {
		indicesArr = append(indicesArr, index)
	}

	var batch pgx.Batch
	for _, index := range indicesArr {
		batch.Queue(appCreatorStmtName, uint64(index))
	}

	results := l.tx.SendBatch(context.Background(), &batch)
	res := make(map[basics.AppIndex]ledger.FoundAddress, len(indices))
	for _, index := range indicesArr {
		row := results.QueryRow()

		address, exists, err := l.parseAddress(row)
		if err != nil {
			return nil, fmt.Errorf("GetAppCreator() err: %w", err)
		}

		res[index] = ledger.FoundAddress{Address: address, Exists: exists}
	}
	results.Close()

	return res, nil
}

// LatestTotals is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LatestTotals() (ledgercore.AccountTotals, error) {
	row := l.tx.QueryRow(context.Background(), accountTotalsStmtName)

	var json string
	err := row.Scan(&json)
	if err != nil {
		return ledgercore.AccountTotals{}, fmt.Errorf("LatestTotals() scan err: %w", err)
	}

	totals, err := encoding.DecodeAccountTotals([]byte(json))
	if err != nil {
		return ledgercore.AccountTotals{}, fmt.Errorf("LatestTotals() decode err: %w", err)
	}

	return totals, nil
}
