package eval_test

import (
	"crypto/rand"
	"fmt"
	"testing"

	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/avm-abi/apps"
	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/rpcs"

	block_processor "github.com/algorand/indexer/conduit/plugins/processors/blockprocessor"
	indxLedger "github.com/algorand/indexer/conduit/plugins/processors/blockprocessor/internal/eval"
	"github.com/algorand/indexer/util/test"
)

func makeTestLedger(t *testing.T) *ledger.Ledger {
	logger, _ := test2.NewNullLogger()
	l, err := test.MakeTestLedger(logger)
	require.NoError(t, err)
	return l
}

func TestLedgerForEvaluatorLatestBlockHdr(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)
	txn := test.MakePaymentTxn(0, 100, 0, 1, 1,
		0, test.AccountA, test.AccountA, basics.Address{}, basics.Address{})
	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err := ld.LatestBlockHdr()
	require.NoError(t, err)

	assert.Equal(t, block.BlockHeader, ret)
}

func TestLedgerForEvaluatorAccountDataBasic(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	accountData, _, err := l.LookupWithoutRewards(0, test.AccountB)
	require.NoError(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err :=
		ld.LookupWithoutRewards(map[basics.Address]struct{}{test.AccountB: {}})
	require.NoError(t, err)

	accountDataRet := ret[test.AccountB]
	require.NotNil(t, accountDataRet)
	assert.Equal(t, accountData, *accountDataRet)
}

func TestLedgerForEvaluatorAccountDataMissingAccount(t *testing.T) {
	l := makeTestLedger(t)
	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer l.Close()
	defer ld.Close()

	var addr basics.Address
	rand.Read(addr[:])
	ret, err :=
		ld.LookupWithoutRewards(map[basics.Address]struct{}{addr: {}})
	require.NoError(t, err)

	accountDataRet := ret[addr]
	assert.Nil(t, accountDataRet)
}

func TestLedgerForEvaluatorAsset(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeAssetConfigTxn(0, 2, 0, false, "", "", "", test.AccountA)
	txn1 := test.MakeAssetConfigTxn(0, 4, 0, false, "", "", "", test.AccountA)
	txn2 := test.MakeAssetConfigTxn(0, 6, 0, false, "", "", "", test.AccountA)
	txn3 := test.MakeAssetConfigTxn(0, 8, 0, false, "", "", "", test.AccountB)
	txn4 := test.MakeAssetDestroyTxn(1, test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0, &txn1, &txn2, &txn3, &txn4)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err :=
		ld.LookupResources(map[basics.Address]map[ledger.Creatable]struct{}{
			test.AccountA: {
				{Index: 1, Type: basics.AssetCreatable}: {},
				{Index: 2, Type: basics.AssetCreatable}: {},
				{Index: 3, Type: basics.AssetCreatable}: {},
			},
			test.AccountB: {
				{Index: 4, Type: basics.AssetCreatable}: {},
			},
		})
	require.NoError(t, err)

	expected := map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource{
		test.AccountA: {
			ledger.Creatable{Index: 1, Type: basics.AssetCreatable}: {},
			ledger.Creatable{Index: 2, Type: basics.AssetCreatable}: {
				AssetHolding: &basics.AssetHolding{
					Amount: txn1.Txn.AssetParams.Total,
					Frozen: txn1.Txn.AssetFrozen,
				},
				AssetParams: &txn1.Txn.AssetParams,
			},
			ledger.Creatable{Index: 3, Type: basics.AssetCreatable}: {
				AssetHolding: &basics.AssetHolding{
					Amount: txn2.Txn.AssetParams.Total,
					Frozen: txn2.Txn.AssetFrozen,
				},
				AssetParams: &txn2.Txn.AssetParams,
			},
		},
		test.AccountB: {
			ledger.Creatable{Index: 4, Type: basics.AssetCreatable}: {
				AssetHolding: &basics.AssetHolding{
					Amount: txn3.Txn.AssetParams.Total,
					Frozen: txn3.Txn.AssetFrozen,
				},
				AssetParams: &txn3.Txn.AssetParams,
			},
		},
	}
	assert.Equal(t, expected, ret)
}

func TestLedgerForEvaluatorApp(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeSimpleAppCallTxn(0, test.AccountA)
	txn1 := test.MakeAppCallTxnWithLogs(0, test.AccountA, []string{"testing"})
	txn2 := test.MakeAppCallWithInnerTxn(test.AccountA, test.AccountA, test.AccountB, basics.Address{}, basics.Address{})
	txn3 := test.MakeAppCallWithMultiLogs(test.AccountA)
	txn4 := test.MakeAppDestroyTxn(1, test.AccountA)
	txn5 := test.MakeSimpleAppCallTxn(0, test.AccountB)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0, &txn1, &txn2, &txn3, &txn4, &txn5)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err :=
		ld.LookupResources(map[basics.Address]map[ledger.Creatable]struct{}{
			test.AccountA: {
				{Index: 1, Type: basics.AppCreatable}: {},
				{Index: 2, Type: basics.AppCreatable}: {},
				{Index: 3, Type: basics.AppCreatable}: {},
				{Index: 4, Type: basics.AppCreatable}: {},
			},
			test.AccountB: {
				{Index: 6, Type: basics.AppCreatable}: {},
			},
		})
	require.NoError(t, err)

	expected := map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource{
		test.AccountA: {
			ledger.Creatable{Index: 1, Type: basics.AppCreatable}: {},
			ledger.Creatable{Index: 2, Type: basics.AppCreatable}: {
				AppParams: &basics.AppParams{
					ApprovalProgram:   txn1.Txn.ApprovalProgram,
					ClearStateProgram: txn1.Txn.ClearStateProgram,
					GlobalState:       nil,
					StateSchemas:      basics.StateSchemas{},
					ExtraProgramPages: txn1.Txn.ExtraProgramPages,
				},
			},
			ledger.Creatable{Index: 3, Type: basics.AppCreatable}: {
				AppParams: &basics.AppParams{
					ApprovalProgram:   txn2.Txn.ApprovalProgram,
					ClearStateProgram: txn2.Txn.ClearStateProgram,
					GlobalState:       nil,
					StateSchemas:      basics.StateSchemas{},
					ExtraProgramPages: txn2.Txn.ExtraProgramPages,
				},
			},
			ledger.Creatable{Index: 4, Type: basics.AppCreatable}: {
				AppParams: &basics.AppParams{
					ApprovalProgram:   txn3.Txn.ApprovalProgram,
					ClearStateProgram: txn3.Txn.ClearStateProgram,
					GlobalState:       nil,
					StateSchemas:      basics.StateSchemas{},
					ExtraProgramPages: txn3.Txn.ExtraProgramPages,
				},
			},
		},
		test.AccountB: {
			ledger.Creatable{Index: 6, Type: basics.AppCreatable}: {
				AppParams: &basics.AppParams{
					ApprovalProgram:   txn5.Txn.ApprovalProgram,
					ClearStateProgram: txn5.Txn.ClearStateProgram,
					GlobalState:       nil,
					StateSchemas:      basics.StateSchemas{},
					ExtraProgramPages: txn5.Txn.ExtraProgramPages,
				},
			},
		},
	}
	assert.Equal(t, expected, ret)
}

