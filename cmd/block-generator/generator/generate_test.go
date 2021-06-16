package generator

import (
	"bytes"
	"testing"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/types"
)

func makePrivateGenerator(t *testing.T) *generator {
	publicGenerator, err := MakeGenerator(GenerationConfig{
		NumGenesisAccounts:           10,
		GenesisAccountInitialBalance: 10000000000000000000,
		PaymentTransactionFraction:   1.0,
		PaymentNewAccountFraction:    1.0,
		AssetCreateFraction:          1.0,
	})
	require.NoError(t, err)
	return publicGenerator.(*generator)
}

func TestPaymentAcctCreate(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)
	g.generatePaymentTxnInternal(paymentAcctCreateTx, sp, 0, 0)
	require.Len(t, g.balances, int(g.config.NumGenesisAccounts+1))
}

func TestPaymentTransfer(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)
	g.generatePaymentTxnInternal(paymentTx, sp, 0, 0)
	require.Len(t, g.balances, int(g.config.NumGenesisAccounts))
}

func TestAssetXferNoAssetsOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)

	// First asset transaction must create.
	actual, txn := g.generateAssetTxnInternal(assetXfer, sp, 0)
	require.Equal(t, assetCreate, actual)
	require.Equal(t, sdk_types.AssetConfigTx, txn.Type)
	require.Len(t, g.assets, 1)
	require.Len(t, g.assets[0].holdings, 1)
	require.Len(t, g.assets[0].holders, 1)
}

func TestAssetXferOneHolderOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)
	g.generateAssetTxnInternal(assetCreate, sp, 0)

	// Transfer converted to optin if there is only 1 holder.
	actual, txn := g.generateAssetTxnInternal(assetXfer, sp, 0)
	require.Equal(t, assetOptin, actual)
	require.Equal(t, sdk_types.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	// A new holding is created, indicating the optin
	require.Len(t, g.assets[0].holdings, 2)
	require.Len(t, g.assets[0].holders, 2)
}

func TestAssetCloseCreatorOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)
	g.generateAssetTxnInternal(assetCreate, sp, 0)

	// Instead of closing the creator, optin a new account
	actual, txn := g.generateAssetTxnInternal(assetClose, sp, 0)
	require.Equal(t, assetOptin, actual)
	require.Equal(t, sdk_types.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	// A new holding is created, indicating the optin
	require.Len(t, g.assets[0].holdings, 2)
	require.Len(t, g.assets[0].holders, 2)
}

func TestAssetOptinEveryAccountOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)
	g.generateAssetTxnInternal(assetCreate, sp, 0)

	// Opt all the accounts in, this also verifies that no account is opted in twice
	var txn sdk_types.Transaction
	var actual TxTypeID
	for i := 2; uint64(i) <= g.numAccounts; i++ {
		actual, txn = g.generateAssetTxnInternal(assetOptin, sp, 0)
		require.Equal(t, assetOptin, actual)
		require.Equal(t, sdk_types.AssetTransferTx, txn.Type)
		require.Len(t, g.assets, 1)
		require.Len(t, g.assets[0].holdings, i)
		require.Len(t, g.assets[0].holders, i)
	}

	// All accounts have opted in
	require.Equal(t, g.numAccounts, uint64(len(g.assets[0].holdings)))

	// The next optin closes instead
	actual, txn = g.generateAssetTxnInternal(assetOptin, sp, 0)
	require.Equal(t, assetClose, actual)
	require.Equal(t, sdk_types.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	require.Len(t, g.assets[0].holdings, int(g.numAccounts-1))
	require.Len(t, g.assets[0].holders, int(g.numAccounts-1))
}

func TestAssetDestroyWithHoldingsOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)
	g.generateAssetTxnInternal(assetCreate, sp, 0)
	g.generateAssetTxnInternal(assetOptin, sp, 0)
	g.generateAssetTxnInternal(assetXfer, sp, 0)
	require.Len(t, g.assets[0].holdings, 2)
	require.Len(t, g.assets[0].holders, 2)

	actual, txn := g.generateAssetTxnInternal(assetDestroy, sp, 0)
	require.Equal(t, assetClose, actual)
	require.Equal(t, sdk_types.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	require.Len(t, g.assets[0].holdings, 1)
	require.Len(t, g.assets[0].holders, 1)
}

func TestAssetTransfer(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)

	g.generateAssetTxnInternal(assetCreate, sp, 0)
	g.generateAssetTxnInternal(assetOptin, sp, 0)
	g.generateAssetTxnInternal(assetXfer, sp, 0)
	require.Greater(t, g.assets[0].holdings[1].balance, uint64(0))
}

func TestAssetDestroy(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(1)
	g.generateAssetTxnInternal(assetCreate, sp, 0)
	require.Len(t, g.assets, 1)

	actual, txn := g.generateAssetTxnInternal(assetDestroy, sp, 0)
	require.Equal(t, assetDestroy, actual)
	require.Equal(t, sdk_types.AssetConfigTx, txn.Type)
	require.Len(t, g.assets, 0)
}

func TestWriteRoundZero(t *testing.T) {
	g := makePrivateGenerator(t)
	var data []byte
	writer := bytes.NewBuffer(data)
	g.WriteBlock(writer, 0)
	var block types.EncodedBlockCert
	msgpack.Decode(data, &block)
	require.Len(t, block.Block.Payset, 0)
}

func TestWriteRound(t *testing.T) {
	g := makePrivateGenerator(t)
	var data []byte
	writer := bytes.NewBuffer(data)
	g.WriteBlock(writer, 1)
	var block types.EncodedBlockCert
	msgpack.Decode(data, &block)
	require.Len(t, block.Block.Payset, int(g.config.TxnPerBlock))
}
