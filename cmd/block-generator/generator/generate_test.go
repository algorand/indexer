package generator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/types"
)

func makePrivateGenerator(t *testing.T) *generator {
	publicGenerator, err := MakeGenerator(GenerationConfig{
		NumGenesisAccounts: 10,
		GenesisAccountInitialBalance: 10000000000000000000,
		PaymentTransactionFraction:   1.0,
		PaymentNewAccountFraction:    1.0,
		AssetCreateFraction:          1.0,
	})
	require.NoError(t, err)
	return publicGenerator.(*generator)
}

func TestAssetTypeOverrides(t *testing.T) {
	g := makePrivateGenerator(t)
	sp := g.getSuggestedParams(0)

	// First asset transaction must create.
	txn := g.generateAssetTxnInternal(assetXfer, sp, 0)
	require.Equal(t, types.AssetConfigTx, txn.Type)
	require.Len(t, g.assets, 1)
	require.Len(t, g.assets[0].holdings, 1)
	require.Len(t, g.assets[0].holders, 1)

	// Transfer refuses if there is only 1 holder.
	txn = g.generateAssetTxnInternal(assetXfer, sp, 0)
	require.Equal(t, types.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	// A new holding is created, indicating the optin
	require.Len(t, g.assets[0].holdings, 2)
	require.Len(t, g.assets[0].holders, 2)

	// Close the new account to see close override to an optin
	txn = g.generateAssetTxnInternal(assetClose, sp, 0)
	require.Equal(t, types.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	// The holding was removed
	require.Len(t, g.assets[0].holdings, 1)
	require.Len(t, g.assets[0].holders, 1)

	// Instead of closing the creator, optin a new account
	txn = g.generateAssetTxnInternal(assetClose, sp, 0)
	require.Equal(t, types.AssetTransferTx, txn.Type)
	require.Len(t, g.assets, 1)
	// A new holding is created, indicating the optin
	require.Len(t, g.assets[0].holdings, 2)
	require.Len(t, g.assets[0].holders, 2)
}
