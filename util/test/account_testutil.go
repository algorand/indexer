package test

import (
	"fmt"
	"math/rand"

	"github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/data/transactions/logic"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/indexer/util"
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
	GenesisHash = MakeGenesis().Hash()
	// Signature is a signature for transactions used in tests.
	Signature = crypto.Signature{88}

	// Proto is a fake protocol version.
	Proto = protocol.ConsensusFuture
)

// DecodeAddressOrPanic is a helper to ensure addresses are initialized.
func DecodeAddressOrPanic(addr string) types.Address {
	if addr == "" {
		return types.Address{}
	}

	result, err := util.UnmarshalChecksumAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decode address: '%s'", addr))
	}
	return result
}

// MakeAssetConfigTxn is a helper to ensure test asset config are initialized.
func MakeAssetConfigTxn(configid, total, decimals uint64, defaultFrozen bool, unitName, assetName, url string, addr types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "acfg",
				Header: transactions.Header{
					Sender:      basics.Address(addr),
					Fee:         basics.MicroAlgos{Raw: 1000},
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
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
						Manager:       basics.Address(addr),
						Reserve:       basics.Address(addr),
						Freeze:        basics.Address(addr),
						Clawback:      basics.Address(addr),
					},
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAssetFreezeTxn create an asset freeze/unfreeze transaction.
func MakeAssetFreezeTxn(assetid uint64, frozen bool, sender, freezeAccount types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "afrz",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					Fee:         basics.MicroAlgos{Raw: 1000},
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				AssetFreezeTxnFields: transactions.AssetFreezeTxnFields{
					FreezeAccount: basics.Address(freezeAccount),
					FreezeAsset:   basics.AssetIndex(assetid),
					AssetFrozen:   frozen,
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAssetTransferTxn creates an asset transfer transaction.
func MakeAssetTransferTxn(assetid, amt uint64, sender, receiver, close types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "axfer",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					Fee:         basics.MicroAlgos{Raw: 1000},
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				AssetTransferTxnFields: transactions.AssetTransferTxnFields{
					XferAsset:   basics.AssetIndex(assetid),
					AssetAmount: amt,
					//only used for clawback transactions
					AssetReceiver: basics.Address(receiver),
					AssetCloseTo:  basics.Address(close),
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAssetOptInTxn makes a transaction that opts in an asset.
func MakeAssetOptInTxn(assetid uint64, address types.Address) transactions.SignedTxnWithAD {
	return MakeAssetTransferTxn(assetid, 0, address, address, types.Address{})
}

// MakeAssetDestroyTxn makes a transaction that destroys an asset.
func MakeAssetDestroyTxn(assetID uint64, sender types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "acfg",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
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
	closeRewards uint64, sender, receiver, close, rekeyTo types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "pay",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					Fee:         basics.MicroAlgos{Raw: fee},
					GenesisHash: GenesisHash,
					RekeyTo:     basics.Address(rekeyTo),
					LastValid:   10,
					Note:        ArbitraryString(),
				},
				PaymentTxnFields: transactions.PaymentTxnFields{
					Receiver:         basics.Address(receiver),
					Amount:           basics.MicroAlgos{Raw: amt},
					CloseRemainderTo: basics.Address(close),
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

// MakeCreateAppTxn makes a transaction that creates a simple application.
func MakeCreateAppTxn(sender types.Address) transactions.SignedTxnWithAD {
	// Create a transaction with ExtraProgramPages field set to 1
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
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

// MakeComplexCreateAppTxn makes a transaction that creates an arbitrary app. When assemblerVersion is set to 0, use the AssemblerDefaultVersion.
func MakeComplexCreateAppTxn(sender types.Address, approval, clear string, assemblerVersion uint64) (transactions.SignedTxnWithAD, error) {
	// Create a transaction with ExtraProgramPages field set to 1
	approvalOps, err := logic.AssembleStringWithVersion(approval, assemblerVersion)
	if err != nil {
		return transactions.SignedTxnWithAD{}, err
	}
	clearOps, err := logic.AssembleString(clear)
	if err != nil {
		return transactions.SignedTxnWithAD{}, err
	}

	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					GenesisHash: GenesisHash,
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApprovalProgram:   approvalOps.Program,
					ClearStateProgram: clearOps.Program,
				},
			},
			Sig: Signature,
		},
	}, nil
}

// MakeAppDestroyTxn makes a transaction that destroys an app.
func MakeAppDestroyTxn(appid uint64, sender types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApplicationID:     basics.AppIndex(appid),
					OnCompletion:      transactions.DeleteApplicationOC,
					ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAppOptInTxn makes a transaction that opts in an app.
func MakeAppOptInTxn(appid uint64, sender types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
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
func MakeAppOptOutTxn(appid uint64, sender types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
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

// MakeSimpleAppCallTxn makes an appl transaction with a NoOp upon completion.
func MakeSimpleAppCallTxn(appid uint64, sender types.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApplicationID:     basics.AppIndex(appid),
					OnCompletion:      transactions.NoOpOC,
					ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAppCallTxnWithBoxes makes an appl transaction with a NoOp upon completion.
func MakeAppCallTxnWithBoxes(appid uint64, sender types.Address, appArgs []string, boxNames []string) transactions.SignedTxnWithAD {
	appArgBytes := [][]byte{}
	for _, appArg := range appArgs {
		appArgBytes = append(appArgBytes, []byte(appArg))
	}
	appBoxes := []transactions.BoxRef{}
	for _, boxName := range boxNames {
		// hard-coding box reference to current app
		appBoxes = append(appBoxes, transactions.BoxRef{Index: uint64(0), Name: []byte(boxName)})
	}
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      basics.Address(sender),
					GenesisHash: GenesisHash,
				},
				ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
					ApplicationID:   basics.AppIndex(appid),
					OnCompletion:    transactions.NoOpOC,
					ApplicationArgs: appArgBytes,
					Boxes:           appBoxes,
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAppCallTxnWithLogs makes an appl NoOp transaction with initialized logs.
func MakeAppCallTxnWithLogs(appid uint64, sender types.Address, logs []string) (txn transactions.SignedTxnWithAD) {
	txn = MakeSimpleAppCallTxn(appid, sender)
	txn.ApplyData.EvalDelta.Logs = logs
	return
}

// MakeAppCallWithInnerTxn creates an app call with 3 levels of transactions:
// application create
// |- payment
// |- application call
//    |- asset transfer
//    |- application call
func MakeAppCallWithInnerTxn(appSender, paymentSender, paymentReceiver, assetSender, assetReceiver types.Address) transactions.SignedTxnWithAD {
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
						Sender: basics.Address(paymentSender),
						Note:   ArbitraryString(),
					},
					PaymentTxnFields: transactions.PaymentTxnFields{
						Receiver: basics.Address(paymentReceiver),
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
						Sender: basics.Address(assetSender),
						Note:   ArbitraryString(),
					},
					ApplicationCallTxnFields: transactions.ApplicationCallTxnFields{
						ApplicationID:     789,
						OnCompletion:      transactions.NoOpOC,
						ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
						ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
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
										Sender: basics.Address(assetSender),
										Note:   ArbitraryString(),
									},
									AssetTransferTxnFields: transactions.AssetTransferTxnFields{
										AssetReceiver: basics.Address(assetReceiver),
										AssetAmount:   456,
									},
								},
							},
						},
						// Inner application call
						MakeSimpleAppCallTxn(789, assetSender),
					},
				},
			},
		},
	}

	return createApp
}

