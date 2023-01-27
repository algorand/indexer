// Package filterprocessor docs
package filterprocessor

//go:generate conduit-docs ../../../../conduit-docs/

import (
	"github.com/algorand/indexer/conduit/plugins/processors/filterprocessor/expression"
)

//Name: conduit_processors_filter

// SubConfig is the configuration needed for each additional filter
type SubConfig struct {
	/* <code>tag</code> is the tag of the struct to analyze.<br/>
	It can be of the form `txn.*` where the specific ending is determined by the field you wish to filter on.<br/>
	It can also be a field in the ApplyData.
	*/
	FilterTag string `yaml:"tag"`
	/* <code>expression-type</code> is the type of comparison applied between the field, identified by the tag, and the expression.<br/>
	<ul>
		<li>exact</li>
		<li>regex</li>
		<li>less-than</li>
		<li>less-than-equal</li>
		<li>greater-than</li>
		<li>great-than-equal</li>
		<li>equal</li>
		<li>not-equal</li>
	</ul>
	*/
	ExpressionType expression.FilterType `yaml:"expression-type"`
	// <code>expression</code> is the user-supplied part of the search or comparison.
	Expression string `yaml:"expression"`
}

// Config configuration for the filter processor
type Config struct {
	/* <code>filters</code> are a list of SubConfig objects with an operation acting as the string key in the map

	filters:
		- [any,all,none]:
			expression: ""
			expression-type: ""
			tag: ""
	*/
	Filters []map[string][]SubConfig `yaml:"filters"`
}