func TestLedgerForEvaluatorFetchAllResourceTypes(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeSimpleAppCallTxn(0, test.AccountA)
	txn1 := test.MakeAssetConfigTxn(0, 2, 0, false, "", "", "", test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0, &txn1)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err :=
		ld.LookupResources(map[basics.Address]map[ledger.Creatable]struct{}{
			test.AccountA: {
				{Index: 1, Type: basics.AppCreatable}:   {},
				{Index: 2, Type: basics.AssetCreatable}: {},
			},
		})
	require.NoError(t, err)

	expected := map[basics.Address]map[ledger.Creatable]ledgercore.AccountResource{
		test.AccountA: {
			ledger.Creatable{Index: 1, Type: basics.AppCreatable}: {
				AppParams: &basics.AppParams{
					ApprovalProgram:   txn0.Txn.ApprovalProgram,
					ClearStateProgram: txn0.Txn.ClearStateProgram,
					GlobalState:       nil,
					StateSchemas:      basics.StateSchemas{},
					ExtraProgramPages: txn0.Txn.ExtraProgramPages,
				},
			},
			ledger.Creatable{Index: 2, Type: basics.AssetCreatable}: {
				AssetHolding: &basics.AssetHolding{
					Amount: 2,
					Frozen: false,
				},
				AssetParams: &txn1.Txn.AssetParams,
			},
		},
	}
	assert.Equal(t, expected, ret)
}

func TestLedgerForEvaluatorLookupMultipleAccounts(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	block_processor.MakeBlockProcessorWithLedger(logger, l, nil)

	addresses := []basics.Address{
		test.AccountA, test.AccountB, test.AccountC, test.AccountD}

	addressesMap := make(map[basics.Address]struct{})
	for _, address := range addresses {
		addressesMap[address] = struct{}{}
	}
	addressesMap[test.FeeAddr] = struct{}{}
	addressesMap[test.RewardAddr] = struct{}{}

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err :=
		ld.LookupWithoutRewards(addressesMap)
	require.NoError(t, err)

	for _, address := range addresses {
		accountData := ret[address]
		require.NotNil(t, accountData)
	}
}

