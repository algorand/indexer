package conduit

import "github.com/algorand/indexer/fetcher"

// pipeline is a struct that orchestrates the entire
// sequence of events, taking in importers, processors and
// exporters and generating the result
type pipeline struct {

	importer *fetcher.Fetcher

}
