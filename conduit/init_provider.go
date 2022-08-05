package conduit

import (
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
)

// AlgodInitProvider algod based init provider
type AlgodInitProvider struct {
	currentRound basics.Round
}

// AdvanceDBRound advances the database round
func (a *AlgodInitProvider) AdvanceDBRound() {
	a.currentRound = a.currentRound + 1
}

// Genesis produces genesis pointer
func (a *AlgodInitProvider) Genesis() *bookkeeping.Genesis {
	return &bookkeeping.Genesis{}
}

// NextDBRound provides next database round
func (a *AlgodInitProvider) NextDBRound() basics.Round {
	return a.currentRound
}
