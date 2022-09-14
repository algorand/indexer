package expression

import (
	"fmt"
	"regexp"
)

// FilterType is the type of the filter (i.e. const, regex, etc)
type FilterType string

const (
	// ConstFilter a filter that looks at a constant string in its entirety
	ConstFilter FilterType = "const"
	// RegexFilter a filter that applies regex rules to the matching
	RegexFilter FilterType = "regex"
)

// TypeToFunctionMap maps the expression-type with the needed function for the expression.
// For instance the const or regex expression-type might need the String() function
// Can't make this consts because there are no constant maps in go...
var TypeToFunctionMap = map[FilterType]string{
	ConstFilter: "String",
	RegexFilter: "String",
}

// Expression the expression interface
type Expression interface {
	Search(input interface{}) bool
}

type regexExpression struct {
	Regex *regexp.Regexp
}

func (e regexExpression) Search(input interface{}) bool {
	return e.Regex.MatchString(input.(string))
}

// MakeExpression creates an expression based on an expression type
func MakeExpression(expressionType FilterType, expressionSearchStr string) (*Expression, error) {
	switch expressionType {
	case ConstFilter:
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
	default:
		return nil, fmt.Errorf("unknown expression type: %s", expressionType)
	}
}
