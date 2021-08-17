package test

import (
	"fmt"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
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
	// AccountE is a premade account for use in tests.
	AccountE = DecodeAddressOrPanic("QYE3RIRIIUS4VRZ4WYR7E5R6WBHTQXUY7F62C7U77SSRAXUSFTSRQPXPPU")
	// FeeAddr is the fee addess to use when creating the state object.
	FeeAddr = DecodeAddressOrPanic("ZROKLZW4GVOK5WQIF2GUR6LHFVEZBMV56BIQEQD4OTIZL2BPSYYUKFBSHM")
	// RewardAddr is the fee addess to use when creating the state object.
	RewardAddr = DecodeAddressOrPanic("4C3S3A5II6AYMEADSW7EVL7JAKVU2ASJMMJAGVUROIJHYMS6B24NCXVEWM")

	// OpenMainStxn is a premade signed transaction which may be useful in tests.
	OpenMainStxn *transactions.SignedTxnWithAD
	// OpenMain is a premade TxnRow which may be useful in tests.
	OpenMain *idb.TxnRow

	// CloseMainToBCStxn is a premade signed transaction which may be useful in tests.
	CloseMainToBCStxn *transactions.SignedTxnWithAD
	// CloseMainToBC is a premade TxnRow which may be useful in tests.
	CloseMainToBC *idb.TxnRow

	// GenesisHash is a genesis hash used in tests.
	GenesisHash = crypto.Digest{77}
	// Signature is a signature for transactions used in tests.
	Signature = crypto.Signature{88}

	// Proto is a fake protocol version.
	Proto = protocol.ConsensusFuture
)

func init() {
	OpenMainStxn, OpenMain = MakePaymentTxn(Round, 1000, 10234, 0, 111, 1111, 0, AccountC,
		AccountA, basics.Address{}, basics.Address{})
	// CloseMainToBCStxn and CloseMainToBC are premade transactions which may be useful in tests.
	CloseMainToBCStxn, CloseMainToBC = MakePaymentTxn(Round, 1000, 1234, 9111, 0, 111, 111,
		AccountA, AccountC, AccountB, basics.Address{})
}

// DecodeAddressOrPanic is a helper to ensure addresses are initialized.
func DecodeAddressOrPanic(addr string) basics.Address {
	if addr == "" {
		return basics.Address{}
	}

	result, err := basics.UnmarshalChecksumAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decode address: '%s'", addr))
	}
	return result
}

// MakeCreateAssetTxn is a helper to ensure test asset config are initialized.
func MakeCreateAssetTxn(round, configid, assetid, total, decimals uint64, defaultFrozen bool, unitName, assetName, url string, addr basics.Address) (*transactions.SignedTxnWithAD, *idb.TxnRow) {
	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "acfg",
				Header: transactions.Header{
					Sender:      addr,
					Fee:         basics.MicroAlgos{Raw: 1000},
					FirstValid:  basics.Round(round),
					LastValid:   basics.Round(round),
					GenesisHash: GenesisHash,
				},
				AssetConfigTxnFields: transactions.AssetConfigTxnFields{
					ConfigAsset: basics.AssetIndex(configid),
					AssetParams: basics.AssetParams{
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
		TxnBytes: protocol.Encode(&txn),
		AssetID:  assetid,
	}

	return &txn, &txnRow
}

// MakeAssetFreezeTxn create an asset freeze/unfreeze transaction.
func MakeAssetFreezeTxn(round, assetid uint64, frozen bool, sender, freezeAccount basics.Address) (*transactions.SignedTxnWithAD, *idb.TxnRow) {
	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "afrz",
				Header: transactions.Header{
					Sender:      sender,
					Fee:         basics.MicroAlgos{Raw: 1000},
					FirstValid:  basics.Round(round),
					LastValid:   basics.Round(round),
					GenesisHash: GenesisHash,
				},
				AssetFreezeTxnFields: transactions.AssetFreezeTxnFields{
					FreezeAccount: freezeAccount,
					FreezeAsset:   basics.AssetIndex(assetid),
					AssetFrozen:   frozen,
				},
			},
		},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: protocol.Encode(&txn),
		AssetID:  assetid,
	}

	return &txn, &txnRow
}

// MakeAssetTransferTxn creates an asset transfer transaction.
func MakeAssetTransferTxn(round, assetid, amt uint64, sender, receiver, close basics.Address) (*transactions.SignedTxnWithAD, *idb.TxnRow) {
	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "axfer",
				Header: transactions.Header{
					Sender:      sender,
					Fee:         basics.MicroAlgos{Raw: 1000},
					FirstValid:  basics.Round(round),
					LastValid:   basics.Round(round),
					GenesisHash: GenesisHash,
				},
				AssetTransferTxnFields: transactions.AssetTransferTxnFields{
					XferAsset:   basics.AssetIndex(assetid),
					AssetAmount: amt,
					//only used for clawback transactions
					//AssetSender:   basics.Address{},
					AssetReceiver: receiver,
					AssetCloseTo:  close,
				},
			},
		},
		ApplyData: transactions.ApplyData{},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: protocol.Encode(&txn),
	}

	return &txn, &txnRow
}

