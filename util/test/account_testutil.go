package test

import (
	"crypto/sha512"
	"fmt"
	"math/rand"

	protocol2 "github.com/algorand/indexer/protocol"
	config2 "github.com/algorand/indexer/protocol/config"
	"github.com/algorand/indexer/util"

	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/data/transactions/logic"
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
	GenesisHash = MakeGenesis().Hash()
	// Signature is a signature for transactions used in tests.
	Signature = crypto.Signature{88}

	// Proto is a fake protocol version.
	Proto = protocol.ConsensusFuture

	// PaysetFlat is the payset HashID defined in go-algorand/protocol/hash.go
	PaysetFlat = "PF"
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
					Note:        ArbitraryString(),
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
					Note:        ArbitraryString(),
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
					LastValid:   10,
					Note:        ArbitraryString(),
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
func MakeComplexCreateAppTxn(sender basics.Address, approval, clear string, assemblerVersion uint64) (transactions.SignedTxnWithAD, error) {
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
					Sender:      sender,
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
func MakeAppDestroyTxn(appid uint64, sender basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      sender,
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
func MakeAppOptInTxn(appid uint64, sender basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      sender,
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
func MakeAppOptOutTxn(appid uint64, sender basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      sender,
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
func MakeSimpleAppCallTxn(appid uint64, sender basics.Address) transactions.SignedTxnWithAD {
	return transactions.SignedTxnWithAD{
		SignedTxn: transactions.SignedTxn{
			Txn: transactions.Transaction{
				Type: "appl",
				Header: transactions.Header{
					Sender:      sender,
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
func MakeAppCallTxnWithBoxes(appid uint64, sender basics.Address, appArgs []string, boxNames []string) transactions.SignedTxnWithAD {
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
					Sender:      sender,
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
func MakeAppCallTxnWithLogs(appid uint64, sender basics.Address, logs []string) (txn transactions.SignedTxnWithAD) {
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
						Note:   ArbitraryString(),
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
										Sender: assetSender,
										Note:   ArbitraryString(),
									},
									AssetTransferTxnFields: transactions.AssetTransferTxnFields{
										AssetReceiver: assetReceiver,
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
func MakeAppCallWithMultiLogs(appSender basics.Address) transactions.SignedTxnWithAD {
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
						Sender: appSender,
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
										Sender: appSender,
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
										Sender:      appSender,
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

//==============================
// TODO: test utils returning sdk types. Rename before release

// MakeAssetConfigTxnV2 is a helper to ensure test asset config are initialized.
func MakeAssetConfigTxnV2(configid, total, decimals uint64, defaultFrozen bool, unitName, assetName, url string, addr sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "acfg",
				Header: sdk.Header{
					Sender:      addr,
					Fee:         sdk.MicroAlgos(1000),
					GenesisHash: sdk.Digest(GenesisHash),
					Note:        ArbitraryString(),
				},
				AssetConfigTxnFields: sdk.AssetConfigTxnFields{
					ConfigAsset: sdk.AssetIndex(configid),
					AssetParams: sdk.AssetParams{
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
			Sig: sdk.Signature(Signature),
		},
	}
}

// MakeAssetTransferTxnV2 creates an asset transfer transaction.
func MakeAssetTransferTxnV2(assetid, amt uint64, sender, receiver, close sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "axfer",
				Header: sdk.Header{
					Sender:      sender,
					Fee:         sdk.MicroAlgos(1000),
					GenesisHash: sdk.Digest(GenesisHash),
					Note:        ArbitraryString(),
				},
				AssetTransferTxnFields: sdk.AssetTransferTxnFields{
					XferAsset:   sdk.AssetIndex(assetid),
					AssetAmount: amt,
					//only used for clawback transactions
					//AssetSender:   basics.Address{},
					AssetReceiver: receiver,
					AssetCloseTo:  close,
				},
			},
			Sig: sdk.Signature(Signature),
		},
	}
}

// MakePaymentTxnV2 creates an algo transfer transaction.
func MakePaymentTxnV2(fee, amt, closeAmt, sendRewards, receiveRewards,
	closeRewards uint64, sender, receiver, close, rekeyTo sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "pay",
				Header: sdk.Header{
					Sender:      sender,
					Fee:         sdk.MicroAlgos(fee),
					GenesisHash: sdk.Digest(GenesisHash),
					RekeyTo:     rekeyTo,
					LastValid:   10,
					Note:        ArbitraryString(),
				},
				PaymentTxnFields: sdk.PaymentTxnFields{
					Receiver:         receiver,
					Amount:           sdk.MicroAlgos(amt),
					CloseRemainderTo: close,
				},
			},
			Sig: sdk.Signature(Signature),
		},
		ApplyData: sdk.ApplyData{
			ClosingAmount:   sdk.MicroAlgos(closeAmt),
			SenderRewards:   sdk.MicroAlgos(sendRewards),
			ReceiverRewards: sdk.MicroAlgos(receiveRewards),
			CloseRewards:    sdk.MicroAlgos(closeRewards),
		},
	}
}

// MakeCreateAppTxnV2 makes a transaction that creates a simple application.
func MakeCreateAppTxnV2(sender sdk.Address) sdk.SignedTxnWithAD {
	// Create a transaction with ExtraProgramPages field set to 1
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "appl",
				Header: sdk.Header{
					Sender:      sender,
					GenesisHash: sdk.Digest(GenesisHash),
					Note:        ArbitraryString(),
				},
				ApplicationFields: sdk.ApplicationFields{
					ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
						ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
						ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					},
				},
			},
			Sig: sdk.Signature(Signature),
		},
	}
}

// MakeSimpleAppCallTxnV2 is a MakeSimpleAppCallTxn return sdk.SignedTxnWithAD.
func MakeSimpleAppCallTxnV2(appid uint64, sender sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "appl",
				Header: sdk.Header{
					Sender:      sender,
					GenesisHash: sdk.Digest(GenesisHash),
					Note:        ArbitraryString(),
				},
				ApplicationFields: sdk.ApplicationFields{
					ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
						ApplicationID:     sdk.AppIndex(appid),
						OnCompletion:      sdk.NoOpOC,
						ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
						ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					},
				},
			},
			Sig: sdk.Signature(Signature),
		},
	}
}

