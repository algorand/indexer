package types

import (
	"github.com/algorand/go-algorand-sdk/v2/protocol/config"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// stateSchemaMinBalance computes the MinBalance requirements for a StateSchema
// based on the consensus parameters
func stateSchemaMinBalance(sm sdk.StateSchema, proto *config.ConsensusParams) uint64 {
	// Flat cost for each key/value pair
	flatCost := proto.SchemaMinBalancePerEntry * (sm.NumUint + sm.NumByteSlice)

	// Cost for uints
	uintCost := proto.SchemaUintMinBalance * sm.NumUint

	// Cost for byte slices
	bytesCost := proto.SchemaBytesMinBalance * sm.NumByteSlice

	// Sum the separate costs
	return flatCost + uintCost + bytesCost
}

// minBalance computes the minimum balance requirements for an account based on
// some consensus parameters. MinBalance should correspond roughly to how much
// storage the account is allowed to store on disk.
func minBalance(
	proto *config.ConsensusParams,
	totalAssets uint64,
	totalAppSchema sdk.StateSchema,
	totalAppParams uint64, totalAppLocalStates uint64,
	totalExtraAppPages uint64,
	totalBoxes uint64, totalBoxBytes uint64,
) uint64 {
	var minBal uint64

	// First, base MinBalance
	minBal = proto.MinBalance

	// MinBalance for each Asset
	assetCost := proto.MinBalance * totalAssets
	minBal += assetCost

	// Base MinBalance for each created application
	appCreationCost := proto.AppFlatParamsMinBalance * totalAppParams
	minBal += appCreationCost

	// Base MinBalance for each opted in application
	appOptInCost := proto.AppFlatOptInMinBalance * totalAppLocalStates
	minBal += appOptInCost

	// MinBalance for state usage measured by LocalStateSchemas and
	// GlobalStateSchemas
	schemaCost := stateSchemaMinBalance(totalAppSchema, proto)
	minBal += schemaCost

	// MinBalance for each extra app program page
	extraAppProgramLenCost := proto.AppFlatParamsMinBalance * totalExtraAppPages
	minBal += extraAppProgramLenCost

	// Base MinBalance for each created box
	boxBaseCost := proto.BoxFlatMinBalance * totalBoxes
	minBal += boxBaseCost

	// Per byte MinBalance for boxes
	boxByteCost := proto.BoxByteMinBalance * totalBoxBytes
	minBal += boxByteCost

	return minBal
}

// AccountMinBalance computes the minimum balance requirements for an account
// based on some consensus parameters. MinBalance should correspond roughly to
// how much storage the account is allowed to store on disk.
func AccountMinBalance(account sdk.AccountData, proto *config.ConsensusParams) uint64 {
	return minBalance(
		proto,
		account.TotalAssets,
		account.TotalAppSchema,
		account.TotalAppParams, account.TotalAppLocalStates,
		uint64(account.TotalExtraAppPages),
		account.TotalBoxes, account.TotalBoxBytes,
	)
}
