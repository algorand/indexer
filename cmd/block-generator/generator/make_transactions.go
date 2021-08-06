package generator

import (
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
)

func (g *generator) makeTxnHeader(sender basics.Address) transactions.Header {
	return transactions.Header{
		Sender:      sender,
		Fee:         basics.MicroAlgos{Raw: fee},
		GenesisID:   g.genesisID,
		GenesisHash: g.genesisHash,
	}
}

func (g *generator) makePaymentTxn(sender basics.Address, receiver basics.Address, amount uint64, closeRemainderTo basics.Address) transactions.Transaction {
	return transactions.Transaction{
		Type:   protocol.PaymentTx,
		Header: g.makeTxnHeader(sender),
		PaymentTxnFields: transactions.PaymentTxnFields{
			Receiver:         receiver,
			Amount:           basics.MicroAlgos{Raw: amount},
			CloseRemainderTo: closeRemainderTo,
		},
	}
}

func (g *generator) makeAssetCreateTxn(sender basics.Address, total uint64, defaultFrozen bool, assetName string) transactions.Transaction {
	return transactions.Transaction{
		Type:   protocol.AssetConfigTx,
		Header: g.makeTxnHeader(sender),
		AssetConfigTxnFields: transactions.AssetConfigTxnFields{
			AssetParams: basics.AssetParams{
				Total:         total,
				DefaultFrozen: defaultFrozen,
			},
		},
	}
}

func (g *generator) makeAssetDestroyTxn(sender basics.Address, index uint64) transactions.Transaction {
	return transactions.Transaction{
		Type:   protocol.AssetConfigTx,
		Header: g.makeTxnHeader(sender),
		AssetConfigTxnFields: transactions.AssetConfigTxnFields{
			ConfigAsset: basics.AssetIndex(index),
		},
	}
}

func (g *generator) makeAssetTransferTxn(sender basics.Address, receiver basics.Address, amount uint64, closeAssetsTo basics.Address, index uint64) transactions.Transaction {
	return transactions.Transaction{
		Type:   protocol.AssetTransferTx,
		Header: g.makeTxnHeader(sender),
		AssetTransferTxnFields: transactions.AssetTransferTxnFields{
			XferAsset:     basics.AssetIndex(index),
			AssetAmount:   amount,
			AssetSender:   sender,
			AssetReceiver: receiver,
			AssetCloseTo:  closeAssetsTo,
		},
	}
}

func (g *generator) makeAssetAcceptanceTxn(account basics.Address, index uint64) transactions.Transaction {
	return g.makeAssetTransferTxn(account, account, 0, basics.Address{}, index)
}
