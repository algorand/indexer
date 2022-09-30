package all

import (
	// Call package wide init function
	_ "github.com/algorand/indexer/importers/algod"
	_ "github.com/algorand/indexer/importers/filereader"
)
