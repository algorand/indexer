package expression

import (
	"fmt"
	"regexp"
	"strconv"
)

// ExpressionType is the type of the filter (i.e. const, regex, etc)
type ExpressionType string

const (
	// EqualTo a filter that applies numerical and string equal to operations
	EqualTo ExpressionType = "equal"
	// Regex a filter that applies regex rules to the matching
	Regex ExpressionType = "regex"

	// LessThan a filter that applies numerical less than operation
	LessThan ExpressionType = "less-than"
	// LessThanEqual a filter that applies numerical less than or equal operation
	LessThanEqual ExpressionType = "less-than-equal"
	// GreaterThan a filter that applies numerical greater than operation
	GreaterThan ExpressionType = "greater-than"
	// GreaterThanEqual a filter that applies numerical greater than or equal operation
	GreaterThanEqual ExpressionType = "greater-than-equal"
	// NotEqualTo a filter that applies numerical NOT equal to operation
	NotEqualTo ExpressionType = "not-equal"
)

// TypeMap contains all the expression types for validation.
var TypeMap = map[ExpressionType]interface{}{
	Regex:            struct{}{},
	LessThan:         struct{}{},
	LessThanEqual:    struct{}{},
	GreaterThan:      struct{}{},
	GreaterThanEqual: struct{}{},
	EqualTo:          struct{}{},
	NotEqualTo:       struct{}{},
}

// Expression the expression interface
type Expression interface {
	Match(input interface{}) (bool, error)
}

type regexExpression struct {
	Regex *regexp.Regexp
}

func (e *regexExpression) Match(input interface{}) (bool, error) {
	switch v := input.(type) {
	case string:
		return e.Regex.MatchString(v), nil
	default:
		return false, fmt.Errorf("unexpected regex search input type (%T)", v)
	}
}

func makeRegexExpression(searchStr string, expressionType ExpressionType) (Expression, error) {
	if expressionType != EqualTo && expressionType != Regex {
		return nil, fmt.Errorf("target type (string) does not support %s filters", expressionType)
	}
	r, err := regexp.Compile(searchStr)
	if err != nil {
		return nil, err
	}

	return &regexExpression{Regex: r}, nil
}

func makeSignedExpression(searchStr string, expressionType ExpressionType) (Expression, error) {
	if expressionType == Regex {
		return nil, fmt.Errorf("target type (numeric) does not support %s filters", expressionType)
	}

	v, err := strconv.ParseInt(searchStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("search string \"%s\" is not a valid int64: %w", searchStr, err)
	}

	return &int64NumericalExpression{
		FilterValue: v,
		Op:          expressionType,
	}, nil
}

func makeUnsignedExpression(searchStr string, expressionType ExpressionType) (Expression, error) {
	if expressionType == Regex {
		return nil, fmt.Errorf("target type (numeric) does not support %s filters", expressionType)
	}

	v, err := strconv.ParseUint(searchStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("search string \"%s\" is not a valid uint64: %w", searchStr, err)
	}

	return &uint64NumericalExpression{
		FilterValue: v,
		Op:          expressionType,
	}, nil
}

// MakeExpression creates an expression based on an expression type
func MakeExpression(filterType ExpressionType, expressionSearchStr string, target interface{}) (exp Expression, err error) {
	switch t := target.(type) {
	case uint64:
		return makeUnsignedExpression(expressionSearchStr, filterType)
	case int64:
		return makeSignedExpression(expressionSearchStr, filterType)
	case string:
		if filterType == EqualTo {
			// Equal to for strings is a special case of the regex pattern.
			expressionSearchStr = fmt.Sprintf("^%s$", regexp.QuoteMeta(expressionSearchStr))
		}
		return makeRegexExpression(expressionSearchStr, filterType)

	default:
		return nil, fmt.Errorf("unknown expression type: %T", t)
	}
}
