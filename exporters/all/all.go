package all

import (
	// Call package wide init function
	_ "github.com/algorand/indexer/exporters/filewriter"
	_ "github.com/algorand/indexer/exporters/noop"
	_ "github.com/algorand/indexer/exporters/postgresql"
	_ "github.com/algorand/indexer/exporters/slack"
)
