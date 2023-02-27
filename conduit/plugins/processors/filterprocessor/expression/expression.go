package expression

import (
	"fmt"
	"regexp"
	"strconv"
)

// Type is the type of the filter (i.e. const, regex, etc)
type Type string

const (
	// EqualTo a filter that applies numerical and string equal to operations
	EqualTo Type = "equal"
	// Regex a filter that applies regex rules to the matching
	Regex Type = "regex"

	// LessThan a filter that applies numerical less than operation
	LessThan Type = "less-than"
	// LessThanEqual a filter that applies numerical less than or equal operation
	LessThanEqual Type = "less-than-equal"
	// GreaterThan a filter that applies numerical greater than operation
	GreaterThan Type = "greater-than"
	// GreaterThanEqual a filter that applies numerical greater than or equal operation
	GreaterThanEqual Type = "greater-than-equal"
	// NotEqualTo a filter that applies numerical NOT equal to operation
	NotEqualTo Type = "not-equal"
)

// TypeMap contains all the expression types for validation.
var TypeMap = map[Type]interface{}{
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

type stringEqualExpression struct {
	Str string
}

func (e *stringEqualExpression) Match(input interface{}) (bool, error) {
	switch v := input.(type) {
	case string:
		return e.Str == v, nil
	default:
		return false, fmt.Errorf("unexpected regex search input type (%T)", v)
	}
}

func makeRegexExpression(searchStr string, expressionType Type) (Expression, error) {
	if expressionType != EqualTo && expressionType != Regex {
		return nil, fmt.Errorf("target type (string) does not support %s filters", expressionType)
	}
	r, err := regexp.Compile(searchStr)
	if err != nil {
		return nil, err
	}

	return &regexExpression{Regex: r}, nil
}

func makeSignedExpression(searchStr string, expressionType Type) (Expression, error) {
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

func makeUnsignedExpression(searchStr string, expressionType Type) (Expression, error) {
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
func MakeExpression(expressionType Type, expressionSearchStr string, target interface{}) (exp Expression, err error) {
	if _, ok := TypeMap[expressionType]; !ok {
		return nil, fmt.Errorf("expression type (%s) is not supported", expressionType)
	}

	switch t := target.(type) {
	case uint64:
		return makeUnsignedExpression(expressionSearchStr, expressionType)
	case int64:
		return makeSignedExpression(expressionSearchStr, expressionType)
	case string:
		if expressionType == EqualTo {
			return &stringEqualExpression{Str: expressionSearchStr}, nil
		}
		return makeRegexExpression(expressionSearchStr, expressionType)

	default:
		return nil, fmt.Errorf("unknown expression type: %T", t)
	}
}
