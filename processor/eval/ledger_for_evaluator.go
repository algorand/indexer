// Package eval implements the 'ledger.indexerLedgerForEval' interface for
// generating StateDelta's and updating ApplyData with a custom protocol.
//
// TODO: Expose private functions in go-algorand to allow code reuse.
// This interface is designed to allow sourcing initial state data from
// postgres. Since we are not sourcing initial states from the ledger there
// is no need for this custom code, except that the interface doesn't support
// reading from the ledger.
package eval

import (
	"fmt"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

// LedgerForEvaluator implements the indexerLedgerForEval interface from
// go-algorand ledger/evalindexer.go and is used for accounting.
type LedgerForEvaluator struct {
	Ledger *ledger.Ledger
}

// MakeLedgerForEvaluator creates a LedgerForEvaluator object.
func MakeLedgerForEvaluator(ld *ledger.Ledger) LedgerForEvaluator {
	l := LedgerForEvaluator{
		Ledger: ld,
	}
	return l
}

// Close shuts down LedgerForEvaluator.
func (l *LedgerForEvaluator) Close() {
	l.Ledger.Close()
}

// LatestBlockHdr is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LatestBlockHdr() (bookkeeping.BlockHeader, error) {
	return l.Ledger.BlockHdr(l.Ledger.Latest())
}

// LookupWithoutRewards is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LookupWithoutRewards(addresses map[basics.Address]struct{}) (map[basics.Address]*ledgercore.AccountData, error) {

	addressesArr := make([]basics.Address, 0, len(addresses))
	for address := range addresses {
		addressesArr = append(addressesArr, address)
	}

	res := make(map[basics.Address]*ledgercore.AccountData, len(addresses))
	for _, address := range addressesArr {
		acctData, _, err := l.Ledger.LookupWithoutRewards(l.Ledger.Latest(), address)
		if err != nil {
			return nil, err
		}
		if acctData.IsZero() {
			res[address] = nil
		} else {
			res[address] = &acctData
		}
	}

	return res, nil
}

// LookupResources is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LookupResources(input map[basics.Address]map[ledger.Creatable]struct{}) (map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource, error) {
	// Initialize the result `res` with the same structure as `input`.
	res := make(
		map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource, len(input))

	for address, creatables := range input {
		creatablesOutput :=
			make(map[ledger.Creatable]ledgercore.AccountResource, len(creatables))
		res[address] = creatablesOutput
		for creatable := range creatables {
			creatablesOutput[creatable] = ledgercore.AccountResource{}
			switch creatable.Type {
			case basics.AssetCreatable:
				resource, err := l.Ledger.LookupAsset(l.Ledger.Latest(), address, basics.AssetIndex(creatable.Index))
				if err != nil {
					return nil, fmt.Errorf(
						"LookupResources() %d failed", creatable.Type)
				}
				res[address][creatable] = ledgercore.AccountResource{AssetHolding: resource.AssetHolding, AssetParams: resource.AssetParams}
			case basics.AppCreatable:
				resource, err := l.Ledger.LookupApplication(l.Ledger.Latest(), address, basics.AppIndex(creatable.Index))
				if err != nil {
					return nil, fmt.Errorf(
						"LookupResources() %d failed", creatable.Type)
				}
				res[address][creatable] = ledgercore.AccountResource{AppLocalState: resource.AppLocalState, AppParams: resource.AppParams}
			default:
				return nil, fmt.Errorf(
					"LookupResources() unknown creatable type %d", creatable.Type)
			}
		}

	}
	return res, nil
}

// GetAssetCreator is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) GetAssetCreator(indices map[basics.AssetIndex]struct{}) (map[basics.AssetIndex]ledger.FoundAddress, error) {
	indicesArr := make([]basics.AssetIndex, 0, len(indices))
	for index := range indices {
		indicesArr = append(indicesArr, index)
	}

	res := make(map[basics.AssetIndex]ledger.FoundAddress, len(indices))
	for _, index := range indicesArr {
		cidx := basics.CreatableIndex(index)
		address, exists, err := l.Ledger.GetCreator(cidx, basics.AssetCreatable)
		if err != nil {
			return nil, fmt.Errorf("GetAssetCreator() err: %w", err)
		}
		res[index] = ledger.FoundAddress{Address: address, Exists: exists}
	}

	return res, nil
}

// GetAppCreator is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) GetAppCreator(indices map[basics.AppIndex]struct{}) (map[basics.AppIndex]ledger.FoundAddress, error) {
	indicesArr := make([]basics.AppIndex, 0, len(indices))
	for index := range indices {
		indicesArr = append(indicesArr, index)
	}

	res := make(map[basics.AppIndex]ledger.FoundAddress, len(indices))
	for _, index := range indicesArr {
		cidx := basics.CreatableIndex(index)
		address, exists, err := l.Ledger.GetCreatorForRound(l.Ledger.Latest(), cidx, basics.AppCreatable)
		if err != nil {
			return nil, fmt.Errorf("GetAppCreator() err: %w", err)
		}
		res[index] = ledger.FoundAddress{Address: address, Exists: exists}
	}

	return res, nil
}

// LookupKv is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LookupKv(round basics.Round, key string) ([]byte, error) {
	// a simple pass-thru to the go-algorand ledger
	return l.Ledger.LookupKv(round, key)
}

// LatestTotals is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LatestTotals() (ledgercore.AccountTotals, error) {
	_, totals, err := l.Ledger.LatestTotals()
	return totals, err
}

// BlockHdrCached is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) BlockHdrCached(round basics.Round) (bookkeeping.BlockHeader, error) {
	return l.Ledger.BlockHdrCached(round)
}