func TestLedgerForEvaluatorAssetCreatorBasic(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeAssetConfigTxn(0, 2, 0, false, "", "", "", test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err := ld.GetAssetCreator(
		map[basics.AssetIndex]struct{}{basics.AssetIndex(1): {}})
	require.NoError(t, err)

	foundAddress, ok := ret[basics.AssetIndex(1)]
	require.True(t, ok)

	expected := ledger.FoundAddress{
		Address: test.AccountA,
		Exists:  true,
	}
	assert.Equal(t, expected, foundAddress)
}

func TestLedgerForEvaluatorAssetCreatorDeleted(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeAssetConfigTxn(0, 2, 0, false, "", "", "", test.AccountA)
	txn1 := test.MakeAssetDestroyTxn(1, test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0, &txn1)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err := ld.GetAssetCreator(
		map[basics.AssetIndex]struct{}{basics.AssetIndex(1): {}})
	require.NoError(t, err)

	foundAddress, ok := ret[basics.AssetIndex(1)]
	require.True(t, ok)

	assert.False(t, foundAddress.Exists)
}

func TestLedgerForEvaluatorAssetCreatorMultiple(t *testing.T) {

	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeAssetConfigTxn(0, 2, 0, false, "", "", "", test.AccountA)
	txn1 := test.MakeAssetConfigTxn(0, 2, 0, false, "", "", "", test.AccountB)
	txn2 := test.MakeAssetConfigTxn(0, 2, 0, false, "", "", "", test.AccountC)
	txn3 := test.MakeAssetConfigTxn(0, 2, 0, false, "", "", "", test.AccountD)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0, &txn1, &txn2, &txn3)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	indices := map[basics.AssetIndex]struct{}{
		1: {}, 2: {}, 3: {}, 4: {}, 5: {}, 6: {}, 7: {}, 8: {}}
	ret, err := ld.GetAssetCreator(indices)
	require.NoError(t, err)

	creatorsMap := map[basics.AssetIndex]basics.Address{
		1: test.AccountA,
		2: test.AccountB,
		3: test.AccountC,
		4: test.AccountD,
	}

	for i := 1; i <= 4; i++ {
		index := basics.AssetIndex(i)

		foundAddress, ok := ret[index]
		require.True(t, ok)

		expected := ledger.FoundAddress{
			Address: creatorsMap[index],
			Exists:  true,
		}
		assert.Equal(t, expected, foundAddress)
	}
	for i := 5; i <= 8; i++ {
		index := basics.AssetIndex(i)

		foundAddress, ok := ret[index]
		require.True(t, ok)

		assert.False(t, foundAddress.Exists)
	}
}

func TestLedgerForEvaluatorAppCreatorBasic(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeSimpleAppCallTxn(0, test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	require.NoError(t, err)
	defer ld.Close()

	ret, err := ld.GetAppCreator(
		map[basics.AppIndex]struct{}{basics.AppIndex(1): {}})
	require.NoError(t, err)

	foundAddress, ok := ret[basics.AppIndex(1)]
	require.True(t, ok)

	expected := ledger.FoundAddress{
		Address: test.AccountA,
		Exists:  true,
	}
	assert.Equal(t, expected, foundAddress)

}

func TestLedgerForEvaluatorAppCreatorDeleted(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeSimpleAppCallTxn(0, test.AccountA)
	txn1 := test.MakeAppDestroyTxn(1, test.AccountA)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0, &txn1)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	ret, err := ld.GetAppCreator(
		map[basics.AppIndex]struct{}{basics.AppIndex(1): {}})
	require.NoError(t, err)

	foundAddress, ok := ret[basics.AppIndex(1)]
	require.True(t, ok)

	assert.False(t, foundAddress.Exists)
}

