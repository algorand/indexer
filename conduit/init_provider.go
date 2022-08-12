package conduit

import (
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
)

// PipelineInitProvider algod based init provider
type PipelineInitProvider struct {
	currentRound *basics.Round
	genesis      *bookkeeping.Genesis
}

// Genesis produces genesis pointer
func (a *PipelineInitProvider) Genesis() *bookkeeping.Genesis {
	return a.genesis
}

// NextDBRound provides next database round
func (a *PipelineInitProvider) NextDBRound() basics.Round {
	return *a.currentRound
}
