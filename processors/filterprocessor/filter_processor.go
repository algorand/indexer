package filterprocessor

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/data/transactions"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
)

const implementationName = "filter_processor"

// package-wide init function
func init() {
	processors.RegisterProcessor(implementationName, &Constructor{})
}

// Constructor is the ProcessorConstructor implementation for the "filter_processor" processor
type Constructor struct{}

// New initializes a FilterProcessor
func (c *Constructor) New() processors.Processor {
	return &FilterProcessor{}
}

type expression interface {
	Search(input interface{}) bool
}

type regexExpression struct {
	Regex *regexp.Regexp
}

func (e regexExpression) Search(input interface{}) bool {
	return e.Regex.MatchString(input.(string))
}

func makeExpression(expressionType FilterExpressionType, expressionSearchStr string) (*expression, error) {
	switch expressionType {
	case ConstFilter:
		{
			r, err := regexp.Compile("^" + expressionSearchStr + "$")
			if err != nil {
				return nil, err
			}

			var exp expression = regexExpression{Regex: r}
			return &exp, nil
		}
	case RegexFilter:
		{
			r, err := regexp.Compile(expressionSearchStr)
			if err != nil {
				return nil, err
			}

			var exp expression = regexExpression{Regex: r}
			return &exp, nil
		}
	default:
		return nil, fmt.Errorf("unknown expression type: %s", expressionType)
	}
}

type fieldOperation string

const someFieldOperation fieldOperation = "some"
const allFieldOperation fieldOperation = "all"

type fieldSearcher struct {
	Exp          *expression
	Tag          string
	MethodToCall string
}

// Search returns true if block contains the expression
func (f fieldSearcher) Search(input transactions.SignedTxnInBlock) bool {

	e := reflect.ValueOf(&input).Elem()

	var field string

	for _, field = range strings.Split(f.Tag, ".") {
		e = e.FieldByName(field)
	}

	toSearch := e.MethodByName(f.MethodToCall).Call([]reflect.Value{})[0].Interface()

	if (*f.Exp).Search(toSearch) {
		return true
	}

	return false
}

// This maps the expression-type with the needed function for the expression.
// For instance the const or regex expression-type might need the String() function
// Can't make this consts because there are no constant maps in go...
var expressionTypeToFunctionMap = map[FilterExpressionType]string{
	ConstFilter: "String",
	RegexFilter:    "String",
}

// checks that the supplied tag exists in the struct and recovers from any panics
func checkTagExistsAndHasCorrectFunction(expressionType FilterExpressionType, tag string) (outError error) {
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

	method, ok := expressionTypeToFunctionMap[expressionType]

	if !ok {
		return fmt.Errorf("expression type (%s) is not supported.  tag value: %s", expressionType, tag)
	}

	if !e.MethodByName(method).IsValid() {
		return fmt.Errorf("variable referenced by tag %s does not contain the needed method: %s", tag, method)
	}

	return nil
}

// makeFieldSearcher will check that the field exists and that it contains the necessary "conversion" function
func makeFieldSearcher(e *expression, expressionType FilterExpressionType, tag string) (*fieldSearcher, error) {

	if err := checkTagExistsAndHasCorrectFunction(expressionType, tag); err != nil {
		return nil, err
	}

	return &fieldSearcher{Exp: e, Tag: tag, MethodToCall: expressionTypeToFunctionMap[expressionType]}, nil
}

type fieldFilter struct {
	Op        fieldOperation
	Searchers []*fieldSearcher
}

func (f fieldFilter) SearchAndFilter(input data.BlockData) (data.BlockData, error) {

	var newPayset []transactions.SignedTxnInBlock
	switch f.Op {
	case someFieldOperation:
		for _, txn := range input.Payset {
			for _, fs := range f.Searchers {
				if fs.Search(txn) {
					newPayset = append(newPayset, txn)
					break
				}
			}
		}

		break
	case allFieldOperation:
		for _, txn := range input.Payset {

			allTrue := true
			for _, fs := range f.Searchers {
				if !fs.Search(txn) {
					allTrue = false
					break
				}
			}

			if allTrue {
				newPayset = append(newPayset, txn)
			}

		}
		break
	default:
		return data.BlockData{}, fmt.Errorf("unknown operation: %s", f.Op)
	}

	input.Payset = newPayset

	return input, nil

}

// FilterProcessor filters transactions by a variety of means
type FilterProcessor struct {
	FieldFilters []fieldFilter

	logger *log.Logger
	cfg    plugins.PluginConfig
	ctx    context.Context
}

// Metadata returns metadata
func (a *FilterProcessor) Metadata() processors.ProcessorMetadata {
	return processors.MakeProcessorMetadata(implementationName, "FilterProcessor Filter Processor", false)
}

// Config returns the config
func (a *FilterProcessor) Config() plugins.PluginConfig {
	return a.cfg
}

// Init initializes the filter processor
func (a *FilterProcessor) Init(ctx context.Context, _ data.InitProvider, cfg plugins.PluginConfig, logger *log.Logger) error {
	a.logger = logger
	a.cfg = cfg
	a.ctx = ctx

	// First get the configuration from the string
	pCfg := Config{}

	err := yaml.Unmarshal([]byte(cfg), &pCfg)
	if err != nil {
		return fmt.Errorf("filter processor init error: %w", err)
	}

	// configMaps is the "- some: ...." portion of the filter config
	for _, configMaps := range pCfg.Filters {

		// We only want one key in the map (i.e. either "some" or "all").  The reason we use a list is that want
		// to maintain ordering of the filters and a straight up map doesn't do that.
		if len(configMaps) != 1 {
			return fmt.Errorf("filter processor Init(): illegal filter tag formation.  tag length was: %d", len(configMaps))
		}

		for key, subConfigs := range configMaps {

			if key != string(someFieldOperation) && key != string(allFieldOperation) {
				return fmt.Errorf("filter processor Init(): filter key was not a valid value: %s", key)
			}

			var searcherList []*fieldSearcher

			for _, subConfig := range subConfigs {

				exp, err := makeExpression(subConfig.ExpressionType, subConfig.Expression)
				if err != nil {
					return fmt.Errorf("filter processor Init(): could not make expression with string %s for filter tag %s - %w", subConfig.Expression, subConfig.FilterTag, err)
				}

				searcher, err := makeFieldSearcher(exp, subConfig.ExpressionType, subConfig.FilterTag)
				if err != nil {
					return fmt.Errorf("filter processor Init(): %w", err)
				}

				searcherList = append(searcherList, searcher)
			}

			ff := fieldFilter{
				Op:        fieldOperation(key),
				Searchers: searcherList,
			}

			a.FieldFilters = append(a.FieldFilters, ff)

		}
	}

	return nil

}

// Close a no-op for this processor
func (a *FilterProcessor) Close() error {
	return nil
}

// Process processes the input data
func (a *FilterProcessor) Process(input data.BlockData) (data.BlockData, error) {

	var err error

	for _, searcher := range a.FieldFilters {
		input, err = searcher.SearchAndFilter(input)
		if err != nil {
			return data.BlockData{}, err
		}
	}

	return input, err
}

// OnComplete a no-op for this processor
func (a *FilterProcessor) OnComplete(input data.BlockData) error {
	return nil
}
