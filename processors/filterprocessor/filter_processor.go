package filterprocessor

import (
	"context"
	"fmt"
	"github.com/algorand/go-algorand/data/transactions"
	"reflect"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/plugins"
	"github.com/algorand/indexer/processors"
	"github.com/algorand/indexer/processors/filterprocessor/expression"
	"github.com/algorand/indexer/processors/filterprocessor/fields"
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

// FilterProcessor filters transactions by a variety of means
type FilterProcessor struct {
	FieldFilters []fields.Filter

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

	// configMaps is the "- any: ...." portion of the filter config
	for _, configMaps := range pCfg.Filters {

		// We only want one key in the map (i.e. either "any" or "all").  The reason we use a list is that want
		// to maintain ordering of the filters and a straight-up map doesn't do that.
		if len(configMaps) != 1 {
			return fmt.Errorf("filter processor Init(): illegal filter tag formation.  tag length was: %d", len(configMaps))
		}

		for key, subConfigs := range configMaps {

			if !fields.ValidFieldOperation(key) {
				return fmt.Errorf("filter processor Init(): filter key was not a valid value: %s", key)
			}

			var searcherList []*fields.Searcher

			for _, subConfig := range subConfigs {

				t, err := fields.SignedTxnFunc(subConfig.FilterTag, &transactions.SignedTxnInBlock{})
				if err != nil {
					return err
				}

				// We need the Elem() here because SignedTxnFunc returns a pointer underneath the interface{}
				targetKind := reflect.TypeOf(t).Elem().Kind()

				exp, err := expression.MakeExpression(subConfig.ExpressionType, subConfig.Expression, targetKind)
				if err != nil {
					return fmt.Errorf("filter processor Init(): could not make expression with string %s for filter tag %s - %w", subConfig.Expression, subConfig.FilterTag, err)
				}

				searcher, err := fields.MakeFieldSearcher(exp, subConfig.ExpressionType, subConfig.FilterTag)
				if err != nil {
					return fmt.Errorf("filter processor Init(): error making field searcher - %w", err)
				}

				searcherList = append(searcherList, searcher)
			}

			ff := fields.Filter{
				Op:        fields.Operation(key),
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
