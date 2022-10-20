package conduit

import "github.com/algorand/indexer/data"

type OnCompleteFunc func(input data.BlockData) error

type Completed interface {
	// OnComplete will be called by the Conduit framework when the pipeline
	// finishes processing a round. It can be used for things like finalizing
	// state.
	OnComplete(input data.BlockData) error
}
