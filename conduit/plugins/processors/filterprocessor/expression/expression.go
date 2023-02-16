package expression

import (
	"fmt"
	"regexp"
	"strconv"
)

// FilterType is the type of the filter (i.e. const, regex, etc)
type FilterType string

const (
	// EqualToFilter a filter that applies numerical and string equal to operations
	EqualToFilter FilterType = "equal"
	// RegexFilter a filter that applies regex rules to the matching
	RegexFilter FilterType = "regex"

	// LessThanFilter a filter that applies numerical less than operation
	LessThanFilter FilterType = "less-than"
	// LessThanEqualFilter a filter that applies numerical less than or equal operation
	LessThanEqualFilter FilterType = "less-than-equal"
	// GreaterThanFilter a filter that applies numerical greater than operation
	GreaterThanFilter FilterType = "greater-than"
	// GreaterThanEqualFilter a filter that applies numerical greater than or equal operation
	GreaterThanEqualFilter FilterType = "greater-than-equal"
	// NotEqualToFilter a filter that applies numerical NOT equal to operation
	NotEqualToFilter FilterType = "not-equal"
)

// TypeMap contains all the expression types for validation.
var TypeMap = map[FilterType]interface{}{
	RegexFilter:            struct{}{},
	LessThanFilter:         struct{}{},
	LessThanEqualFilter:    struct{}{},
	GreaterThanFilter:      struct{}{},
	GreaterThanEqualFilter: struct{}{},
	EqualToFilter:          struct{}{},
	NotEqualToFilter:       struct{}{},
}

// Expression the expression interface
type Expression interface {
	Search(input interface{}) (bool, error)
}

type regexExpression struct {
	Regex *regexp.Regexp
}

func (e *regexExpression) Search(input interface{}) (bool, error) {
	switch v := input.(type) {
	case string:
		return e.Regex.MatchString(v), nil
	default:
		return false, fmt.Errorf("unexpected regex search input type (%T)", v)
	}
}

func makeRegexExpression(searchStr string, expressionType FilterType) (Expression, error) {
	if expressionType != EqualToFilter && expressionType != RegexFilter {
		return nil, fmt.Errorf("target type (string) does not support %s filters", expressionType)
	}
	r, err := regexp.Compile(searchStr)
	if err != nil {
		return nil, err
	}

	return &regexExpression{Regex: r}, nil
}

func makeSignedExpression(searchStr string, expressionType FilterType) (Expression, error) {
	if expressionType == RegexFilter {
		return nil, fmt.Errorf("target type (numeric) does not support %s filters", expressionType)
	}

	v, err := strconv.ParseInt(searchStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("search string \"%s\" is not numeric: %w", searchStr, err)
	}

	return &int64NumericalExpression{
		FilterValue: v,
		Op:          expressionType,
	}, nil
}

func makeUnsignedExpression(searchStr string, expressionType FilterType) (Expression, error) {
	if expressionType == RegexFilter {
		return nil, fmt.Errorf("target type (numeric) does not support %s filters", expressionType)
	}

	v, err := strconv.ParseUint(searchStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("search string \"%s\" is not numeric: %w", searchStr, err)
	}

	return &uint64NumericalExpression{
		FilterValue: v,
		Op:          expressionType,
	}, nil
}

// MakeExpression creates an expression based on an expression type
func MakeExpression(filterType FilterType, expressionSearchStr string, target interface{}) (exp Expression, err error) {
	switch t := target.(type) {
	case uint64:
		return makeUnsignedExpression(expressionSearchStr, filterType)
	case int64:
		return makeSignedExpression(expressionSearchStr, filterType)
	case string:
		if filterType == EqualToFilter {
			// Equal to for strings is a special case of the regex pattern.
			expressionSearchStr = fmt.Sprintf("^%s$", regexp.QuoteMeta(expressionSearchStr))
		}
		return makeRegexExpression(expressionSearchStr, filterType)

	default:
		return nil, fmt.Errorf("unknown expression type: %T", t)
	}
}
