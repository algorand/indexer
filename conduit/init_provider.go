package conduit

import (
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
)

type AlgodInitProvider struct {
	currentRound basics.Round
}

func (a *AlgodInitProvider) AdvanceDBRound() {
	a.currentRound = a.currentRound + 1
}

func (a *AlgodInitProvider) Genesis() *bookkeeping.Genesis {
	return &bookkeeping.Genesis{}
}

func (a *AlgodInitProvider) NextDBRound() basics.Round {
	return a.currentRound
}
