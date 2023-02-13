package fields

//go:generate go run ../gen/generate.go fields ./generated_signed_txn_map.go

import (
	"fmt"
	"reflect"

	"github.com/algorand/indexer/conduit/plugins/processors/filterprocessor/expression"

	"github.com/algorand/go-algorand/data/transactions"
)

// Searcher searches the struct with an expression and method to call
type Searcher struct {
	Exp *expression.Expression
	Tag string
}

// This function is ONLY to be used by the filter.field function.
// The reason being is that without validation of the tag (which is provided by
// MakeFieldSearcher) then this can panic
func (f Searcher) search(input transactions.SignedTxnInBlock) (bool, error) {

	val, err := LookupFieldByTag(f.Tag, &input)
	if err != nil {
		return false, err
	}

	e := reflect.ValueOf(val).Elem()

	b, err := (*f.Exp).Search(e.Interface())
	if err != nil {
		return false, err
	}

	return b, nil
}

// checks that the supplied tag exists in the struct and recovers from any panics
func checkTagAndExpressionExist(expressionType expression.FilterType, tag string) (outError error) {
	defer func() {
		// This defer'd function is a belt and suspenders type thing.  We check every reflected
		// evaluation's IsValid() function to make sure not to operate on a zero value.  Therfore we can't
		// actually reach inside the if conditional unless we intentionally panic.
		// However, having this function gives additional safety to a critical function
		if r := recover(); r != nil {
			outError = fmt.Errorf("error occurred regarding tag %s - %v", tag, r)
		}
	}()

	_, err := LookupFieldByTag(tag, &transactions.SignedTxnInBlock{})

	if err != nil {
		return fmt.Errorf("%s does not exist in transactions.SignedTxnInBlock struct", tag)
	}

	if _, ok := expression.TypeMap[expressionType]; !ok {
		return fmt.Errorf("expression type (%s) is not supported", expressionType)
	}

	return nil
}

// MakeFieldSearcher will check that the field exists and that it contains the necessary "conversion" function
func MakeFieldSearcher(e *expression.Expression, expressionType expression.FilterType, tag string) (*Searcher, error) {

	if err := checkTagAndExpressionExist(expressionType, tag); err != nil {
		return nil, err
	}

	return &Searcher{Exp: e, Tag: tag}, nil
}
