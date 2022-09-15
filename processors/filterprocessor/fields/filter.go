package fields

import (
	"fmt"

	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/indexer/data"
)

// Operation an operation like "any" or "all" for boolean logic
type Operation string

const anyFieldOperation Operation = "any"
const allFieldOperation Operation = "all"

// ValidFieldOperation returns true if the input is a valid operation
func ValidFieldOperation(input string) bool {
	if input != string(anyFieldOperation) && input != string(allFieldOperation) {
		return false
	}

	return true
}

// Filter an object that combines field searches with a boolean operator
type Filter struct {
	Op        Operation
	Searchers []*Searcher
}

// SearchAndFilter searches through the block data and applies the operation to the results
func (f Filter) SearchAndFilter(input data.BlockData) (data.BlockData, error) {

	var newPayset []transactions.SignedTxnInBlock
	switch f.Op {
	case anyFieldOperation:
		for _, txn := range input.Payset {
			for _, fs := range f.Searchers {
				if fs.Search(txn) {
					newPayset = append(newPayset, txn)
					break
				}
			}
		}

		break
	case allFieldOperation:
		for _, txn := range input.Payset {

			allTrue := true
			for _, fs := range f.Searchers {
				if !fs.Search(txn) {
					allTrue = false
					break
				}
			}

			if allTrue {
				newPayset = append(newPayset, txn)
			}

		}
		break
	default:
		return data.BlockData{}, fmt.Errorf("unknown operation: %s", f.Op)
	}

	input.Payset = newPayset

	return input, nil

}