func TestLedgerForEvaluatorAppCreatorMultiple(t *testing.T) {

	l := makeTestLedger(t)
	defer l.Close()
	logger, _ := test2.NewNullLogger()
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)

	txn0 := test.MakeSimpleAppCallTxn(0, test.AccountA)
	txn1 := test.MakeSimpleAppCallTxn(0, test.AccountB)
	txn2 := test.MakeSimpleAppCallTxn(0, test.AccountC)
	txn3 := test.MakeSimpleAppCallTxn(0, test.AccountD)

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &txn0, &txn1, &txn2, &txn3)
	assert.Nil(t, err)
	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	assert.Nil(t, err)

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	creatorsMap := map[basics.AppIndex]basics.Address{
		1: test.AccountA,
		2: test.AccountB,
		3: test.AccountC,
		4: test.AccountD,
	}

	indices := map[basics.AppIndex]struct{}{
		1: {}, 2: {}, 3: {}, 4: {}, 5: {}, 6: {}, 7: {}, 8: {}}
	ret, err := ld.GetAppCreator(indices)
	require.NoError(t, err)

	assert.Equal(t, len(indices), len(ret))
	for i := 1; i <= 4; i++ {
		index := basics.AppIndex(i)

		foundAddress, ok := ret[index]
		require.True(t, ok)

		expected := ledger.FoundAddress{
			Address: creatorsMap[index],
			Exists:  true,
		}
		assert.Equal(t, expected, foundAddress)
	}
	for i := 5; i <= 8; i++ {
		index := basics.AppIndex(i)

		foundAddress, ok := ret[index]
		require.True(t, ok)

		assert.False(t, foundAddress.Exists)
	}
}

// compareAppBoxesAgainstLedger uses LedgerForEvaluator to assert that provided app boxes can be retrieved as expected
func compareAppBoxesAgainstLedger(t *testing.T, ld indxLedger.LedgerForEvaluator, round basics.Round,
	appBoxes map[basics.AppIndex]map[string]string, extras ...map[basics.AppIndex]map[string]bool) {
	require.LessOrEqual(t, len(extras), 1)
	var deletedBoxes map[basics.AppIndex]map[string]bool
	if len(extras) == 1 {
		deletedBoxes = extras[0]
	}

	caseNum := 1
	for appIdx, boxes := range appBoxes {
		for key, expectedValue := range boxes {
			msg := fmt.Sprintf("caseNum=%d, appIdx=%d, key=%#v", caseNum, appIdx, key)
			expectedAppIdx, _, err := apps.SplitBoxKey(key)
			require.NoError(t, err, msg)
			require.Equal(t, uint64(appIdx), expectedAppIdx, msg)

			boxDeleted := false
			if deletedBoxes != nil {
				if _, ok := deletedBoxes[appIdx][key]; ok {
					boxDeleted = true
				}
			}

			value, err := ld.LookupKv(round, key)
			require.NoError(t, err, msg)
			if !boxDeleted {
				require.Equal(t, []byte(expectedValue), value, msg)
			} else {
				require.Nil(t, value, msg)
			}
		}
		caseNum++
	}
}

