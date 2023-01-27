package expression

import (
	"fmt"
	"reflect"

	"github.com/algorand/go-algorand/data/basics"
)

type microAlgoExpression struct {
	FilterValue basics.MicroAlgos
	Op          FilterType
}

func (m microAlgoExpression) Search(input interface{}) (bool, error) {

	inputValue, ok := input.(basics.MicroAlgos)
	if !ok {
		return false, fmt.Errorf("supplied type (%s) was not microalgos", reflect.TypeOf(input).String())
	}

	switch m.Op {
	case LessThanFilter:
		return inputValue.Raw < m.FilterValue.Raw, nil
	case LessThanEqualFilter:
		return inputValue.Raw <= m.FilterValue.Raw, nil
	case EqualToFilter:
		return inputValue.Raw == m.FilterValue.Raw, nil
	case NotEqualToFilter:
		return inputValue.Raw != m.FilterValue.Raw, nil
	case GreaterThanFilter:
		return inputValue.Raw > m.FilterValue.Raw, nil
	case GreaterThanEqualFilter:
		return inputValue.Raw >= m.FilterValue.Raw, nil
	default:
		return false, fmt.Errorf("unknown op: %s", m.Op)
	}
}

type int64NumericalExpression struct {
	FilterValue int64
	Op          FilterType
}

func (s int64NumericalExpression) Search(input interface{}) (bool, error) {
	inputValue, ok := input.(int64)
	if !ok {
		// If the input interface{} isn't int64 but an alias for int64, we want to check that
		if reflect.TypeOf(input).ConvertibleTo(reflect.TypeOf(int64(0))) {
			inputValue = reflect.ValueOf(input).Convert(reflect.TypeOf(int64(0))).Int()
		} else {
			return false, fmt.Errorf("supplied type (%s) was not int64", reflect.TypeOf(input).String())
		}
	}

	switch s.Op {
	case LessThanFilter:
		return inputValue < s.FilterValue, nil
	case LessThanEqualFilter:
		return inputValue <= s.FilterValue, nil
	case EqualToFilter:
		return inputValue == s.FilterValue, nil
	case NotEqualToFilter:
		return inputValue != s.FilterValue, nil
	case GreaterThanFilter:
		return inputValue > s.FilterValue, nil
	case GreaterThanEqualFilter:
		return inputValue >= s.FilterValue, nil
	default:
		return false, fmt.Errorf("unknown op: %s", s.Op)
	}

}

type uint64NumericalExpression struct {
	FilterValue uint64
	Op          FilterType
}

func (u uint64NumericalExpression) Search(input interface{}) (bool, error) {
	inputValue, ok := input.(uint64)
	if !ok {
		// If the input interface{} isn't uint64 but an alias for uint64, we want to check that
		if reflect.TypeOf(input).ConvertibleTo(reflect.TypeOf(uint64(0))) {
			inputValue = reflect.ValueOf(input).Convert(reflect.TypeOf(uint64(0))).Uint()
		} else {
			return false, fmt.Errorf("supplied type (%s) was not uint64", reflect.TypeOf(input).String())
		}
	}

	switch u.Op {
	case LessThanFilter:
		return inputValue < u.FilterValue, nil
	case LessThanEqualFilter:
		return inputValue <= u.FilterValue, nil
	case EqualToFilter:
		return inputValue == u.FilterValue, nil
	case NotEqualToFilter:
		return inputValue != u.FilterValue, nil
	case GreaterThanFilter:
		return inputValue > u.FilterValue, nil
	case GreaterThanEqualFilter:
		return inputValue >= u.FilterValue, nil

	default:
		return false, fmt.Errorf("unknown op: %s", u.Op)
	}
}
