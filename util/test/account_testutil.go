package test

import (
	"fmt"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/idb"
)

// Round is the round used in pre-made transactions.
const Round = uint64(10)

var (
	// AccountA is a premade account for use in tests.
	AccountA = DecodeAddressOrPanic("GJR76Q6OXNZ2CYIVCFCDTJRBAAR6TYEJJENEII3G2U3JH546SPBQA62IFY")
	// AccountB is a premade account for use in tests.
	AccountB = DecodeAddressOrPanic("N5T74SANUWLHI6ZWYFQBEB6J2VXBTYUYZNWQB2V26DCF4ARKC7GDUW3IRU")
	// AccountC is a premade account for use in tests.
	AccountC = DecodeAddressOrPanic("OKUWMFFEKF4B4D7FRQYBVV3C2SNS54ZO4WZ2MJ3576UYKFDHM5P3AFMRWE")
	// AccountD is a premade account for use in tests.
	AccountD = DecodeAddressOrPanic("6TB2ZQA2GEEDH6XTIOH5A7FUSGINXDPW5ONN6XBOBBGGUXVHRQTITAIIVI")
	// FeeAddr is the fee addess to use when creating the state object.
	FeeAddr = DecodeAddressOrPanic("ZROKLZW4GVOK5WQIF2GUR6LHFVEZBMV56BIQEQD4OTIZL2BPSYYUKFBSHM")
	// RewardAddr is the fee addess to use when creating the state object.
	RewardAddr = DecodeAddressOrPanic("4C3S3A5II6AYMEADSW7EVL7JAKVU2ASJMMJAGVUROIJHYMS6B24NCXVEWM")

	// OpenMainStxn is a premade signed transaction which may be useful in tests.
	OpenMainStxn *types.SignedTxnWithAD
	// OpenMain is a premade TxnRow which may be useful in tests.
	OpenMain *idb.TxnRow

	// CloseMainToBCStxn is a premade signed transaction which may be useful in tests.
	CloseMainToBCStxn *types.SignedTxnWithAD
	// CloseMainToBC is a premade TxnRow which may be useful in tests.
	CloseMainToBC *idb.TxnRow
)

func init() {
	OpenMainStxn, OpenMain = MakePayTxnRowOrPanic(Round, 1000, 10234, 0, 111, 1111, 0, AccountC,
		AccountA, types.ZeroAddress, types.ZeroAddress)
	// CloseMainToBCStxn and CloseMainToBC are premade transactions which may be useful in tests.
	CloseMainToBCStxn, CloseMainToBC = MakePayTxnRowOrPanic(Round, 1000, 1234, 9111, 0, 111, 111,
		AccountA, AccountC, AccountB, types.ZeroAddress)
}

// DecodeAddressOrPanic is a helper to ensure addresses are initialized.
func DecodeAddressOrPanic(addr string) types.Address {
	if addr == "" {
		return types.ZeroAddress
	}

	result, err := types.DecodeAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decode address: '%s'", addr))
	}
	return result
}

// MakeAssetConfigOrPanic is a helper to ensure test asset config are initialized.
func MakeAssetConfigOrPanic(round, configid, assetid, total, decimals uint64, defaultFrozen bool, unitName, assetName, url string, addr types.Address) (*types.SignedTxnWithAD, *idb.TxnRow) {
	txn := types.SignedTxnWithAD{
		SignedTxn: types.SignedTxn{
			Txn: types.Transaction{
				Type: "acfg",
				Header: types.Header{
					Sender:     addr,
					Fee:        types.MicroAlgos(1000),
					FirstValid: types.Round(round),
					LastValid:  types.Round(round),
				},
				AssetConfigTxnFields: types.AssetConfigTxnFields{
					ConfigAsset: types.AssetIndex(configid),
					AssetParams: types.AssetParams{
						Total:         total,
						Decimals:      uint32(decimals),
						DefaultFrozen: defaultFrozen,
						UnitName:      unitName,
						AssetName:     assetName,
						URL:           url,
						MetadataHash:  [32]byte{},
						Manager:       addr,
						Reserve:       addr,
						Freeze:        addr,
						Clawback:      addr,
					},
				},
			},
		},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: msgpack.Encode(txn),
		AssetID:  assetid,
	}

	return &txn, &txnRow
}

