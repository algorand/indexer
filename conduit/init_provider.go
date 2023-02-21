package conduit

import (
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// PipelineInitProvider algod based init provider
type PipelineInitProvider struct {
	currentRound *sdk.Round
	genesis      *sdk.Genesis
}

// MakePipelineInitProvider constructs an init provider.
func MakePipelineInitProvider(currentRound *sdk.Round, genesis *sdk.Genesis) *PipelineInitProvider {
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
func (a *PipelineInitProvider) NextDBRound() sdk.Round {
	return *a.currentRound
}
