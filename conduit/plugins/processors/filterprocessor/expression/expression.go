package expression

import (
	"fmt"
	"regexp"
	"strconv"
)

// FilterType is the type of the filter (i.e. const, regex, etc)
type FilterType string

const (
	// ExactFilter a filter that looks at a constant string in its entirety
	ExactFilter FilterType = "exact"
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
	// EqualToFilter a filter that applies numerical equal to operation
	EqualToFilter FilterType = "equal"
	// NotEqualToFilter a filter that applies numerical NOT equal to operation
	NotEqualToFilter FilterType = "not-equal"
)

// TypeMap contains all the expression types for validation.
var TypeMap = map[FilterType]interface{}{
	ExactFilter:            struct{}{},
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

func (e regexExpression) Search(input interface{}) (bool, error) {
	return e.Regex.MatchString(input.(string)), nil
}

// MakeExpression creates an expression based on an expression type
func MakeExpression(expressionType FilterType, expressionSearchStr string, target interface{}) (*Expression, error) {
	switch expressionType {
	case ExactFilter:
		{
			switch t := target.(type) {
			case string:
				r, err := regexp.Compile("^" + expressionSearchStr + "$")
				if err != nil {
					return nil, err
				}

				var exp Expression = regexExpression{Regex: r}
				return &exp, nil
			default:
				return nil, fmt.Errorf("unknown target type (%T) for filter type: %s", t, expressionType)
			}
		}
	case RegexFilter:
		{
			switch t := target.(type) {
			case string:
				r, err := regexp.Compile(expressionSearchStr)
				if err != nil {
					return nil, err
				}

				var exp Expression = regexExpression{Regex: r}
				return &exp, nil
			default:
				return nil, fmt.Errorf("unknown target type (%T) for filter type: %s", t, expressionType)
			}
		}

	case LessThanFilter:
		fallthrough
	case LessThanEqualFilter:
		fallthrough
	case GreaterThanFilter:
		fallthrough
	case GreaterThanEqualFilter:
		fallthrough
	case EqualToFilter:
		fallthrough
	case NotEqualToFilter:
		{
			var exp Expression

			switch t := target.(type) {
			case int64:
				{
					v, err := strconv.ParseInt(expressionSearchStr, 10, 64)
					if err != nil {
						return nil, err
					}

					exp = int64NumericalExpression{
						FilterValue: v,
						Op:          expressionType,
					}
				}

			case uint64:
				{
					v, err := strconv.ParseUint(expressionSearchStr, 10, 64)
					if err != nil {
						return nil, err
					}

					exp = uint64NumericalExpression{
						FilterValue: v,
						Op:          expressionType,
					}
				}

			default:
				return nil, fmt.Errorf("unknown target type (%T) for filter type: %s", t, expressionType)
			}

			return &exp, nil

		}

	default:
		return nil, fmt.Errorf("unknown expression type: %s", expressionType)
	}
}