// MakeAssetFreezeOrPanic create an asset freeze/unfreeze transaction.
func MakeAssetFreezeOrPanic(round, assetid uint64, frozen bool, addr types.Address) (*types.SignedTxnWithAD, *idb.TxnRow) {
	txn := types.SignedTxnWithAD{
		SignedTxn: types.SignedTxn{
			Txn: types.Transaction{
				Type: "afrz",
				Header: types.Header{
					Sender:     addr,
					Fee:        types.MicroAlgos(1000),
					FirstValid: types.Round(round),
					LastValid:  types.Round(round),
				},
				AssetFreezeTxnFields: types.AssetFreezeTxnFields{
					FreezeAccount: addr,
					FreezeAsset:   types.AssetIndex(assetid),
					AssetFrozen:   frozen,
				},
			},
		},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: msgpack.Encode(txn),
		AssetID:  assetid,
	}

	return &txn, &txnRow
}

// MakeAssetTxnOrPanic creates an asset transfer transaction.
func MakeAssetTxnOrPanic(round, assetid, amt uint64, sender, receiver, close types.Address) (*types.SignedTxnWithAD, *idb.TxnRow) {
	txn := types.SignedTxnWithAD{
		SignedTxn: types.SignedTxn{
			Txn: types.Transaction{
				Type: "axfer",
				Header: types.Header{
					Sender:     sender,
					Fee:        types.MicroAlgos(1000),
					FirstValid: types.Round(round),
					LastValid:  types.Round(round),
				},
				AssetTransferTxnFields: types.AssetTransferTxnFields{
					XferAsset:   types.AssetIndex(assetid),
					AssetAmount: amt,
					//only used for clawback transactions
					//AssetSender:   types.Address{},
					AssetReceiver: receiver,
					AssetCloseTo:  close,
				},
			},
		},
		ApplyData: types.ApplyData{},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: msgpack.Encode(txn),
	}

	return &txn, &txnRow
}

// MakeAssetDestroyTxn makes a transaction that destroys an asset.
func MakeAssetDestroyTxn(round uint64, assetID uint64) (*types.SignedTxnWithAD, *idb.TxnRow) {
	txn := types.SignedTxnWithAD{
		SignedTxn: types.SignedTxn{
			Txn: types.Transaction{
				Type: "acfg",
				AssetConfigTxnFields: types.AssetConfigTxnFields{
					ConfigAsset: types.AssetIndex(assetID),
				},
			},
		},
	}

	txnRow := idb.TxnRow{
		Round:    round,
		TxnBytes: msgpack.Encode(txn),
		AssetID:  assetID,
	}

	return &txn, &txnRow
}

// MakePayTxnRowOrPanic creates an algo transfer transaction.
func MakePayTxnRowOrPanic(round, fee, amt, closeAmt, sendRewards, receiveRewards,
	closeRewards uint64, sender, receiver, close, rekeyTo types.Address) (*types.SignedTxnWithAD,
	*idb.TxnRow) {
	txn := types.SignedTxnWithAD{
		SignedTxn: types.SignedTxn{
			Txn: types.Transaction{
				Type: "pay",
				Header: types.Header{
					Sender:     sender,
					Fee:        types.MicroAlgos(fee),
					FirstValid: types.Round(round),
					LastValid:  types.Round(round),
					RekeyTo:    rekeyTo,
				},
				PaymentTxnFields: types.PaymentTxnFields{
					Receiver:         receiver,
					Amount:           types.MicroAlgos(amt),
					CloseRemainderTo: close,
				},
			},
		},
		ApplyData: types.ApplyData{
			ClosingAmount:   types.MicroAlgos(closeAmt),
			SenderRewards:   types.MicroAlgos(sendRewards),
			ReceiverRewards: types.MicroAlgos(receiveRewards),
			CloseRewards:    types.MicroAlgos(closeRewards),
		},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: msgpack.Encode(txn),
	}

	return &txn, &txnRow
}
