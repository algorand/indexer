package accounting

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/indexer/api/generated/v2"
	"github.com/stretchr/testify/require"
)

var protoV24 config.ConsensusParams

type minBalVector struct{ assets, appParams, localStates, uints, byteSlices, extraAppPages uint64 }

func asVector(a *basics.AccountData) minBalVector {
	if a == nil {
		return minBalVector{}
	}
	return minBalVector{
		assets:        uint64(len(a.Assets)),
		appParams:     uint64(len(a.AppParams)),
		localStates:   uint64(len(a.AppLocalStates)),
		uints:         a.TotalAppSchema.NumUint,
		byteSlices:    a.TotalAppSchema.NumByteSlice,
		extraAppPages: uint64(a.TotalExtraAppPages),
	}
}

// DeepCopy uses gob to marshall and unmarshall an object into a deep copy.
// From: https://xuri.me/2018/06/17/deep-copy-object-with-reflecting-or-gob-in-go.html
func DeepCopy(src, dst interface{}) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(src); err != nil {
		return err
	}
	return gob.NewDecoder(bytes.NewBuffer(buf.Bytes())).Decode(dst)
}

// calculateMinBalance is a sanity checker that takes a linear combination
// of the cost weights of the consensus params with an account's cost attributes (aka minBalVector)
func minBalanceLinearCombo(proto *config.ConsensusParams, vect minBalVector) basics.MicroAlgos {
	raw := proto.MinBalance*(1+vect.assets) +
		proto.AppFlatParamsMinBalance*(vect.appParams+vect.extraAppPages) +
		proto.AppFlatOptInMinBalance*vect.localStates +
		proto.SchemaMinBalancePerEntry*(vect.uints+vect.byteSlices) +
		proto.SchemaUintMinBalance*vect.uints +
		proto.SchemaBytesMinBalance*vect.byteSlices

	return basics.MicroAlgos{Raw: raw}
}

type testCase struct {
	name     string
	gAccount *generated.Account
	expected basics.AccountData
	vect     minBalVector
}

var testCases []testCase

func TestMinBalanceProjection(t *testing.T) {
	for i, tCase := range testCases {
		gAccount := tCase.gAccount
		actual := minBalanceProjection(gAccount)
		require.Equal(t, tCase.expected, actual, "failed generated v. basic account for test case %d: %s", i+1, tCase.name)
		vect := asVector(&actual)
		require.Equal(t, tCase.vect, vect, "expected essential minbalance information failed for test case %d: %s", i+1, tCase.name)
	}
}

func TestMinBalanceComputation(t *testing.T) {
	for i, tCase := range testCases {
		mbVect := tCase.vect
		expected := minBalanceLinearCombo(&protoV24, mbVect)
		actual := tCase.expected.MinBalance(&protoV24)
		require.Equal(t, expected, actual, "failed equality for testcase %d: %+v", i, mbVect)
	}
}

func TestMinBalanceEnricher(t *testing.T) {
	blockheader := bookkeeping.BlockHeader{
		UpgradeState: bookkeeping.UpgradeState{
			CurrentProtocol: protocol.ConsensusV24,
		},
	}

	for i, tCase := range testCases {
		expected := minBalanceLinearCombo(&protoV24, tCase.vect).Raw
		gAccountMutate := generated.Account{}
		err := DeepCopy(tCase.gAccount, &gAccountMutate)
		require.NoErrorf(t, err, "failed to deep copy the generated account for testcase %d [%s]: %v", i, tCase.name, err)
		require.Nil(t, gAccountMutate.MinBalance, "generated account's MinBalance shouldn't be pre-populate but is for testcase %d [%s]", i, tCase.name)

		EnrichMinBalance(&gAccountMutate, &blockheader)
		require.NotNil(t, gAccountMutate.MinBalance, "failed to populate generated account's MinBalance field for testcase %d [%s]", i, tCase.name)
		actual := *gAccountMutate.MinBalance
		require.Equal(t, expected, actual, "MinBalance was not computed correctly for test case %d [%s]", i, tCase.name)
	}

}