// MakeAppCallWithInnerTxnV2 is a MakeAppCallWithInnerTxn returning sdk.SignedTxnWithAD
func MakeAppCallWithInnerTxnV2(appSender, paymentSender, paymentReceiver, assetSender, assetReceiver sdk.Address) sdk.SignedTxnWithAD {
	createApp := MakeCreateAppTxnV2(appSender)

	// In order to simplify the test,
	// since db.AddBlock uses ApplyData from the block and not from the evaluator,
	// fake ApplyData to have inner txn
	// otherwise it requires funding the app account and other special setup
	createApp.ApplyData.EvalDelta.InnerTxns = []sdk.SignedTxnWithAD{
		{
			SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{
					Type: sdk.PaymentTx,
					Header: sdk.Header{
						Sender: paymentSender,
						Note:   ArbitraryString(),
					},
					PaymentTxnFields: sdk.PaymentTxnFields{
						Receiver: paymentReceiver,
						Amount:   sdk.MicroAlgos(12),
					},
				},
			},
		},
		{
			SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{
					Type: sdk.ApplicationCallTx,
					Header: sdk.Header{
						Sender: assetSender,
						Note:   ArbitraryString(),
					},
					ApplicationFields: sdk.ApplicationFields{
						ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
							ApplicationID:     789,
							OnCompletion:      sdk.NoOpOC,
							ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
							ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
						},
					},
				},
			},
			// also add a fake second-level ApplyData to ensure the recursive part works
			ApplyData: sdk.ApplyData{
				EvalDelta: sdk.EvalDelta{
					InnerTxns: []sdk.SignedTxnWithAD{
						// Inner axfer call
						{
							SignedTxn: sdk.SignedTxn{
								Txn: sdk.Transaction{
									Type: sdk.AssetTransferTx,
									Header: sdk.Header{
										Sender: assetSender,
										Note:   ArbitraryString(),
									},
									AssetTransferTxnFields: sdk.AssetTransferTxnFields{
										AssetReceiver: assetReceiver,
										AssetAmount:   456,
									},
								},
							},
						},
						// Inner application call
						MakeSimpleAppCallTxnV2(789, assetSender),
					},
				},
			},
		},
	}

	return createApp
}

