package fields

//go:generate go run ../gen/generate.go fields ./generated_signed_txn_map.go

import (
	"fmt"

	"github.com/algorand/indexer/conduit/plugins/processors/filterprocessor/expression"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// Searcher searches the struct with an expression
type Searcher struct {
	Exp expression.Expression
	Tag string
}

// This function is ONLY to be used by the filter.field function.
// The reason being is that without validation of the tag (which is provided by
// MakeFieldSearcher) then this can panic
func (f Searcher) search(input sdk.SignedTxnInBlock) (bool, error) {

	val, err := LookupFieldByTag(f.Tag, &input.SignedTxnWithAD)
	if err != nil {
		return false, err
	}

	b, err := f.Exp.Search(val)
	if err != nil {
		return false, err
	}

	return b, nil
}

// checks that the supplied tag exists in the struct and recovers from any panics
func checkTagAndExpressionExist(expressionType expression.FilterType, tag string) (outError error) {
	_, err := LookupFieldByTag(tag, &sdk.SignedTxnWithAD{})

	if err != nil {
		return fmt.Errorf("%s does not exist in transactions.SignedTxnInBlock struct", tag)
	}

	if _, ok := expression.TypeMap[expressionType]; !ok {
		return fmt.Errorf("expression type (%s) is not supported", expressionType)
	}

	return nil
}

// MakeFieldSearcher will check that the field exists and that it contains the necessary "conversion" function
func MakeFieldSearcher(e expression.Expression, expressionType expression.FilterType, tag string) (*Searcher, error) {

	if err := checkTagAndExpressionExist(expressionType, tag); err != nil {
		return nil, err
	}

	return &Searcher{Exp: e, Tag: tag}, nil
}
