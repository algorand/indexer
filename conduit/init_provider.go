package conduit

import (
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/data/basics"
)

// PipelineInitProvider algod based init provider
type PipelineInitProvider struct {
	currentRound *basics.Round
	genesis      *sdk.Genesis
}

// MakePipelineInitProvider constructs an init provider.
func MakePipelineInitProvider(currentRound *basics.Round, genesis *sdk.Genesis) *PipelineInitProvider {
	return &PipelineInitProvider{
		currentRound: currentRound,
		genesis:      genesis,
	}
}

// GetGenesis produces genesis pointer
func (a *PipelineInitProvider) GetGenesis() *sdk.Genesis {
	return a.genesis
}

// NextDBRound provides next database round
func (a *PipelineInitProvider) NextDBRound() basics.Round {
	return *a.currentRound
}
