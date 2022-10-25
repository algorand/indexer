package conduit

import "github.com/algorand/indexer/data"

// OnCompleteFunc is the signature for the Completed functional interface.
type OnCompleteFunc func(input data.BlockData) error

// Completed is called by the conduit pipeline after every exporter has
// finished. It can be used for things like finalizing state.
type Completed interface {
	// OnComplete will be called by the Conduit framework when the pipeline
	// finishes processing a round.
	OnComplete(input data.BlockData) error
}
