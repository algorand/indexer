package filterprocessor

// FilterExpressionType is the type of the filter (i.e. const, regex, etc)
type FilterExpressionType string

const (
	// ConstFilter a filter that looks at a constant string in its entirety
	ConstFilter FilterExpressionType = "const"
	// RegexFilter a filter that applies regex rules to the matching
	RegexFilter FilterExpressionType = "regex"
)

// SubConfig is the configuration needed for each additional filter
type SubConfig struct {
	// The tag of the struct to analyze
	FilterTag string `yaml:"tag"`
	// The type of expression to search for "const" or "regex"
	ExpressionType FilterExpressionType `yaml:"expression-type"`
	// The expression to search
	Expression string `yaml:"expression"`
}

// Config configuration for the filter processor
type Config struct {
	Filters []map[string][]SubConfig `yaml:"filters"`
}
