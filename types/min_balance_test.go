package types

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/algorand/go-algorand-sdk/v2/protocol/config"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

func TestMinBalance(t *testing.T) {
	testConsensusParams := &config.ConsensusParams{
		MinBalance:               100000,
		AppFlatParamsMinBalance:  100000,
		AppFlatOptInMinBalance:   100000,
		SchemaMinBalancePerEntry: 25000,
		SchemaUintMinBalance:     3500,
		SchemaBytesMinBalance:    25000,
		BoxFlatMinBalance:        2500,
		BoxByteMinBalance:        400,
	}

	tests := []struct {
		name                string
		expectedResult      uint64
		proto               *config.ConsensusParams
		totalAssets         uint64
		totalAppSchema      sdk.StateSchema
		totalAppParams      uint64
		totalAppLocalStates uint64
		totalExtraAppPages  uint64
		totalBoxes          uint64
		totalBoxBytes       uint64
	}{
		{
			"Passing all 0s/empties to minBalance",
			0,
			&config.ConsensusParams{},
			0,
			sdk.StateSchema{},
			0,
			0,
			0,
			0,
			0,
		},
		{
			"Base Case: Use non-zero consensus minBalance with otherwise 0s/empties",
			100000,
			testConsensusParams,
			0,
			sdk.StateSchema{},
			0,
			0,
			0,
			0,
			0,
		},
		{
			"Base Case with non-zero totalAssets",
			testConsensusParams.MinBalance + (testConsensusParams.MinBalance * 20),
			testConsensusParams,
			20,
			sdk.StateSchema{},
			0,
			0,
			0,
			0,
			0,
		},
		{
			"Layering in created applications",
			testConsensusParams.MinBalance + (testConsensusParams.MinBalance * 20) +
				(testConsensusParams.AppFlatParamsMinBalance * 30),
			testConsensusParams,
			20,
			sdk.StateSchema{},
			30,
			0,
			0,
			0,
			0,
		},
		{
			"Layering in opted in applications",
			testConsensusParams.MinBalance + (testConsensusParams.MinBalance * 20) +
				(testConsensusParams.AppFlatParamsMinBalance * 30) + (testConsensusParams.AppFlatOptInMinBalance * 5),
			testConsensusParams,
			20,
			sdk.StateSchema{},
			30,
			5,
			0,
			0,
			0,
		},
		{
			"Including State Usage Costs",
			testConsensusParams.MinBalance + (testConsensusParams.MinBalance * 20) +
				(testConsensusParams.AppFlatParamsMinBalance * 30) + (testConsensusParams.AppFlatOptInMinBalance * 5) +
				(testConsensusParams.SchemaMinBalancePerEntry * (500 + 1000)) +
				(testConsensusParams.SchemaUintMinBalance * 500) +
				(testConsensusParams.SchemaBytesMinBalance * 1000),
			testConsensusParams,
			20,
			sdk.StateSchema{
				NumUint:      500,
				NumByteSlice: 1000,
			},
			30,
			5,
			0,
			0,
			0,
		},
		{
			"Including Extra App Pages",
			testConsensusParams.MinBalance + (testConsensusParams.MinBalance * 20) +
				(testConsensusParams.AppFlatParamsMinBalance * 30) + (testConsensusParams.AppFlatOptInMinBalance * 5) +
				(testConsensusParams.SchemaMinBalancePerEntry * (500 + 1000)) +
				(testConsensusParams.SchemaUintMinBalance * 500) +
				(testConsensusParams.SchemaBytesMinBalance * 1000) +
				(testConsensusParams.AppFlatParamsMinBalance * 300),
			testConsensusParams,
			20,
			sdk.StateSchema{
				NumUint:      500,
				NumByteSlice: 1000,
			},
			30,
			5,
			300,
			0,
			0,
		},
		{
			"Add in Total Boxes and Bytes",
			testConsensusParams.MinBalance + (testConsensusParams.MinBalance * 20) +
				(testConsensusParams.AppFlatParamsMinBalance * 30) + (testConsensusParams.AppFlatOptInMinBalance * 5) +
				(testConsensusParams.SchemaMinBalancePerEntry * (500 + 1000)) +
				(testConsensusParams.SchemaUintMinBalance * 500) +
				(testConsensusParams.SchemaBytesMinBalance * 1000) +
				(testConsensusParams.AppFlatParamsMinBalance * 300) +
				(testConsensusParams.BoxFlatMinBalance * 8) +
				(testConsensusParams.BoxByteMinBalance * 7500),
			testConsensusParams,
			20,
			sdk.StateSchema{
				NumUint:      500,
				NumByteSlice: 1000,
			},
			30,
			5,
			300,
			8,
			7500,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := minBalance(
				test.proto,
				test.totalAssets,
				test.totalAppSchema,
				test.totalAppParams,
				test.totalAppLocalStates,
				test.totalExtraAppPages,
				test.totalBoxes,
				test.totalBoxBytes,
			)

			assert.Equal(t, test.expectedResult, result)
		})
	}
}
