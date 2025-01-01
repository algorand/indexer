package test

import (
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"os"

	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/util"

	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/v2/protocol"
	"github.com/algorand/go-algorand-sdk/v2/protocol/config"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
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
	Signature = sdk.Signature{88}

	// Proto is a fake protocol version.
	Proto = protocol.ConsensusFuture

	// PaysetFlat is the payset HashID defined in go-algorand/protocol/hash.go
	PaysetFlat = "PF"
)

// DecodeAddressOrPanic is a helper to ensure addresses are initialized.
func DecodeAddressOrPanic(addr string) sdk.Address {
	if addr == "" {
		return sdk.Address{}
	}

	result, err := sdk.DecodeAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decode address: '%s'", addr))
	}
	return result
}

// ArbitraryString should be used to generate a pseudo-random string to put in the Note field of a Txn Header.
// This is necessary to ensure the hash of any two txns used in tests are never the same.
func ArbitraryString() []byte {
	arb := make([]byte, config.MaxTxnNoteBytes)
	_, err := rand.Read(arb)
	if err != nil {
		panic("rand.Read error")
	}
	return arb
}

// MakeAssetConfigTxn is a helper to ensure test asset config are initialized.
func MakeAssetConfigTxn(configid, total, decimals uint64, defaultFrozen bool, unitName, assetName, url string, addr sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "acfg",
				Header: sdk.Header{
					Sender:      addr,
					Fee:         sdk.MicroAlgos(1000),
					GenesisHash: GenesisHash,
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
			Sig: Signature,
		},
	}
}

// MakeAssetTransferTxn creates an asset transfer transaction.
func MakeAssetTransferTxn(assetid, amt uint64, sender, receiver, closeTo sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "axfer",
				Header: sdk.Header{
					Sender:      sender,
					Fee:         sdk.MicroAlgos(1000),
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				AssetTransferTxnFields: sdk.AssetTransferTxnFields{
					XferAsset:   sdk.AssetIndex(assetid),
					AssetAmount: amt,
					//only used for clawback transactions
					//AssetSender:   sdk.Address{},
					AssetReceiver: receiver,
					AssetCloseTo:  closeTo,
				},
			},
			Sig: Signature,
		},
	}
}

// MakePaymentTxn creates an algo transfer transaction.
func MakePaymentTxn(fee, amt, closeAmt, sendRewards, receiveRewards,
	closeRewards uint64, sender, receiver, closeTo, rekeyTo sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "pay",
				Header: sdk.Header{
					Sender:      sender,
					Fee:         sdk.MicroAlgos(fee),
					GenesisHash: GenesisHash,
					RekeyTo:     rekeyTo,
					LastValid:   10,
					Note:        ArbitraryString(),
				},
				PaymentTxnFields: sdk.PaymentTxnFields{
					Receiver:         receiver,
					Amount:           sdk.MicroAlgos(amt),
					CloseRemainderTo: closeTo,
				},
			},
			Sig: Signature,
		},
		ApplyData: sdk.ApplyData{
			ClosingAmount:   sdk.MicroAlgos(closeAmt),
			SenderRewards:   sdk.MicroAlgos(sendRewards),
			ReceiverRewards: sdk.MicroAlgos(receiveRewards),
			CloseRewards:    sdk.MicroAlgos(closeRewards),
		},
	}
}

// MakeAppDestroyTxn makes a transaction that destroys an app.
func MakeAppDestroyTxn(appid uint64, sender sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "appl",
				Header: sdk.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				ApplicationFields: sdk.ApplicationFields{
					ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
						ApplicationID:     sdk.AppIndex(appid),
						OnCompletion:      sdk.DeleteApplicationOC,
						ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
						ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					},
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAssetDestroyTxn makes a transaction that destroys an asset.
func MakeAssetDestroyTxn(assetID uint64, sender sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "acfg",
				Header: sdk.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				AssetConfigTxnFields: sdk.AssetConfigTxnFields{
					ConfigAsset: sdk.AssetIndex(assetID),
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAppOptInTxn makes a transaction that opts in an app.
func MakeAppOptInTxn(appid uint64, sender sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "appl",
				Header: sdk.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				ApplicationFields: sdk.ApplicationFields{
					ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
						ApplicationID: sdk.AppIndex(appid),
						OnCompletion:  sdk.OptInOC,
					},
				},
			},
			Sig: Signature,
		},
	}
}

