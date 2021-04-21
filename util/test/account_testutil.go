package test

import (
	"fmt"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
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
	OpenMainStxn *atypes.SignedTxnWithAD
	// OpenMain is a premade TxnRow which may be useful in tests.
	OpenMain *idb.TxnRow

	// CloseMainToBCStxn is a premade signed transaction which may be useful in tests.
	CloseMainToBCStxn *atypes.SignedTxnWithAD
	// CloseMainToBC is a premade TxnRow which may be useful in tests.
	CloseMainToBC *idb.TxnRow
)

func init() {
	OpenMainStxn, OpenMain = MakePayTxnRowOrPanic(Round, 1000, 10234, 0, 111, 1111, 0, AccountC,
		AccountA, atypes.ZeroAddress, atypes.ZeroAddress)
	// CloseMainToBCStxn and CloseMainToBC are premade transactions which may be useful in tests.
	CloseMainToBCStxn, CloseMainToBC = MakePayTxnRowOrPanic(Round, 1000, 1234, 9111, 0, 111, 111,
		AccountA, AccountC, AccountB, atypes.ZeroAddress)
}

// DecodeAddressOrPanic is a helper to ensure addresses are initialized.
func DecodeAddressOrPanic(addr string) atypes.Address {
	if addr == "" {
		return atypes.ZeroAddress
	}

	result, err := atypes.DecodeAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decode address: '%s'", addr))
	}
	return result
}

// MakeAssetConfigOrPanic is a helper to ensure test asset config are initialized.
func MakeAssetConfigOrPanic(round, configid, assetid, total, decimals uint64, defaultFrozen bool, unitName, assetName, url string, addr atypes.Address) (*atypes.SignedTxnWithAD, *idb.TxnRow) {
	txn := atypes.SignedTxnWithAD{
		SignedTxn: atypes.SignedTxn{
			Txn: atypes.Transaction{
				Type: "acfg",
				Header: atypes.Header{
					Sender:     addr,
					Fee:        atypes.MicroAlgos(1000),
					FirstValid: atypes.Round(round),
					LastValid:  atypes.Round(round),
				},
				AssetConfigTxnFields: atypes.AssetConfigTxnFields{
					ConfigAsset: atypes.AssetIndex(configid),
					AssetParams: atypes.AssetParams{
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
func MakeAssetFreezeOrPanic(round, assetid uint64, frozen bool, sender, freezeAccount atypes.Address) (*atypes.SignedTxnWithAD, *idb.TxnRow) {
	txn := atypes.SignedTxnWithAD{
		SignedTxn: atypes.SignedTxn{
			Txn: atypes.Transaction{
				Type: "afrz",
				Header: atypes.Header{
					Sender:     sender,
					Fee:        atypes.MicroAlgos(1000),
					FirstValid: atypes.Round(round),
					LastValid:  atypes.Round(round),
				},
				AssetFreezeTxnFields: atypes.AssetFreezeTxnFields{
					FreezeAccount: freezeAccount,
					FreezeAsset:   atypes.AssetIndex(assetid),
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
func MakeAssetTxnOrPanic(round, assetid, amt uint64, sender, receiver, close atypes.Address) (*atypes.SignedTxnWithAD, *idb.TxnRow) {
	txn := atypes.SignedTxnWithAD{
		SignedTxn: atypes.SignedTxn{
			Txn: atypes.Transaction{
				Type: "axfer",
				Header: atypes.Header{
					Sender:     sender,
					Fee:        atypes.MicroAlgos(1000),
					FirstValid: atypes.Round(round),
					LastValid:  atypes.Round(round),
				},
				AssetTransferTxnFields: atypes.AssetTransferTxnFields{
					XferAsset:   atypes.AssetIndex(assetid),
					AssetAmount: amt,
					//only used for clawback transactions
					//AssetSender:   atypes.Address{},
					AssetReceiver: receiver,
					AssetCloseTo:  close,
				},
			},
		},
		ApplyData: atypes.ApplyData{},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: msgpack.Encode(txn),
	}

	return &txn, &txnRow
}

// MakeAssetDestroyTxn makes a transaction that destroys an asset.
func MakeAssetDestroyTxn(round uint64, assetID uint64) (*atypes.SignedTxnWithAD, *idb.TxnRow) {
	txn := atypes.SignedTxnWithAD{
		SignedTxn: atypes.SignedTxn{
			Txn: atypes.Transaction{
				Type: "acfg",
				AssetConfigTxnFields: atypes.AssetConfigTxnFields{
					ConfigAsset: atypes.AssetIndex(assetID),
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
	closeRewards uint64, sender, receiver, close, rekeyTo atypes.Address) (*atypes.SignedTxnWithAD,
	*idb.TxnRow) {
	txn := atypes.SignedTxnWithAD{
		SignedTxn: atypes.SignedTxn{
			Txn: atypes.Transaction{
				Type: "pay",
				Header: atypes.Header{
					Sender:     sender,
					Fee:        atypes.MicroAlgos(fee),
					FirstValid: atypes.Round(round),
					LastValid:  atypes.Round(round),
					RekeyTo:    rekeyTo,
				},
				PaymentTxnFields: atypes.PaymentTxnFields{
					Receiver:         receiver,
					Amount:           atypes.MicroAlgos(amt),
					CloseRemainderTo: close,
				},
			},
		},
		ApplyData: atypes.ApplyData{
			ClosingAmount:   atypes.MicroAlgos(closeAmt),
			SenderRewards:   atypes.MicroAlgos(sendRewards),
			ReceiverRewards: atypes.MicroAlgos(receiveRewards),
			CloseRewards:    atypes.MicroAlgos(closeRewards),
		},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: msgpack.Encode(txn),
	}

	return &txn, &txnRow
}

// MakeBlockForTxns takes some transactions and constructs a block compatible with the indexer import function.
func MakeBlockForTxns(inputs ...*atypes.SignedTxnWithAD) types.EncodedBlockCert {
	var txns []types.SignedTxnInBlock

	for _, txn := range inputs {
		txns = append(txns, types.SignedTxnInBlock{
			SignedTxnWithAD: types.SignedTxnWithAD{SignedTxn: txn.SignedTxn},
			HasGenesisID:    true,
			HasGenesisHash:  true,
		})
	}

	return types.EncodedBlockCert{
		Block: types.Block{
			BlockHeader: types.BlockHeader{
				UpgradeState: types.UpgradeState{CurrentProtocol: "future"},
			},
			Payset: txns,
		},
		Certificate: types.Certificate{},
	}
}