// MakeAppCallWithMultiLogs creates an app call that creates multiple logs
// at the same level.
// application create
//   |- application call
//     |- application call
//   |- application call
//   |- application call
//   |- application call
func MakeAppCallWithMultiLogs(appSender types.Address) transactions.SignedTxnWithAD {
	createApp := MakeCreateAppTxn(appSender)

	// Add a log to the outer appl call
	createApp.ApplicationID = 123
	createApp.ApplyData.EvalDelta.Logs = []string{
		"testing outer appl log",
		"appId 123 log",
	}

	createApp.ApplyData.EvalDelta.InnerTxns = []transactions.SignedTxnWithAD{
		{
			SignedTxn: transactions.SignedTxn{
				Txn: transactions.Transaction{
					Type: protocol.ApplicationCallTx,
					Header: transactions.Header{
						Sender: basics.Address(appSender),
						Note:   ArbitraryString(),
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
						// Inner application call
						MakeSimpleAppCallTxn(789, appSender),
					},
					Logs: []string{
						"testing inner log",
						"appId 789 log",
					},
				},
			},
		},
		MakeAppCallTxnWithLogs(222, appSender, []string{
			"testing multiple logs 1",
			"appId 222 log 1",
		}),
		MakeAppCallTxnWithLogs(222, appSender, []string{
			"testing multiple logs 2",
			"appId 222 log 2",
		}),
		MakeAppCallTxnWithLogs(222, appSender, []string{
			"testing multiple logs 3",
			"appId 222 log 3",
		}),
	}

	return createApp
}

// MakeAppCallWithInnerAppCall creates an app call with 3 levels of app txns:
// application create
//   |- application call
//     |- application create
func MakeAppCallWithInnerAppCall(appSender types.Address) transactions.SignedTxnWithAD {
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
						Sender: basics.Address(appSender),
						Note:   ArbitraryString(),
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
										Sender: basics.Address(appSender),
										Note:   ArbitraryString(),
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
										Sender:      basics.Address(appSender),
										GenesisHash: GenesisHash,
										Note:        ArbitraryString(),
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
		FeeSink:     basics.Address(FeeAddr),
		RewardsPool: basics.Address(RewardAddr),
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
	genesis := MakeGenesis()
	balances, err := genesis.Balances()
	if err != nil {
		return bookkeeping.Block{}
	}
	genesisBlock, err := bookkeeping.MakeGenesisBlock(genesis.Proto, balances, genesis.ID(), genesis.Hash())
	if err != nil {
		return bookkeeping.Block{}
	}
	return genesisBlock
}

// ArbitraryString should be used to generate a pseudo-random string to put in the Note field of a Txn Header.
// This is necessary to ensure the hash of any two txns used in tests are never the same.
func ArbitraryString() []byte {
	arb := make([]byte, config.MaxTxnNoteBytes)
	rand.Read(arb)
	return arb
}