// MakeAssetOptInTxn makes a transaction that opts in an asset.
func MakeAssetOptInTxn(assetid uint64, address sdk.Address) sdk.SignedTxnWithAD {
	return MakeAssetTransferTxn(assetid, 0, address, address, sdk.Address{})
}

// MakeCreateAppTxn makes a transaction that creates a simple application.
func MakeCreateAppTxn(sender sdk.Address) sdk.SignedTxnWithAD {
	// Create a transaction with ExtraProgramPages field set to 1
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "appl",
				Header: sdk.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
					Note:        ArbitraryString(),
				},
				ApplicationFields: sdk.ApplicationFields{
					ApplicationCallTxnFields: sdk.ApplicationCallTxnFields{
						ApprovalProgram:   []byte{0x02, 0x20, 0x01, 0x01, 0x22},
						ClearStateProgram: []byte{0x02, 0x20, 0x01, 0x01, 0x22},
					},
				},
			},
			Sig: Signature,
		},
	}
}

// MakeSimpleAppCallTxn is a MakeSimpleAppCallTxn return sdk.SignedTxnWithAD.
func MakeSimpleAppCallTxn(appid uint64, sender sdk.Address) sdk.SignedTxnWithAD {
	return sdk.SignedTxnWithAD{
		SignedTxn: sdk.SignedTxn{
			Txn: sdk.Transaction{
				Type: "appl",
				Header: sdk.Header{
					Sender:      sender,
					GenesisHash: GenesisHash,
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
			Sig: Signature,
		},
	}
}

// MakeAppCallWithInnerTxn is a MakeAppCallWithInnerTxn returning sdk.SignedTxnWithAD
func MakeAppCallWithInnerTxn(appSender, paymentSender, paymentReceiver, assetSender, assetReceiver sdk.Address) sdk.SignedTxnWithAD {
	createApp := MakeCreateAppTxn(appSender)

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
						MakeSimpleAppCallTxn(789, assetSender),
					},
				},
			},
		},
	}

	return createApp
}

// MakeHeartbeatTxn creates a heartbeat transaction, overriding fields with the provided values.
func MakeHeartbeatTxn(sender, hbAddress sdk.Address) sdk.SignedTxnWithAD {
	var hbTxn sdk.SignedTxn
	_ = msgpack.Decode(loadResourceFileOrPanic("test_resources/heartbeat.txn"), &hbTxn)

	hbTxn.Txn.Sender = sender
	hbTxn.Txn.GenesisHash = GenesisHash
	var fields = hbTxn.Txn.HeartbeatTxnFields
	fields.HbAddress = hbAddress
	return sdk.SignedTxnWithAD{
		SignedTxn: hbTxn,
	}
}

func loadResourceFileOrPanic(path string) []byte {
	data, err := os.ReadFile(path)

	if err != nil {
		panic(fmt.Sprintf("Failed to load resource file: '%s'", path))
	}
	var ret idb.TxnRow
	_ = msgpack.Decode(data, &ret)
	return data
}

// MakeBlockForTxns takes some transactions and constructs a block compatible with the indexer import function.
func MakeBlockForTxns(prevHeader sdk.BlockHeader, inputs ...*sdk.SignedTxnWithAD) (sdk.Block, error) {
	res := sdk.Block{BlockHeader: prevHeader}
	res.Round = prevHeader.Round + 1
	res.RewardsState = sdk.RewardsState{
		FeeSink:     FeeAddr,
		RewardsPool: RewardAddr,
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

// MakeGenesisBlock makes a genesis block.
func MakeGenesisBlock() sdk.Block {
	params := config.Consensus[Proto]
	genesis := MakeGenesis()
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
			GenesisHash: GenesisHash,
			RewardsState: sdk.RewardsState{
				FeeSink:                   FeeAddr,
				RewardsPool:               RewardAddr,
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

// MakeGenesis creates a sample sdk.Genesis info.
func MakeGenesis() sdk.Genesis {
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
