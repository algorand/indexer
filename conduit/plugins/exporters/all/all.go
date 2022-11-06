package all

import (
	// Call package wide init function
	_ "github.com/algorand/indexer/conduit/plugins/exporters/filewriter"
	_ "github.com/algorand/indexer/conduit/plugins/exporters/noop"
	_ "github.com/algorand/indexer/conduit/plugins/exporters/postgresql"
)
