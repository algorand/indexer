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

func (f Filter) matches(txn *sdk.SignedTxnWithAD) (bool, error) {
	numMatches := 0
	for _, fs := range f.Searchers {
		b, err := fs.search(txn)
		if err != nil {
			return false, err
		}
		if b {
			numMatches++
		}
	}

	switch f.Op {
	case noneFieldOperation:
		return numMatches == 0, nil
	case anyFieldOperation:
		return numMatches > 0, nil
	case allFieldOperation:
		return numMatches == len(f.Searchers), nil
	default:
		return false, fmt.Errorf("unknown operation: %s", f.Op)
	}
}

// SearchAndFilter searches through the block data and applies the operation to the results
func (f Filter) SearchAndFilter(payset []sdk.SignedTxnInBlock) ([]sdk.SignedTxnInBlock, error) {
	var result []sdk.SignedTxnInBlock
	for _, txn := range payset {
		match, err := f.matches(&txn.SignedTxnWithAD)
		if err != nil {
			return nil, err
		}
		if match {
			result = append(result, txn)
		}
	}

	return result, nil
}
