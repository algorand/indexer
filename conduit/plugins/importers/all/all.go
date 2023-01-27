package all

import (
	// Call package wide init function
	_ "github.com/algorand/indexer/conduit/plugins/importers/algod"
	_ "github.com/algorand/indexer/conduit/plugins/importers/algodfollower"
	_ "github.com/algorand/indexer/conduit/plugins/importers/filereader"
)