// MakeAssetDestroyTxn makes a transaction that destroys an asset.
func MakeAssetDestroyTxn(round uint64, assetID uint64) (*transactions.SignedTxnWithAD, *idb.TxnRow) {
	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "acfg",
				Header: transactions.Header{
					GenesisHash: GenesisHash,
				},
				AssetConfigTxnFields: transactions.AssetConfigTxnFields{
					ConfigAsset: basics.AssetIndex(assetID),
				},
			},
		},
	}

	txnRow := idb.TxnRow{
		Round:    round,
		TxnBytes: protocol.Encode(&txn),
		AssetID:  assetID,
	}

	return &txn, &txnRow
}

// MakePaymentTxn creates an algo transfer transaction.
func MakePaymentTxn(round, fee, amt, closeAmt, sendRewards, receiveRewards,
	closeRewards uint64, sender, receiver, close, rekeyTo basics.Address) (*transactions.SignedTxnWithAD, *idb.TxnRow) {
	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "pay",
				Header: transactions.Header{
					Sender:      sender,
					Fee:         basics.MicroAlgos{Raw: 1000},
					FirstValid:  basics.Round(round),
					LastValid:   basics.Round(round),
					RekeyTo:     rekeyTo,
					GenesisHash: GenesisHash,
				},
				PaymentTxnFields: transactions.PaymentTxnFields{
					Receiver:         receiver,
					Amount:           basics.MicroAlgos{Raw: amt},
					CloseRemainderTo: close,
				},
			},
		},
		ApplyData: transactions.ApplyData{
			ClosingAmount:   basics.MicroAlgos{Raw: closeAmt},
			SenderRewards:   basics.MicroAlgos{Raw: sendRewards},
			ReceiverRewards: basics.MicroAlgos{Raw: receiveRewards},
			CloseRewards:    basics.MicroAlgos{Raw: closeRewards},
		},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: protocol.Encode(&txn),
	}

	return &txn, &txnRow
}

// MakeSimpleKeyregOnlineTxn creates a fake key registration transaction.
func MakeSimpleKeyregOnlineTxn(round uint64, sender basics.Address) (*transactions.SignedTxnWithAD, *idb.TxnRow) {
	var votePK crypto.OneTimeSignatureVerifier
	votePK[0] = 1

	var selectionPK crypto.VRFVerifier
	selectionPK[0] = 2

	txn := transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "keyreg",
				Header: transactions.Header{
					Sender:      sender,
					FirstValid:  basics.Round(round),
					LastValid:   basics.Round(round),
					GenesisHash: GenesisHash,
				},
				KeyregTxnFields: transactions.KeyregTxnFields{
					VotePK:          votePK,
					SelectionPK:     selectionPK,
					VoteFirst:       basics.Round(round),
					VoteLast:        basics.Round(round),
					VoteKeyDilution: 1,
				},
			},
		},
	}

	txnRow := idb.TxnRow{
		Round:    uint64(txn.Txn.FirstValid),
		TxnBytes: protocol.Encode(&txn),
	}

	return &txn, &txnRow
}

// MakeBlockForTxns takes some transactions and constructs a block compatible with the indexer import function.
func MakeBlockForTxns(round uint64, inputs ...*transactions.SignedTxnWithAD) (rpcs.EncodedBlockCert, error) {
	res := rpcs.EncodedBlockCert{
		Block: bookkeeping.Block{
			BlockHeader: bookkeeping.BlockHeader{
				Round:       basics.Round(round),
				GenesisID:   MakeGenesis().ID(),
				GenesisHash: GenesisHash,
				RewardsState: bookkeeping.RewardsState{
					FeeSink:     FeeAddr,
					RewardsPool: RewardAddr,
				},
				UpgradeState: bookkeeping.UpgradeState{CurrentProtocol: Proto},
			},
		},
	}

	for _, stxnad := range inputs {
		stxnib, err := res.Block.EncodeSignedTxn(stxnad.SignedTxn, stxnad.ApplyData)
		if err != nil {
			return rpcs.EncodedBlockCert{}, err
		}
		res.Block.Payset = append(res.Block.Payset, stxnib)
	}

	return res, nil
}

// MakeGenesis creates a sample genesis info.
func MakeGenesis() bookkeeping.Genesis {
	return bookkeeping.Genesis{
		SchemaID: "main",
		Network:  "mynet",
		Proto:    Proto,
		Allocation: []bookkeeping.GenesisAllocation{
			{
				Address: AccountA.String(),
				State: basics.AccountData{
					MicroAlgos: basics.MicroAlgos{Raw: 1000 * 1000 * 1000 * 1000},
				},
			},
			{
				Address: AccountB.String(),
				State: basics.AccountData{
					MicroAlgos: basics.MicroAlgos{Raw: 1000 * 1000 * 1000 * 1000},
				},
			},
			{
				Address: AccountC.String(),
				State: basics.AccountData{
					MicroAlgos: basics.MicroAlgos{Raw: 1000 * 1000 * 1000 * 1000},
				},
			},
			{
				Address: AccountD.String(),
				State: basics.AccountData{
					MicroAlgos: basics.MicroAlgos{Raw: 1000 * 1000 * 1000 * 1000},
				},
			},
		},
		RewardsPool: RewardAddr.String(),
		FeeSink:     FeeAddr.String(),
	}
}
