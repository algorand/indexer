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
	Exp          *expression.Expression
	Tag          string
	MethodToCall string
}

// This function is ONLY to be used by the filter.field function.
// The reason being is that without validation of the tag (which is provided by
// MakeFieldSearcher) then this can panic
func (f Searcher) search(input transactions.SignedTxnInBlock) (bool, error) {

	val, err := SignedTxnFunc(f.Tag, &input)
	if err != nil {
		return false, err
	}

	e := reflect.ValueOf(val).Elem()

	var toSearch interface{}
	if f.MethodToCall != "" {
		// If there is a function, search what is returned
		toSearch = e.MethodByName(f.MethodToCall).Call([]reflect.Value{})[0].Interface()
	} else {
		// Otherwise, get the original value
		toSearch = e.Interface()
	}

	b, err := (*f.Exp).Search(toSearch)
	if err != nil {
		return false, err
	}

	return b, nil
}

// checks that the supplied tag exists in the struct and recovers from any panics
func checkTagExistsAndHasCorrectFunction(expressionType expression.FilterType, tag string) (outError error) {
	defer func() {
		// This defer'd function is a belt and suspenders type thing.  We check every reflected
		// evaluation's IsValid() function to make sure not to operate on a zero value.  Therfore we can't
		// actually reach inside the if conditional unless we intentionally panic.
		// However, having this function gives additional safety to a critical function
		if r := recover(); r != nil {
			outError = fmt.Errorf("error occurred regarding tag %s - %v", tag, r)
		}
	}()

	val, err := SignedTxnFunc(tag, &transactions.SignedTxnInBlock{})

	if err != nil {
		return fmt.Errorf("%s does not exist in transactions.SignedTxnInBlock struct", tag)
	}

	e := reflect.ValueOf(val).Elem()

	method, ok := expression.TypeToFunctionMap[expressionType]

	if !ok {
		return fmt.Errorf("expression type (%s) is not supported.  tag value: %s", expressionType, tag)
	}

	if method != "" && !e.MethodByName(method).IsValid() {
		return fmt.Errorf("variable referenced by tag %s does not contain the needed method: %s", tag, method)
	}

	return nil
}

// MakeFieldSearcher will check that the field exists and that it contains the necessary "conversion" function
func MakeFieldSearcher(e *expression.Expression, expressionType expression.FilterType, tag string) (*Searcher, error) {

	if err := checkTagExistsAndHasCorrectFunction(expressionType, tag); err != nil {
		return nil, err
	}

	return &Searcher{Exp: e, Tag: tag, MethodToCall: expression.TypeToFunctionMap[expressionType]}, nil
}
