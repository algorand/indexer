package all

import (
	// Call package wide init function
	_ "github.com/algorand/indexer/processors/blockprocessor"
	_ "github.com/algorand/indexer/processors/noop"
)
