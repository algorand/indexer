package filterprocessor

import "github.com/algorand/indexer/processors/filterprocessor/expression"

// SubConfig is the configuration needed for each additional filter
type SubConfig struct {
	// The tag of the struct to analyze
	FilterTag string `yaml:"tag"`
	// The type of expression to search for "const" or "regex"
	ExpressionType expression.FilterType `yaml:"expression-type"`
	// The expression to search
	Expression string `yaml:"expression"`
}

// Config configuration for the filter processor
type Config struct {
	Filters []map[string][]SubConfig `yaml:"filters"`
}
