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
	blockHeaderStmtName   = "block_header"
	assetCreatorStmtName  = "asset_creator"
	appCreatorStmtName    = "app_creator"
	accountStmtName       = "account"
	assetHoldingStmtName  = "asset_holding"
	assetParamsStmtName   = "asset_params"
	appParamsStmtName     = "app_params"
	appLocalStateStmtName = "app_local_state"
	accountTotalsStmtName = "account_totals"
)

var statements = map[string]string{
	blockHeaderStmtName: "SELECT header FROM block_header WHERE round = $1",
	assetCreatorStmtName: "SELECT creator_addr FROM asset " +
		"WHERE index = $1 AND NOT deleted",
	appCreatorStmtName: "SELECT creator FROM app WHERE index = $1 AND NOT deleted",
	accountStmtName: "SELECT microalgos, rewardsbase, rewards_total, account_data " +
		"FROM account WHERE addr = $1 AND NOT deleted",
	assetHoldingStmtName: "SELECT amount, frozen FROM account_asset " +
		"WHERE addr = $1 AND assetid = $2 AND NOT deleted",
	assetParamsStmtName: "SELECT creator_addr, params FROM asset " +
		"WHERE index = $1 AND NOT deleted",
	appParamsStmtName: "SELECT creator, params FROM app WHERE index = $1 AND NOT deleted",
	appLocalStateStmtName: "SELECT localstate FROM account_app " +
		"WHERE addr = $1 AND app = $2 AND NOT deleted",
	accountTotalsStmtName: `SELECT v FROM metastate WHERE k = '` +
		schema.AccountTotals + `'`,
}

// DeprecatedLedgerForEvaluator implements the indexerLedgerForEval interface from
// go-algorand ledger/eval.go and is used for accounting.
type DeprecatedLedgerForEvaluator struct {
	tx          pgx.Tx
	latestRound basics.Round
}

// MakeDeprecatedLedgerForEvaluator creates a LedgerForEvaluator object.
func MakeDeprecatedLedgerForEvaluator(tx pgx.Tx, latestRound basics.Round) (DeprecatedLedgerForEvaluator, error) {
	l := DeprecatedLedgerForEvaluator{
		tx:          tx,
		latestRound: latestRound,
	}

	for name, query := range statements {
		_, err := tx.Prepare(context.Background(), name, query)
		if err != nil {
			return DeprecatedLedgerForEvaluator{},
				fmt.Errorf("MakeLedgerForEvaluator() prepare statement err: %w", err)
		}
	}

	return l, nil
}

// Close shuts down LedgerForEvaluator.
func (l *DeprecatedLedgerForEvaluator) Close() {
	for name := range statements {
		l.tx.Conn().Deallocate(context.Background(), name)
	}
}

