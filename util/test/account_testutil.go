package test

import (
	"fmt"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
)

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

	// GenesisHash is a genesis hash used in tests.
	GenesisHash = crypto.Digest{77}
	// Signature is a signature for transactions used in tests.
	Signature = crypto.Signature{88}

	// Proto is a fake protocol version.
	Proto = protocol.ConsensusFuture
)

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

// MakeAssetConfigTxn is a helper to ensure test asset config are initialized.
func MakeAssetConfigTxn(configid, total, decimals uint64, defaultFrozen bool, unitName, assetName, url string, addr basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "acfg",
				Header: transactions.Header{
					Sender:      addr,
					Fee:         basics.MicroAlgos{Raw: 1000},
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
			Sig: Signature,
		},
	}
}

// MakeAssetFreezeTxn create an asset freeze/unfreeze transaction.
func MakeAssetFreezeTxn(assetid uint64, frozen bool, sender, freezeAccount basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "afrz",
				Header: transactions.Header{
					Sender:      sender,
					Fee:         basics.MicroAlgos{Raw: 1000},
					GenesisHash: GenesisHash,
				},
				AssetFreezeTxnFields: transactions.AssetFreezeTxnFields{
					FreezeAccount: freezeAccount,
					FreezeAsset:   basics.AssetIndex(assetid),
					AssetFrozen:   frozen,
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAssetTransferTxn creates an asset transfer transaction.
func MakeAssetTransferTxn(assetid, amt uint64, sender, receiver, close basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "axfer",
				Header: transactions.Header{
					Sender:      sender,
					Fee:         basics.MicroAlgos{Raw: 1000},
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
			Sig: Signature,
		},
	}
}

// MakeAssetOptInTxn makes a transaction that opts in an asset.
func MakeAssetOptInTxn(assetid uint64, address basics.Address) transactions.SignedTxnWithAD {
	return MakeAssetTransferTxn(assetid, 0, address, address, basics.Address{})
}

// MakeAssetDestroyTxn makes a transaction that destroys an asset.
func MakeAssetDestroyTxn(assetID uint64, sender basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "acfg",
				Header: transactions.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
				},
				AssetConfigTxnFields: transactions.AssetConfigTxnFields{
					ConfigAsset: basics.AssetIndex(assetID),
				},
			},
			Sig: Signature,
		},
	}
}

// MakePaymentTxn creates an algo transfer transaction.
func MakePaymentTxn(fee, amt, closeAmt, sendRewards, receiveRewards,
	closeRewards uint64, sender, receiver, close, rekeyTo basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "pay",
				Header: transactions.Header{
					Sender:      sender,
					Fee:         basics.MicroAlgos{Raw: fee},
					GenesisHash: GenesisHash,
					RekeyTo:     rekeyTo,
				},
				PaymentTxnFields: transactions.PaymentTxnFields{
					Receiver:         receiver,
					Amount:           basics.MicroAlgos{Raw: amt},
					CloseRemainderTo: close,
				},
			},
			Sig: Signature,
		},
		ApplyData: transactions.ApplyData{
			ClosingAmount:   basics.MicroAlgos{Raw: closeAmt},
			SenderRewards:   basics.MicroAlgos{Raw: sendRewards},
			ReceiverRewards: basics.MicroAlgos{Raw: receiveRewards},
			CloseRewards:    basics.MicroAlgos{Raw: closeRewards},
		},
	}
}

// MakeSimpleKeyregOnlineTxn creates a fake key registration transaction.
func MakeSimpleKeyregOnlineTxn(sender basics.Address) transactions.SignedTxnWithAD {
	var votePK crypto.OneTimeSignatureVerifier
	votePK[0] = 1

	var selectionPK crypto.VRFVerifier
	selectionPK[0] = 2

	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "keyreg",
				Header: transactions.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
				},
				KeyregTxnFields: transactions.KeyregTxnFields{
					VotePK:          votePK,
					SelectionPK:     selectionPK,
					VoteKeyDilution: 1,
				},
			},
			Sig: Signature,
		},
	}
}

