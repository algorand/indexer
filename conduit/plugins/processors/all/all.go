package all

import (
	// Call package wide init function
	_ "github.com/algorand/indexer/conduit/plugins/processors/blockprocessor"
	_ "github.com/algorand/indexer/conduit/plugins/processors/filterprocessor"
	_ "github.com/algorand/indexer/conduit/plugins/processors/noop"
)