// LatestBlockHdr is part of go-algorand's indexerLedgerForEval interface.
func (l DeprecatedLedgerForEvaluator) LatestBlockHdr() (bookkeeping.BlockHeader, error) {
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

func (l *DeprecatedLedgerForEvaluator) parseAccountTable(row pgx.Row) (ledgercore.AccountData, bool /*exists*/, error) {
	var microalgos uint64
	var rewardsbase uint64
	var rewardsTotal uint64
	var accountData []byte

	err := row.Scan(&microalgos, &rewardsbase, &rewardsTotal, &accountData)
	if err == pgx.ErrNoRows {
		return ledgercore.AccountData{}, false, nil
	}
	if err != nil {
		return ledgercore.AccountData{}, false, fmt.Errorf("parseAccountTable() scan row err: %w", err)
	}

	var res ledgercore.AccountData
	if accountData != nil {
		res, err = encoding.DecodeTrimmedLcAccountData(accountData)
		if err != nil {
			return ledgercore.AccountData{}, false,
				fmt.Errorf("parseAccountTable() decode account data err: %w", err)
		}
	}

	res.MicroAlgos = basics.MicroAlgos{Raw: microalgos}
	res.RewardsBase = rewardsbase
	res.RewardedMicroAlgos = basics.MicroAlgos{Raw: rewardsTotal}

	return res, true, nil
}

// LookupWithoutRewards is part of go-algorand's indexerLedgerForEval interface.
func (l DeprecatedLedgerForEvaluator) LookupWithoutRewards(addresses map[basics.Address]struct{}) (map[basics.Address]*ledgercore.AccountData, error) {
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

	res := make(map[basics.Address]*ledgercore.AccountData, len(addresses))
	for _, address := range addressesArr {
		row := results.QueryRow()

		lcAccountData := new(ledgercore.AccountData)
		var exists bool
		var err error

		*lcAccountData, exists, err = l.parseAccountTable(row)
		if err != nil {
			return nil, fmt.Errorf("LookupWithoutRewards() err: %w", err)
		}

		if exists {
			res[address] = lcAccountData
		} else {
			res[address] = nil
		}
	}

	err := results.Close()
	if err != nil {
		return nil, fmt.Errorf("LookupWithoutRewards() close results err: %w", err)
	}

	return res, nil
}

func (l *DeprecatedLedgerForEvaluator) parseAccountAssetTable(row pgx.Row) (basics.AssetHolding, bool /*exists*/, error) {
	var amount uint64
	var frozen bool

	err := row.Scan(&amount, &frozen)
	if err == pgx.ErrNoRows {
		return basics.AssetHolding{}, false, nil
	}
	if err != nil {
		return basics.AssetHolding{}, false,
			fmt.Errorf("parseAccountAssetTable() scan row err: %w", err)
	}

	assetHolding := basics.AssetHolding{
		Amount: amount,
		Frozen: frozen,
	}
	return assetHolding, true, nil
}

func (l *DeprecatedLedgerForEvaluator) parseAssetTable(row pgx.Row) (basics.Address /*creator*/, basics.AssetParams, bool /*exists*/, error) {
	var creatorAddr []byte
	var params []byte

	err := row.Scan(&creatorAddr, &params)
	if err == pgx.ErrNoRows {
		return basics.Address{}, basics.AssetParams{}, false, nil
	}
	if err != nil {
		return basics.Address{}, basics.AssetParams{}, false,
			fmt.Errorf("parseAssetTable() scan row err: %w", err)
	}

	var creator basics.Address
	copy(creator[:], creatorAddr)

	assetParams, err := encoding.DecodeAssetParams(params)
	if err != nil {
		return basics.Address{}, basics.AssetParams{}, false,
			fmt.Errorf("parseAssetTable() decode params err: %w", err)
	}

	return creator, assetParams, true, nil
}

func (l *DeprecatedLedgerForEvaluator) parseAppTable(row pgx.Row) (basics.Address /*creator*/, basics.AppParams, bool /*exists*/, error) {
	var creatorAddr []byte
	var params []byte

	err := row.Scan(&creatorAddr, &params)
	if err == pgx.ErrNoRows {
		return basics.Address{}, basics.AppParams{}, false, nil
	}
	if err != nil {
		return basics.Address{}, basics.AppParams{}, false,
			fmt.Errorf("parseAppTable() scan row err: %w", err)
	}

	var creator basics.Address
	copy(creator[:], creatorAddr)

	appParams, err := encoding.DecodeAppParams(params)
	if err != nil {
		return basics.Address{}, basics.AppParams{}, false,
			fmt.Errorf("parseAppTable() decode params err: %w", err)
	}

	return creator, appParams, true, nil
}

func (l *DeprecatedLedgerForEvaluator) parseAccountAppTable(row pgx.Row) (basics.AppLocalState, bool /*exists*/, error) {
	var localstate []byte

	err := row.Scan(&localstate)
	if err == pgx.ErrNoRows {
		return basics.AppLocalState{}, false, nil
	}
	if err != nil {
		return basics.AppLocalState{}, false,
			fmt.Errorf("parseAccountAppTable() scan row err: %w", err)
	}

	appLocalState, err := encoding.DecodeAppLocalState(localstate)
	if err != nil {
		return basics.AppLocalState{}, false,
			fmt.Errorf("parseAccountAppTable() decode local state err: %w", err)
	}

	return appLocalState, true, nil
}

// LookupResources is part of go-algorand's indexerLedgerForEval interface.
func (l DeprecatedLedgerForEvaluator) LookupResources(input map[basics.Address]map[ledger.Creatable]struct{}) (map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource, error) {
	// Create request arrays since iterating over maps is non-deterministic.
	type AddrID struct {
		addr basics.Address
		id   basics.CreatableIndex
	}
	// Asset holdings to request.
	assetHoldingsReq := make([]AddrID, 0, len(input))
	// Asset params to request.
	assetParamsReq := make([]basics.CreatableIndex, 0, len(input))
	// For each asset id, record for which addresses it was requested.
	assetParamsToAddresses := make(map[basics.CreatableIndex]map[basics.Address]struct{})
	// App local states to request.
	appLocalStatesReq := make([]AddrID, 0, len(input))
	// App params to request.
	appParamsReq := make([]basics.CreatableIndex, 0, len(input))
	// For each app id, record for which addresses it was requested.
	appParamsToAddresses := make(map[basics.CreatableIndex]map[basics.Address]struct{})

	for address, creatables := range input {
		for creatable := range creatables {
			switch creatable.Type {
			case basics.AssetCreatable:
				assetHoldingsReq = append(assetHoldingsReq, AddrID{address, creatable.Index})
				if addresses, ok := assetParamsToAddresses[creatable.Index]; ok {
					addresses[address] = struct{}{}
				} else {
					assetParamsReq = append(assetParamsReq, creatable.Index)
					addresses = make(map[basics.Address]struct{})
					addresses[address] = struct{}{}
					assetParamsToAddresses[creatable.Index] = addresses
				}
			case basics.AppCreatable:
				appLocalStatesReq = append(appLocalStatesReq, AddrID{address, creatable.Index})
				if addresses, ok := appParamsToAddresses[creatable.Index]; ok {
					addresses[address] = struct{}{}
				} else {
					appParamsReq = append(appParamsReq, creatable.Index)
					addresses = make(map[basics.Address]struct{})
					addresses[address] = struct{}{}
					appParamsToAddresses[creatable.Index] = addresses
				}
			default:
				return nil, fmt.Errorf(
					"LookupResources() unknown creatable type %d", creatable.Type)
			}
		}
	}

	// Prepare a batch of sql queries.
	var batch pgx.Batch
	for i := range assetHoldingsReq {
		batch.Queue(
			assetHoldingStmtName, assetHoldingsReq[i].addr[:], assetHoldingsReq[i].id)
	}
	for _, cidx := range assetParamsReq {
		batch.Queue(assetParamsStmtName, cidx)
	}
	for _, cidx := range appParamsReq {
		batch.Queue(appParamsStmtName, cidx)
	}
	for i := range appLocalStatesReq {
		batch.Queue(
			appLocalStateStmtName, appLocalStatesReq[i].addr[:], appLocalStatesReq[i].id)
	}

	// Execute the sql queries.
	results := l.tx.SendBatch(context.Background(), &batch)
	defer results.Close()

	// Initialize the result `res` with the same structure as `input`.
	res := make(
		map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource, len(input))
	for address, creatables := range input {
		creatablesOutput :=
			make(map[ledger.Creatable]ledgercore.AccountResource, len(creatables))
		res[address] = creatablesOutput
		for creatable := range creatables {
			creatablesOutput[creatable] = ledgercore.AccountResource{}
		}
	}

	// Parse sql query results in the same order the queries were made.
	for _, addrID := range assetHoldingsReq {
		row := results.QueryRow()
		assetHolding, exists, err := l.parseAccountAssetTable(row)
		if err != nil {
			return nil, fmt.Errorf("LookupResources() err: %w", err)
		}
		if exists {
			creatable := ledger.Creatable{
				Index: addrID.id,
				Type:  basics.AssetCreatable,
			}
			resource := res[addrID.addr][creatable]
			resource.AssetHolding = new(basics.AssetHolding)
			*resource.AssetHolding = assetHolding
			res[addrID.addr][creatable] = resource
		}
	}
	for _, cidx := range assetParamsReq {
		row := results.QueryRow()
		creator, assetParams, exists, err := l.parseAssetTable(row)
		if err != nil {
			return nil, fmt.Errorf("LookupResources() err: %w", err)
		}
		if exists {
			if _, ok := assetParamsToAddresses[cidx][creator]; ok {
				creatable := ledger.Creatable{
					Index: cidx,
					Type:  basics.AssetCreatable,
				}
				resource := res[creator][creatable]
				resource.AssetParams = new(basics.AssetParams)
				*resource.AssetParams = assetParams
				res[creator][creatable] = resource
			}
		}
	}
	for _, cidx := range appParamsReq {
		row := results.QueryRow()
		creator, appParams, exists, err := l.parseAppTable(row)
		if err != nil {
			return nil, fmt.Errorf("LookupResources() err: %w", err)
		}
		if exists {
			if _, ok := appParamsToAddresses[cidx][creator]; ok {
				creatable := ledger.Creatable{
					Index: cidx,
					Type:  basics.AppCreatable,
				}
				resource := res[creator][creatable]
				resource.AppParams = new(basics.AppParams)
				*resource.AppParams = appParams
				res[creator][creatable] = resource
			}
		}
	}
	for _, addrID := range appLocalStatesReq {
		row := results.QueryRow()
		appLocalState, exists, err := l.parseAccountAppTable(row)
		if err != nil {
			return nil, fmt.Errorf("LookupResources() err: %w", err)
		}
		if exists {
			creatable := ledger.Creatable{
				Index: addrID.id,
				Type:  basics.AppCreatable,
			}
			resource := res[addrID.addr][creatable]
			resource.AppLocalState = new(basics.AppLocalState)
			*resource.AppLocalState = appLocalState
			res[addrID.addr][creatable] = resource
		}
	}

	err := results.Close()
	if err != nil {
		return nil, fmt.Errorf("LookupResources() close results err: %w", err)
	}

	return res, nil
}

func (l *DeprecatedLedgerForEvaluator) parseAddress(row pgx.Row) (basics.Address, bool /*exists*/, error) {
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
func (l DeprecatedLedgerForEvaluator) GetAssetCreator(indices map[basics.AssetIndex]struct{}) (map[basics.AssetIndex]ledger.FoundAddress, error) {
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
func (l DeprecatedLedgerForEvaluator) GetAppCreator(indices map[basics.AppIndex]struct{}) (map[basics.AppIndex]ledger.FoundAddress, error) {
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
func (l DeprecatedLedgerForEvaluator) LatestTotals() (ledgercore.AccountTotals, error) {
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
