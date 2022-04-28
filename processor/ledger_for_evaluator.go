package processor

import (
	"fmt"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

// LedgerForEvaluator implements the indexerLedgerForEval interface from
// go-algorand ledger/eval.go and is used for accounting.
type LedgerForEvaluator struct {
	Ledger *ledger.Ledger
}

// MakeLedgerForEvaluator creates a LedgerForEvaluator object.
func MakeLedgerForEvaluator(ld *ledger.Ledger) (LedgerForEvaluator, error) {
	l := LedgerForEvaluator{
		Ledger: ld,
	}
	return l, nil
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
		res[address] = &acctData
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
		}
	}

	for address, creatables := range input {
		for creatable := range creatables {
			switch creatable.Type {
			case basics.AssetCreatable:
				resource, err := l.Ledger.LookupResource(0, address, creatable.Index, basics.AssetCreatable)
				if err != nil {
					return nil, fmt.Errorf(
						"LookupResources() %d failed", creatable.Type)
				}
				res[address][creatable] = resource
			case basics.AppCreatable:
				resource, err := l.Ledger.LookupResource(0, address, creatable.Index, basics.AppCreatable)
				if err != nil {
					return nil, fmt.Errorf(
						"LookupResources() %d failed", creatable.Type)
				}
				res[address][creatable] = resource
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
		address, exists, err := l.Ledger.GetCreatorForRound(0, cidx, basics.AssetCreatable)
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
		address, exists, err := l.Ledger.GetCreatorForRound(0, cidx, basics.AssetCreatable)
		if err != nil {
			return nil, fmt.Errorf("GetAppCreator() err: %w", err)
		}
		res[index] = ledger.FoundAddress{Address: address, Exists: exists}
	}

	return res, nil
}

// LatestTotals is part of go-algorand's indexerLedgerForEval interface.
func (l LedgerForEvaluator) LatestTotals() (ledgercore.AccountTotals, error) {
	_, totals, err := l.Ledger.LatestTotals()
	return totals, err
}
