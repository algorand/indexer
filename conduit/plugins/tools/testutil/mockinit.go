package testutil

import sdk "github.com/algorand/go-algorand-sdk/v2/types"

// MockInitProvider mock an init provider
type MockInitProvider struct {
	CurrentRound *sdk.Round
	Genesis      *sdk.Genesis
}

// GetGenesis produces genesis pointer
func (m *MockInitProvider) GetGenesis() *sdk.Genesis {
	return m.Genesis
}

// NextDBRound provides next database round
func (m *MockInitProvider) NextDBRound() sdk.Round {
	return *m.CurrentRound
}

// MockedInitProvider returns an InitProvider for testing
func MockedInitProvider(round *sdk.Round) *MockInitProvider {
	return &MockInitProvider{
		CurrentRound: round,
		Genesis:      &sdk.Genesis{},
	}
}
