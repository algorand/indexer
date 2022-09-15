package fields

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/indexer/processors/filterprocessor/expression"
)

// Searcher searches the struct with an expression and method to call
type Searcher struct {
	Exp          *expression.Expression
	Tag          string
	MethodToCall string
}

// Search returns true if block contains the expression
func (f Searcher) Search(input transactions.SignedTxnInBlock) bool {

	e := reflect.ValueOf(&input).Elem()

	for _, field := range strings.Split(f.Tag, ".") {
		e = e.FieldByName(field)
	}

	toSearch := e.MethodByName(f.MethodToCall).Call([]reflect.Value{})[0].Interface()

	if (*f.Exp).Search(toSearch) {
		return true
	}

	return false
}

// checks that the supplied tag exists in the struct and recovers from any panics
func checkTagExistsAndHasCorrectFunction(expressionType expression.FilterType, tag string) (outError error) {
	var field string
	defer func() {
		if r := recover(); r != nil {
			outError = fmt.Errorf("error occured regarding tag %s. last searched field was: %s - %v", tag, field, r)
		}
	}()

	e := reflect.ValueOf(&transactions.SignedTxnInBlock{}).Elem()

	for _, field = range strings.Split(tag, ".") {
		e = e.FieldByName(field)
		if !e.IsValid() {
			return fmt.Errorf("%s does not exist in transactions.SignedTxnInBlock struct. last searched field was: %s", tag, field)
		}
	}

	method, ok := expression.TypeToFunctionMap[expressionType]

	if !ok {
		return fmt.Errorf("expression type (%s) is not supported.  tag value: %s", expressionType, tag)
	}

	if !e.MethodByName(method).IsValid() {
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
