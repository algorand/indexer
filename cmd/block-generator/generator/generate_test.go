package generator

import (
	"bytes"
	"testing"

	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/stretchr/testify/require"
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
	g.generatePaymentTxnInternal(paymentAcctCreateTx, 0, 0)
	require.Len(t, g.balances, int(g.config.NumGenesisAccounts+1))
}

func TestPaymentTransfer(t *testing.T) {
	g := makePrivateGenerator(t)
	g.generatePaymentTxnInternal(paymentTx, 0, 0)
	require.Len(t, g.balances, int(g.config.NumGenesisAccounts))
}

func TestAssetXferNoAssetsOverride(t *testing.T) {
	g := makePrivateGenerator(t)

	// First asset transaction must create.
	actual, txn := g.generateAssetTxnInternal(assetXfer, 0)
	require.Equal(t, assetCreate, actual)
	require.Equal(t, protocol.AssetConfigTx, txn.Type)
	require.Len(t, g.assets, 1)
	require.Len(t, g.assets[0].holdings, 1)
	require.Len(t, g.assets[0].holders, 1)
}

func TestAssetXferOneHolderOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	g.generateAssetTxnInternal(assetCreate, 0)

	// Transfer converted to optin if there is only 1 holder.
	actual, txn := g.generateAssetTxnInternal(assetXfer, 0)
	require.Equal(t, assetOptin, actual)
	require.Equal(t, protocol.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	// A new holding is created, indicating the optin
	require.Len(t, g.assets[0].holdings, 2)
	require.Len(t, g.assets[0].holders, 2)
}

func TestAssetCloseCreatorOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	g.generateAssetTxnInternal(assetCreate, 0)

	// Instead of closing the creator, optin a new account
	actual, txn := g.generateAssetTxnInternal(assetClose, 0)
	require.Equal(t, assetOptin, actual)
	require.Equal(t, protocol.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	// A new holding is created, indicating the optin
	require.Len(t, g.assets[0].holdings, 2)
	require.Len(t, g.assets[0].holders, 2)
}

func TestAssetOptinEveryAccountOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	g.generateAssetTxnInternal(assetCreate, 0)

	// Opt all the accounts in, this also verifies that no account is opted in twice
	var txn transactions.Transaction
	var actual TxTypeID
	for i := 2; uint64(i) <= g.numAccounts; i++ {
		actual, txn = g.generateAssetTxnInternal(assetOptin, 0)
		require.Equal(t, assetOptin, actual)
		require.Equal(t, protocol.AssetTransferTx, txn.Type)
		require.Len(t, g.assets, 1)
		require.Len(t, g.assets[0].holdings, i)
		require.Len(t, g.assets[0].holders, i)
	}

	// All accounts have opted in
	require.Equal(t, g.numAccounts, uint64(len(g.assets[0].holdings)))

	// The next optin closes instead
	actual, txn = g.generateAssetTxnInternal(assetOptin, 0)
	require.Equal(t, assetClose, actual)
	require.Equal(t, protocol.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	require.Len(t, g.assets[0].holdings, int(g.numAccounts-1))
	require.Len(t, g.assets[0].holders, int(g.numAccounts-1))
}

func TestAssetDestroyWithHoldingsOverride(t *testing.T) {
	g := makePrivateGenerator(t)
	g.generateAssetTxnInternal(assetCreate, 0)
	g.generateAssetTxnInternal(assetOptin, 0)
	g.generateAssetTxnInternal(assetXfer, 0)
	require.Len(t, g.assets[0].holdings, 2)
	require.Len(t, g.assets[0].holders, 2)

	actual, txn := g.generateAssetTxnInternal(assetDestroy, 0)
	require.Equal(t, assetClose, actual)
	require.Equal(t, protocol.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	require.Len(t, g.assets[0].holdings, 1)
	require.Len(t, g.assets[0].holders, 1)
}

func TestAssetTransfer(t *testing.T) {
	g := makePrivateGenerator(t)

	g.generateAssetTxnInternal(assetCreate, 0)
	g.generateAssetTxnInternal(assetOptin, 0)
	g.generateAssetTxnInternal(assetXfer, 0)
	require.Greater(t, g.assets[0].holdings[1].balance, uint64(0))
}

func TestAssetDestroy(t *testing.T) {
	g := makePrivateGenerator(t)
	g.generateAssetTxnInternal(assetCreate, 0)
	require.Len(t, g.assets, 1)

	actual, txn := g.generateAssetTxnInternal(assetDestroy, 0)
	require.Equal(t, assetDestroy, actual)
	require.Equal(t, protocol.AssetConfigTx, txn.Type)
	require.Len(t, g.assets, 0)
}

func TestWriteRoundZero(t *testing.T) {
	g := makePrivateGenerator(t)
	var data []byte
	writer := bytes.NewBuffer(data)
	g.WriteBlock(writer, 0)
	var block rpcs.EncodedBlockCert
	protocol.Decode(data, &block)
	require.Len(t, block.Block.Payset, 0)
}

func TestWriteRound(t *testing.T) {
	g := makePrivateGenerator(t)
	var data []byte
	writer := bytes.NewBuffer(data)
	g.WriteBlock(writer, 1)
	var block rpcs.EncodedBlockCert
	protocol.Decode(data, &block)
	require.Len(t, block.Block.Payset, int(g.config.TxnPerBlock))
}
