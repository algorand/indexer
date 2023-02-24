package fields

import (
	"fmt"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// Operation an operation like "any" or "all" for boolean logic
type Operation string

const anyFieldOperation Operation = "any"
const allFieldOperation Operation = "all"
const noneFieldOperation Operation = "none"

// ValidFieldOperation returns true if the input is a valid operation
func ValidFieldOperation(input string) bool {
	if input != string(anyFieldOperation) && input != string(allFieldOperation) && input != string(noneFieldOperation) {
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
func (f Filter) SearchAndFilter(payset []sdk.SignedTxnInBlock) ([]sdk.SignedTxnInBlock, error) {
	var result []sdk.SignedTxnInBlock
	switch f.Op {
	case noneFieldOperation:
		for _, txn := range payset {

			allFalse := true
			for _, fs := range f.Searchers {
				b, err := fs.search(txn)
				if err != nil {
					return nil, err
				}
				if b {
					allFalse = false
					break
				}
			}

			if allFalse {
				result = append(result, txn)
			}

		}
		break

	case anyFieldOperation:
		for _, txn := range payset {
			for _, fs := range f.Searchers {
				b, err := fs.search(txn)
				if err != nil {
					return nil, err
				}
				if b {
					result = append(result, txn)
					break
				}
			}
		}

		break
	case allFieldOperation:
		for _, txn := range payset {

			allTrue := true
			for _, fs := range f.Searchers {
				b, err := fs.search(txn)
				if err != nil {
					return nil, err
				}
				if !b {
					allTrue = false
					break
				}
			}

			if allTrue {
				result = append(result, txn)
			}
		}
		break
	default:
		return nil, fmt.Errorf("unknown operation: %s", f.Op)
	}

	return result, nil
}
