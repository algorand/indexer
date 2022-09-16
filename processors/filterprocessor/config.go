package filterprocessor

import "github.com/algorand/indexer/processors/filterprocessor/expression"

// SubConfig is the configuration needed for each additional filter
type SubConfig struct {
	// FilterTag the tag of the struct to analyze
	FilterTag string `yaml:"tag"`
	// ExpressionType the type of expression to search for (i.e. "exact" or "regex")
	ExpressionType expression.FilterType `yaml:"expression-type"`
	// Expression the expression to search
	Expression string `yaml:"expression"`
}

// Config configuration for the filter processor
type Config struct {
	// Filters are a list of SubConfig objects with an operation acting as the string key in the map
	Filters []map[string][]SubConfig `yaml:"filters"`
}