func init() {
	protoV24 = config.Consensus[protocol.ConsensusV24]

	trueVal := true
	falseVal := false
	roundVal := uint64(17)
	extraPagesVal29 := uint64(29)
	extraPagesVal129 := uint64(129)
	extraPagesVal229 := uint64(229)
	// extraPagesVal329 := uint64(329)

	testCases = []testCase{
		// empty case:
		{"nada", &generated.Account{}, basics.AccountData{}, minBalVector{}},

		// deletions:
		{
			"three more typical assets - DELETED!!!!",
			&generated.Account{
				Deleted: &trueVal,
				Assets: &[]generated.AssetHolding{
					{AssetId: 1337, Amount: 1400},
					{AssetId: 42, Amount: 40},
					{AssetId: 2001, Amount: 2001000},
				},
			},
			basics.AccountData{},
			minBalVector{},
		},
		{
			"three more typical assets - NOT deleted!!!!",
			&generated.Account{
				Deleted: &falseVal,
				Assets: &[]generated.AssetHolding{
					{AssetId: 1337},
					{AssetId: 42},
					{AssetId: 2001},
				},
			},
			basics.AccountData{Assets: map[basics.AssetIndex]basics.AssetHolding{
				basics.AssetIndex(1337): {},
				basics.AssetIndex(42):   {},
				basics.AssetIndex(2001): {},
			}},
			minBalVector{3, 0, 0, 0, 0, 0},
		},

		// test totalAppSchema
		{
			"totalAppSchema non-zero",
			&generated.Account{AppsTotalSchema: &generated.ApplicationStateSchema{NumUint: 3453, NumByteSlice: 876}},
			basics.AccountData{TotalAppSchema: basics.StateSchema{NumUint: 3453, NumByteSlice: 876}},
			minBalVector{0, 0, 0, 3453, 876, 0},
		},
		{
			"totalAppSchema non-zero - Deleted",
			&generated.Account{
				Deleted:         &trueVal,
				AppsTotalSchema: &generated.ApplicationStateSchema{NumUint: 3453, NumByteSlice: 876},
			},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},

		// test totalExtraAppPages
		{
			"totalExtraAppPages non-zero",
			&generated.Account{AppsTotalExtraPages: &extraPagesVal29},
			basics.AccountData{TotalExtraAppPages: 29},
			minBalVector{0, 0, 0, 0, 0, 29},
		},
		{
			"totalExtraAppPages non-zero - Deleted",
			&generated.Account{
				Deleted:             &trueVal,
				AppsTotalExtraPages: &extraPagesVal29,
			},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},

		// test convertAssetHoldings:
		{"empty assets", &generated.Account{Assets: &[]generated.AssetHolding{}}, basics.AccountData{}, minBalVector{}},
		{
			"one zero asset",
			&generated.Account{Assets: &[]generated.AssetHolding{{AssetId: 1337}}},
			basics.AccountData{Assets: map[basics.AssetIndex]basics.AssetHolding{basics.AssetIndex(1337): {}}},
			minBalVector{1, 0, 0, 0, 0, 0},
		},
		{
			"one frozen zero asset",
			&generated.Account{Assets: &[]generated.AssetHolding{{AssetId: 1337, IsFrozen: true}}},
			basics.AccountData{Assets: map[basics.AssetIndex]basics.AssetHolding{basics.AssetIndex(1337): {}}},
			minBalVector{1, 0, 0, 0, 0, 0},
		},
		{
			"one deleted zero asset",
			&generated.Account{Assets: &[]generated.AssetHolding{{AssetId: 1337, Deleted: &trueVal}}},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"one frozen deleted zero asset",
			&generated.Account{Assets: &[]generated.AssetHolding{{AssetId: 1337, IsFrozen: true, Deleted: &trueVal}}},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"three more typical assets",
			&generated.Account{Assets: &[]generated.AssetHolding{
				{AssetId: 1337, Amount: 1400},
				{AssetId: 42, Amount: 40},
				{AssetId: 2001, Amount: 2001000},
			}},
			basics.AccountData{Assets: map[basics.AssetIndex]basics.AssetHolding{
				basics.AssetIndex(1337): {},
				basics.AssetIndex(42):   {},
				basics.AssetIndex(2001): {},
			}},
			minBalVector{3, 0, 0, 0, 0, 0},
		},
		{
			"five assets with two dupes",
			&generated.Account{Assets: &[]generated.AssetHolding{
				{AssetId: 1337, Amount: 1400},
				{AssetId: 42, Amount: 40},
				{AssetId: 2001, Amount: 2001000},
				{AssetId: 42, Amount: 4000},
				{AssetId: 2001, Amount: 1000},
			}},
			basics.AccountData{Assets: map[basics.AssetIndex]basics.AssetHolding{
				basics.AssetIndex(1337): {},
				basics.AssetIndex(42):   {},
				basics.AssetIndex(2001): {},
			}},
			minBalVector{3, 0, 0, 0, 0, 0},
		},

		// test convertAppsCreated:
		{
			"created one simple app",
			&generated.Account{CreatedApps: &[]generated.Application{{Id: 1337, Params: generated.ApplicationParams{}}}},
			basics.AccountData{AppParams: map[basics.AppIndex]basics.AppParams{basics.AppIndex(1337): {}}},
			minBalVector{0, 1, 0, 0, 0, 0},
		},
		{
			"created one simple app without app params",
			&generated.Account{CreatedApps: &[]generated.Application{{Id: 1337}}},
			basics.AccountData{AppParams: map[basics.AppIndex]basics.AppParams{basics.AppIndex(1337): {}}},
			minBalVector{0, 1, 0, 0, 0, 0},
		},
		{
			"created one app but deleted",
			&generated.Account{CreatedApps: &[]generated.Application{{Id: 1337, Deleted: &trueVal}}},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"created one app but deleted at round",
			&generated.Account{CreatedApps: &[]generated.Application{{Id: 1337, DeletedAtRound: &roundVal}}},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"created one app with extra pages - ignore from CreatedApps",
			&generated.Account{CreatedApps: &[]generated.Application{{Id: 1337, Params: generated.ApplicationParams{ExtraProgramPages: &extraPagesVal29}}}},
			basics.AccountData{AppParams: map[basics.AppIndex]basics.AppParams{basics.AppIndex(1337): {}}, TotalExtraAppPages: 0},
			minBalVector{0, 1, 0, 0, 0, 0},
		},
		{
			"created one app with extra pages but deleted",
			&generated.Account{CreatedApps: &[]generated.Application{{Id: 1337, Params: generated.ApplicationParams{ExtraProgramPages: &extraPagesVal29}, Deleted: &trueVal}}},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"created one app with extra pages but deleted at round",
			&generated.Account{CreatedApps: &[]generated.Application{{Id: 1337, Params: generated.ApplicationParams{ExtraProgramPages: &extraPagesVal29}, DeletedAtRound: &roundVal}}},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"created one app with global schema - ignore from CreatedApps",
			&generated.Account{
				CreatedApps: &[]generated.Application{
					{
						Id: 1337,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
						},
					},
				},
			},
			basics.AccountData{
				AppParams:      map[basics.AppIndex]basics.AppParams{basics.AppIndex(1337): {}},
				TotalAppSchema: basics.StateSchema{NumUint: 0, NumByteSlice: 0},
			},
			minBalVector{0, 1, 0, 0, 0, 0},
		},
		{
			"created one app with global schema but deleted",
			&generated.Account{
				CreatedApps: &[]generated.Application{
					{
						Id: 1337,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
						},
						Deleted: &trueVal,
					},
				},
			},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"created one app with global schema but deleted at round",
			&generated.Account{
				CreatedApps: &[]generated.Application{
					{
						Id: 1337,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
						},
						DeletedAtRound: &roundVal,
					},
				},
			},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"created one app with global schema and extra pages - ignore from CreatedApps",
			&generated.Account{
				CreatedApps: &[]generated.Application{
					{
						Id: 1337,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
							ExtraProgramPages: &extraPagesVal29,
						},
					},
				},
			},
			basics.AccountData{
				AppParams:          map[basics.AppIndex]basics.AppParams{basics.AppIndex(1337): {}},
				TotalAppSchema:     basics.StateSchema{NumUint: 0, NumByteSlice: 0},
				TotalExtraAppPages: 0,
			},
			minBalVector{0, 1, 0, 0, 0, 0},
		},
		{
			"four typical created apps - ignore from CreatedApps",
			&generated.Account{
				CreatedApps: &[]generated.Application{
					{
						Id: 1337,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
							ExtraProgramPages: &extraPagesVal29,
						},
					},
					{
						Id: 1338,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
							ExtraProgramPages: &extraPagesVal29,
						},
					},
					{
						Id: 1339,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
							ExtraProgramPages: &extraPagesVal29,
						},
					},
					{
						Id: 1340,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
							ExtraProgramPages: &extraPagesVal29,
						},
					},
				},
			},
			basics.AccountData{
				AppParams: map[basics.AppIndex]basics.AppParams{
					basics.AppIndex(1337): {},
					basics.AppIndex(1338): {},
					basics.AppIndex(1339): {},
					basics.AppIndex(1340): {},
				},
				TotalAppSchema:     basics.StateSchema{NumUint: 0, NumByteSlice: 0},
				TotalExtraAppPages: 0,
			},
			minBalVector{0, 4, 0, 0, 0, 0},
		},
		{
			"four created apps with two duplicate appids - ignore from CreatedApps",
			&generated.Account{
				CreatedApps: &[]generated.Application{
					{
						Id: 1337,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 11, NumByteSlice: 15},
							ExtraProgramPages: &extraPagesVal129,
						},
					},
					{
						Id: 1338,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 10, NumByteSlice: 20},
							ExtraProgramPages: &extraPagesVal229,
						},
					},
					{
						Id: 1337,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
							ExtraProgramPages: &extraPagesVal29,
						},
					},
					{
						Id: 1337,
						Params: generated.ApplicationParams{
							GlobalStateSchema: &generated.ApplicationStateSchema{NumUint: 17, NumByteSlice: 117},
							ExtraProgramPages: &extraPagesVal29,
						},
					},
				},
			},
			basics.AccountData{
				AppParams: map[basics.AppIndex]basics.AppParams{
					basics.AppIndex(1337): {},
					basics.AppIndex(1338): {},
				},
				TotalAppSchema:     basics.StateSchema{NumUint: 0, NumByteSlice: 0},
				TotalExtraAppPages: 0,
			},
			minBalVector{0, 2, 0, 0, 0, 0},
		},

		// test convertAppsOptedIn:
		{
			"opted into one simple app",
			&generated.Account{AppsLocalState: &[]generated.ApplicationLocalState{{Id: 1337}}},
			basics.AccountData{AppLocalStates: map[basics.AppIndex]basics.AppLocalState{
				basics.AppIndex(1337): {},
			}},
			minBalVector{0, 0, 1, 0, 0, 0},
		},
		{
			"opted into one simple app - deleted",
			&generated.Account{AppsLocalState: &[]generated.ApplicationLocalState{{Id: 1337, Deleted: &trueVal}}},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"opted into one simple app - closed out at round",
			&generated.Account{AppsLocalState: &[]generated.ApplicationLocalState{{Id: 1337, ClosedOutAtRound: &roundVal}}},
			basics.AccountData{},
			minBalVector{0, 0, 0, 0, 0, 0},
		},
		{
			"five typical opted in apps",
			&generated.Account{
				AppsLocalState: &[]generated.ApplicationLocalState{
					{Id: 1337},
					{Id: 1338},
					{Id: 1339},
					{Id: 13310},
					{Id: 13311},
				},
			},
			basics.AccountData{
				AppLocalStates: map[basics.AppIndex]basics.AppLocalState{
					basics.AppIndex(13311): {},
					basics.AppIndex(13310): {},
					basics.AppIndex(1339):  {},
					basics.AppIndex(1338):  {},
					basics.AppIndex(1337):  {},
				},
			},
			minBalVector{0, 0, 5, 0, 0, 0},
		},
		{
			"five opted in apps with 3 dupes",
			&generated.Account{
				AppsLocalState: &[]generated.ApplicationLocalState{
					{Id: 13311},
					{Id: 1338},
					{Id: 13311},
					{Id: 1338},
					{Id: 13311},
				},
			},
			basics.AccountData{
				AppLocalStates: map[basics.AppIndex]basics.AppLocalState{
					basics.AppIndex(13311): {},
					basics.AppIndex(1338):  {},
				},
			},
			minBalVector{0, 0, 2, 0, 0, 0},
		},
		{
			"five opted in apps with 3 dupes - first is deleted",
			&generated.Account{
				AppsLocalState: &[]generated.ApplicationLocalState{
					{Id: 13311},
					{Id: 1338, Deleted: &trueVal},
					{Id: 13311},
					{Id: 1338},
					{Id: 13311},
				},
			},
			basics.AccountData{
				AppLocalStates: map[basics.AppIndex]basics.AppLocalState{
					basics.AppIndex(13311): {},
					basics.AppIndex(1338):  {},
				},
			},
			minBalVector{0, 0, 2, 0, 0, 0},
		},

		// smorgasbord
		{
			"an everything bagel",
			&generated.Account{
				Assets: &[]generated.AssetHolding{
					{AssetId: 1337, Amount: 1400},
					{AssetId: 42, Amount: 40},
					{AssetId: 2001, Amount: 2001000},
				},
				CreatedApps: &[]generated.Application{
					{Id: 10},
					{Id: 8},
					{Id: 12},
					{Id: 13},
					{Id: 107},
					{Id: 1337},
					{Id: 4},
				},
				AppsLocalState: &[]generated.ApplicationLocalState{
					{Id: 1337},
					{Id: 1338},
					{Id: 1339},
					{Id: 13310},
					{Id: 13311},
				},
				AppsTotalSchema: &generated.ApplicationStateSchema{
					NumUint:      3453,
					NumByteSlice: 876,
				},
				AppsTotalExtraPages: &extraPagesVal229,
			},
			basics.AccountData{
				Assets: map[basics.AssetIndex]basics.AssetHolding{
					basics.AssetIndex(1337): {},
					basics.AssetIndex(42):   {},
					basics.AssetIndex(2001): {},
				},
				AppParams: map[basics.AppIndex]basics.AppParams{
					basics.AppIndex(4):    {},
					basics.AppIndex(8):    {},
					basics.AppIndex(10):   {},
					basics.AppIndex(12):   {},
					basics.AppIndex(13):   {},
					basics.AppIndex(107):  {},
					basics.AppIndex(1337): {},
				},
				AppLocalStates: map[basics.AppIndex]basics.AppLocalState{
					basics.AppIndex(13311): {},
					basics.AppIndex(13310): {},
					basics.AppIndex(1339):  {},
					basics.AppIndex(1338):  {},
					basics.AppIndex(1337):  {},
				},
				TotalAppSchema: basics.StateSchema{
					NumUint:      3453,
					NumByteSlice: 876,
				},
				TotalExtraAppPages: 229,
			},
			minBalVector{3, 7, 5, 3453, 876, 229},
		},
	}
}