// Test the functionality of `func (l LedgerForEvaluator) LookupKv()`.
// This is done by handing off a pointer to Struct `processor/eval/ledger_for_evaluator.go::LedgerForEvaluator`
// to `compareAppBoxesAgainstLedger()` which then asserts using `LookupKv()`
func TestLedgerForEvaluatorLookupKv(t *testing.T) {
	logger, _ := test2.NewNullLogger()
	l := makeTestLedger(t)
	pr, _ := block_processor.MakeBlockProcessorWithLedger(logger, l, nil)
	proc := block_processor.MakeBlockProcessorHandlerAdapter(&pr, nil)
	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer l.Close()
	defer ld.Close()

	// ---- ROUND 1: create and fund the box app  ---- //

	appid := basics.AppIndex(1)
	currentRound := basics.Round(1)

	createTxn, err := test.MakeComplexCreateAppTxn(test.AccountA, test.BoxApprovalProgram, test.BoxClearProgram, 8)
	require.NoError(t, err)

	payNewAppTxn := test.MakePaymentTxn(1000, 500000, 0, 0, 0, 0, test.AccountA, appid.Address(), basics.Address{},
		basics.Address{})

	block, err := test.MakeBlockForTxns(test.MakeGenesisBlock().BlockHeader, &createTxn, &payNewAppTxn)
	require.NoError(t, err)

	rawBlock := rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	require.NoError(t, err)

	ret, err := ld.LookupKv(currentRound, "sanity check")
	require.NoError(t, err)
	require.Nil(t, ret) // nothing found isn't considered an error

	// block header handoff: round 1 --> round 2
	blockHdr, err := l.BlockHdr(currentRound)
	require.NoError(t, err)

	// ---- ROUND 2: create 8 boxes of appid == 1  ---- //
	currentRound = basics.Round(2)

	boxNames := []string{
		"a great box",
		"another great box",
		"not so great box",
		"disappointing box",
		"don't box me in this way",
		"I will be assimilated",
		"I'm destined for deletion",
		"box #8",
	}

	expectedAppBoxes := map[basics.AppIndex]map[string]string{}
	expectedAppBoxes[appid] = map[string]string{}
	newBoxValue := "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	boxTxns := make([]*transactions.SignedTxnWithAD, 0)
	for _, boxName := range boxNames {
		expectedAppBoxes[appid][apps.MakeBoxKey(uint64(appid), boxName)] = newBoxValue

		args := []string{"create", boxName}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

	}

	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	require.NoError(t, err)

	compareAppBoxesAgainstLedger(t, ld, currentRound, expectedAppBoxes)

	// block header handoff: round 2 --> round 3
	blockHdr, err = l.BlockHdr(currentRound)
	require.NoError(t, err)

	// ---- ROUND 3: populate the boxes appropriately  ---- //
	currentRound = basics.Round(3)

	appBoxesToSet := map[string]string{
		"a great box":               "it's a wonderful box",
		"another great box":         "I'm wonderful too",
		"not so great box":          "bummer",
		"disappointing box":         "RUG PULL!!!!",
		"don't box me in this way":  "non box-conforming",
		"I will be assimilated":     "THE BORG",
		"I'm destined for deletion": "I'm still alive!!!",
		"box #8":                    "eight is beautiful",
	}

	boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	expectedAppBoxes[appid] = make(map[string]string)
	for boxName, valPrefix := range appBoxesToSet {
		args := []string{"set", boxName, valPrefix}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}
	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	require.NoError(t, err)

	compareAppBoxesAgainstLedger(t, ld, currentRound, expectedAppBoxes)

	// block header handoff: round 3 --> round 4
	blockHdr, err = l.BlockHdr(currentRound)
	require.NoError(t, err)

	// ---- ROUND 4: delete the unhappy boxes  ---- //
	currentRound = basics.Round(4)

	appBoxesToDelete := []string{
		"not so great box",
		"disappointing box",
		"I'm destined for deletion",
	}

	boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	for _, boxName := range appBoxesToDelete {
		args := []string{"delete", boxName}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

		key := apps.MakeBoxKey(uint64(appid), boxName)
		delete(expectedAppBoxes[appid], key)
	}
	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	require.NoError(t, err)

	deletedBoxes := make(map[basics.AppIndex]map[string]bool)
	deletedBoxes[appid] = make(map[string]bool)
	for _, deletedBox := range appBoxesToDelete {
		deletedBoxes[appid][deletedBox] = true
	}
	compareAppBoxesAgainstLedger(t, ld, currentRound, expectedAppBoxes, deletedBoxes)

	// block header handoff: round 4 --> round 5
	blockHdr, err = l.BlockHdr(currentRound)
	require.NoError(t, err)

	// ---- ROUND 5: create 3 new boxes, overwriting one of the former boxes  ---- //
	currentRound = basics.Round(5)

	appBoxesToCreate := []string{
		"fantabulous",
		"disappointing box", // overwriting here
		"AVM is the new EVM",
	}
	boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	for _, boxName := range appBoxesToCreate {
		args := []string{"create", boxName}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid] = make(map[string]string)
		expectedAppBoxes[appid][key] = newBoxValue
	}
	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	require.NoError(t, err)

	compareAppBoxesAgainstLedger(t, ld, currentRound, expectedAppBoxes)

	// block header handoff: round 5 --> round 6
	blockHdr, err = l.BlockHdr(currentRound)
	require.NoError(t, err)

	// ---- ROUND 6: populate the 3 new boxes  ---- //
	currentRound = basics.Round(6)

	appBoxesToSet = map[string]string{
		"fantabulous":        "Italian food's the best!", // max char's
		"disappointing box":  "you made it!",
		"AVM is the new EVM": "yes we can!",
	}
	boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	for boxName, valPrefix := range appBoxesToSet {
		args := []string{"set", boxName, valPrefix}
		boxTxn := test.MakeAppCallTxnWithBoxes(uint64(appid), test.AccountA, args, []string{boxName})
		boxTxns = append(boxTxns, &boxTxn)

		key := apps.MakeBoxKey(uint64(appid), boxName)
		expectedAppBoxes[appid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}
	block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	require.NoError(t, err)

	rawBlock = rpcs.EncodedBlockCert{Block: block, Certificate: agreement.Certificate{}}
	err = proc(&rawBlock)
	require.NoError(t, err)

	compareAppBoxesAgainstLedger(t, ld, currentRound, expectedAppBoxes, deletedBoxes)
}

func TestLedgerForEvaluatorAccountTotals(t *testing.T) {
	l := makeTestLedger(t)
	defer l.Close()

	ld := indxLedger.MakeLedgerForEvaluator(l)
	defer ld.Close()

	accountTotalsRead, err := ld.LatestTotals()
	require.NoError(t, err)

	_, total, _ := l.LatestTotals()
	assert.Equal(t, total, accountTotalsRead)

}