// MakeCreateAppTxn makes a transaction that creates a simple application.
func MakeCreateAppTxn(sender basics.Address) transactions.SignedTxnWithAD {
	// Create a transaction with ExtraProgramPages field set to 1
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAppDestroyTxn makes a transaction that destroys an app.
func MakeAppDestroyTxn(appid uint64, sender basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApplicationID: basics.AppIndex(appid),
					OnCompletion:  transactions.DeleteApplicationOC,
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAppOptInTxn makes a transaction that opts in an app.
func MakeAppOptInTxn(appid uint64, sender basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApplicationID: basics.AppIndex(appid),
					OnCompletion:  transactions.OptInOC,
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAppOptOutTxn makes a transaction that opts out an app.
func MakeAppOptOutTxn(appid uint64, sender basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApplicationID: basics.AppIndex(appid),
					OnCompletion:  transactions.CloseOutOC,
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAppCallWithInnerTxn creates an app call with 3 levels of transactions:
// application create
// |- payment
// |- application call
//    |- asset transfer
//    |- application call
func MakeAppCallWithInnerTxn(appSender, paymentSender, paymentReceiver, assetSender, assetReceiver basics.Address) transactions.SignedTxnWithAD {
	createApp := MakeCreateAppTxn(appSender)

	// In order to simplify the test,
	// since db.AddBlock uses ApplyData from the block and not from the evaluator,
	// fake ApplyData to have inner txn
	// otherwise it requires funding the app account and other special setup
	createApp.ApplyData.EvalDelta.InnerTxns = []transactions.SignedTxnWithAD{
		{
			SignedTxn: transactions.SignedTxn{
				Txn: transactions.Transaction{
					Type: protocol.PaymentTx,
					Header: transactions.Header{
						Sender: paymentSender,
					},
					PaymentTxnFields: transactions.PaymentTxnFields{
						Receiver: paymentReceiver,
						Amount:   basics.MicroAlgos{Raw: 12},
					},
				},
			},
		},
		{
			SignedTxn: transactions.SignedTxn{
				Txn: transactions.Transaction{
					Type: protocol.ApplicationCallTx,
					Header: transactions.Header{
						Sender: assetSender,
					},
					ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
						ApplicationID: 789,
						OnCompletion:  transactions.NoOpOC,
					},
				},
			},
			// also add a fake second-level ApplyData to ensure the recursive part works
			ApplyData: transactions.ApplyData{
				EvalDelta: transactions.EvalDelta{
					InnerTxns: []transactions.SignedTxnWithAD{
						// Inner axfer call
						{
							SignedTxn: transactions.SignedTxn{
								Txn: transactions.Transaction{
									Type: protocol.AssetTransferTx,
									Header: transactions.Header{
										Sender: assetSender,
									},
									AssetTransferTxnFields: transactions.AssetTransferTxnFields{
										AssetReceiver: assetReceiver,
										AssetAmount:   456,
									},
								},
							},
						},
						// Inner application call
						{
							SignedTxn: transactions.SignedTxn{
								Txn: transactions.Transaction{
									Type: protocol.ApplicationCallTx,
									Header: transactions.Header{
										Sender: assetSender,
									},
									ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
										ApplicationID: 789,
										OnCompletion:  transactions.NoOpOC,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return createApp
}

// MakeAppCallWithInnerAppCall creates an app call with 3 levels of app txns:
// application create
//   |- application call
//     |- application create
func MakeAppCallWithInnerAppCall(appSender basics.Address) transactions.SignedTxnWithAD {
	createApp := MakeCreateAppTxn(appSender)

	// Add a log to the outer appl call
	createApp.ApplicationID = 123
	createApp.ApplyData.EvalDelta.Logs = []string{
		"testing outer appl log",
		"appId 123 log",
	}

	// In order to simplify the test,
	// since db.AddBlock uses ApplyData from the block and not from the evaluator,
	// fake ApplyData to have inner txn
	// otherwise it requires funding the app account and other special setup
	createApp.ApplyData.EvalDelta.InnerTxns = []transactions.SignedTxnWithAD{
		// Inner application call
		{
			SignedTxn: transactions.SignedTxn{
				Txn: transactions.Transaction{
					Type: protocol.ApplicationCallTx,
					Header: transactions.Header{
						Sender: appSender,
					},
					ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
						ApplicationID: 789,
						OnCompletion:  transactions.NoOpOC,
					},
				},
			},
			ApplyData: transactions.ApplyData{
				EvalDelta: transactions.EvalDelta{
					Logs: []string{
						"testing inner log",
						"appId 789 log",
					},
					InnerTxns: []transactions.SignedTxnWithAD{
						// Inner application call
						{
							SignedTxn: transactions.SignedTxn{
								Txn: transactions.Transaction{
									Type: protocol.ApplicationCallTx,
									Header: transactions.Header{
										Sender: appSender,
									},
									ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
										ApplicationID: 789,
										OnCompletion:  transactions.NoOpOC,
									},
								},
							},
						},
						// Inner transaction that creates a new application
						{
							SignedTxn: transactions.SignedTxn{
								Txn: transactions.Transaction{
									Type: "appl",
									Header: transactions.Header{
										Sender:      appSender,
										GenesisHash: GenesisHash,
									},
									ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
										ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
										ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
									},
								},
								Sig: Signature,
							},
							// For appl creation in inner txn, the id must be set in ApplyData
							ApplyData: transactions.ApplyData{
								EvalDelta: transactions.EvalDelta{
									Logs: []string{
										"testing inner-inner log",
										"appId 999 log",
									},
								},
								ApplicationID: 999,
							},
						},
					},
				},
			},
		},
	}

	return createApp
}

// MakeBlockForTxns takes some transactions and constructs a block compatible with the indexer import function.
func MakeBlockForTxns(prevHeader bookkeeping.BlockHeader, inputs ...*transactions.SignedTxnWithAD) (bookkeeping.Block, error) {
	res := bookkeeping.MakeBlock(prevHeader)

	res.RewardsState = bookkeeping.RewardsState{
		FeeSink:     FeeAddr,
		RewardsPool: RewardAddr,
	}
	res.TxnCounter = prevHeader.TxnCounter + uint64(len(inputs))

	for _, stxnad := range inputs {
		stxnib, err := res.EncodeSignedTxn(stxnad.SignedTxn, stxnad.ApplyData)
		if err != nil {
			return bookkeeping.Block{}, err
		}

		res.Payset = append(res.Payset, stxnib)
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
				Address: RewardAddr.String(),
				Comment: "RewardsPool",
				State: basics.AccountData{
					MicroAlgos: basics.MicroAlgos{Raw: 100000}, // minimum balance
					Status:     basics.NotParticipating,
				},
			},
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

// MakeGenesisBlock makes a genesis block.
func MakeGenesisBlock() bookkeeping.Block {
	return bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			GenesisID:   MakeGenesis().ID(),
			GenesisHash: GenesisHash,
			RewardsState: bookkeeping.RewardsState{
				FeeSink:     FeeAddr,
				RewardsPool: RewardAddr,
			},
			UpgradeState: bookkeeping.UpgradeState{CurrentProtocol: Proto},
		},
	}
}