// MakeBlockForTxnsV2 takes some transactions and constructs a block compatible with the indexer import function.
func MakeBlockForTxnsV2(prevHeader sdk.BlockHeader, inputs ...*sdk.SignedTxnWithAD) (sdk.Block, error) {
	res := sdk.Block{BlockHeader: prevHeader}
	res.Round = prevHeader.Round + 1
	res.RewardsState = sdk.RewardsState{
		FeeSink:     sdk.Address(FeeAddr),
		RewardsPool: sdk.Address(RewardAddr),
	}
	res.TxnCounter = prevHeader.TxnCounter + uint64(len(inputs))

	for _, stxnad := range inputs {
		stxnib, err := util.EncodeSignedTxn(res.BlockHeader, stxnad.SignedTxn, stxnad.ApplyData)
		if err != nil {
			return sdk.Block{}, err
		}

		res.Payset = append(res.Payset, stxnib)
	}

	return res, nil
}

// MakeGenesisBlockV2 makes a genesis block.
func MakeGenesisBlockV2() sdk.Block {
	params := config2.Consensus[protocol2.ConsensusVersion(Proto)]
	genesis := MakeGenesisV2()
	// payset hashRep
	data := msgpack.Encode(sdk.Payset{})
	hashRep := []byte(PaysetFlat)
	hashRep = append(hashRep, data...)

	blk := sdk.Block{
		BlockHeader: sdk.BlockHeader{
			Round:  0,
			Branch: sdk.BlockHash{},
			Seed:   sdk.Seed(genesis.Hash()),
			TxnCommitments: sdk.TxnCommitments{
				NativeSha512_256Commitment: sdk.Digest(sha512.Sum512_256(hashRep)),
				Sha256Commitment:           sdk.Digest{},
			},
			TimeStamp:   genesis.Timestamp,
			GenesisID:   genesis.ID(),
			GenesisHash: sdk.Digest(GenesisHash),
			RewardsState: sdk.RewardsState{
				FeeSink:                   sdk.Address(FeeAddr),
				RewardsPool:               sdk.Address(RewardAddr),
				RewardsRecalculationRound: sdk.Round(params.RewardsRateRefreshInterval),
			},
			UpgradeState: sdk.UpgradeState{
				CurrentProtocol: "future",
			},
			UpgradeVote: sdk.UpgradeVote{},
		},
	}

	return blk
}

// MakeGenesisV2 creates a sample sdk.Genesis info.
func MakeGenesisV2() sdk.Genesis {
	return sdk.Genesis{
		SchemaID: "main",
		Network:  "mynet",
		Proto:    string(Proto),
		Allocation: []sdk.GenesisAllocation{
			{
				Address: RewardAddr.String(),
				Comment: "RewardsPool",
				State: sdk.Account{
					MicroAlgos: 100000, // minimum balance
					Status:     2,
				},
			},
			{
				Address: AccountA.String(),
				State: sdk.Account{
					MicroAlgos: 1000 * 1000 * 1000 * 1000,
				},
			},
			{
				Address: AccountB.String(),
				State: sdk.Account{
					MicroAlgos: 1000 * 1000 * 1000 * 1000,
				},
			},
			{
				Address: AccountC.String(),
				State: sdk.Account{
					MicroAlgos: 1000 * 1000 * 1000 * 1000,
				},
			},
			{
				Address: AccountD.String(),
				State: sdk.Account{
					MicroAlgos: 1000 * 1000 * 1000 * 1000,
				},
			},
		},
		RewardsPool: RewardAddr.String(),
		FeeSink:     FeeAddr.String(),
	}
}
