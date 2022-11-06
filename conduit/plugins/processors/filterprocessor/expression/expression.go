package expression

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	"github.com/algorand/go-algorand/data/basics"
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

// TypeToFunctionMap maps the expression-type with the needed function for the expression.
// For instance the exact or regex expression-type might need the String() function
// A blank string means a function is not required
// Can't make this const because there are no constant maps in go...
var TypeToFunctionMap = map[FilterType]string{
	ExactFilter:            "String",
	RegexFilter:            "String",
	LessThanFilter:         "",
	LessThanEqualFilter:    "",
	GreaterThanFilter:      "",
	GreaterThanEqualFilter: "",
	EqualToFilter:          "",
	NotEqualToFilter:       "",
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
func MakeExpression(expressionType FilterType, expressionSearchStr string, targetKind reflect.Kind) (*Expression, error) {
	switch expressionType {
	case ExactFilter:
		{
			r, err := regexp.Compile("^" + expressionSearchStr + "$")
			if err != nil {
				return nil, err
			}

			var exp Expression = regexExpression{Regex: r}
			return &exp, nil
		}
	case RegexFilter:
		{
			r, err := regexp.Compile(expressionSearchStr)
			if err != nil {
				return nil, err
			}

			var exp Expression = regexExpression{Regex: r}
			return &exp, nil
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

			switch targetKind {
			case reflect.TypeOf(basics.MicroAlgos{}).Kind():
				{
					v, err := strconv.ParseUint(expressionSearchStr, 10, 64)
					if err != nil {
						return nil, err
					}

					exp = microAlgoExpression{
						FilterValue: basics.MicroAlgos{Raw: v},
						Op:          expressionType,
					}
				}

			case reflect.Int64:
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

			case reflect.Uint64:
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
				return nil, fmt.Errorf("unknown target kind (%s) for filter type: %s", targetKind.String(), expressionType)
			}

			return &exp, nil

		}

	default:
		return nil, fmt.Errorf("unknown expression type: %s", expressionType)
	}
}
