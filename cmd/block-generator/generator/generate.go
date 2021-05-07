package generator

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/types"
)

// GenerationConfig defines the tunable parameters for block generation.
type GenerationConfig struct {
	// Block generation features
	TxnPerBlock         uint64
	NewAccountFrequency uint64

	Protocol                     string
	NumGenesisAccounts           uint64
	GenesisAccountInitialBalance uint64
	GenesisID                    string
	GenesisHash                  sdk_types.Digest
}

// MakeGenerator initializes the Generator object.
func MakeGenerator(config GenerationConfig) Generator {
	gen := &generator{
		config:                    config,
		prevBlockHash:             "",
		round:                     0,
		txnCounter:                0,
		timestamp:                 0,
		rewardsLevel:              0,
		rewardsResidue:            0,
		rewardsRate:               0,
		rewardsRecalculationRound: 0,
	}

	gen.feeSink[31] = 1
	gen.rewardsPool[31] = 2

	gen.initializeAccounting()

	return gen
}

// Generator is the interface needed to generate blocks.
type Generator interface {
	WriteBlock(output io.Writer, round uint64)
	WriteGenesis(output io.Writer)
}

type generator struct {
	config GenerationConfig

	// payment transaction metadata
	numPayments uint64

	// Number of algorand accounts
	numAccounts uint64

	// Blockchain stuff
	round         uint64
	txnCounter    uint64
	prevBlockHash string
	//currentProtocol string
	timestamp int64
	//genesisID       string
	//genesisHash		[32]byte

	// Rewards stuff
	feeSink                   types.Address
	rewardsPool               types.Address
	rewardsLevel              uint64
	rewardsResidue            uint64
	rewardsRate               uint64
	rewardsRecalculationRound uint64

	// balances To avoid crypto and reduce storage, accounts are faked.
	// The account is based on the index into the balances array.
	balances []uint64

	assetBalances map[uint32][]uint64
	numAssets     uint64
}

func (g *generator) WriteGenesis(output io.Writer) {
	var allocations []types.GenesisAllocation

	for i := uint64(0); i < g.config.NumGenesisAccounts; i++ {
		addr := indexToAccount(i)
		allocations = append(allocations, types.GenesisAllocation{
			Address: addr.String(),
			State: types.AccountData{
				MicroAlgos: types.MicroAlgos(g.config.GenesisAccountInitialBalance),
			},
		})
	}

	gen := types.Genesis{
		SchemaID:    "v1",
		Network:     "generated-network",
		Proto:       "future",
		Allocation:  allocations,
		RewardsPool: g.rewardsPool.String(),
		FeeSink:     g.feeSink.String(),
		Timestamp:   g.timestamp,
	}

	output.Write(json.Encode(gen))
}

func (g *generator) generateTransaction(round uint64, intra uint64) (types.SignedTxnWithAD, error) {
	// TODO: Distribute transactions according to configuration
	return g.generatePaymentTxn(round, intra)
}

// WriteBlock generates a block full of new transactions and writes it to the writer.
func (g *generator) WriteBlock(output io.Writer, _ uint64) {
	// Generate the transactions
	transactions := make([]types.SignedTxnInBlock, 0, g.config.TxnPerBlock)
	for i := uint64(0); i < g.config.TxnPerBlock; i++ {
		txn, err := g.generateTransaction(g.round, i)
		stxnib := types.SignedTxnInBlock{
			SignedTxnWithAD: txn,
			HasGenesisID:    true,
			HasGenesisHash:  true,
		}
		if err != nil {
			panic(fmt.Sprintf("failed to generate transaction: %v\n", err))
		}
		transactions = append(transactions, stxnib)
	}

	block := types.Block{
		BlockHeader: types.BlockHeader{
			Round:       types.Round(g.round),
			Branch:      [32]byte{},
			Seed:        types.Seed{},
			TxnRoot:     types.Digest{},
			TimeStamp:   g.timestamp,
			GenesisID:   g.config.GenesisID,
			GenesisHash: g.config.GenesisHash,
			RewardsState: types.RewardsState{
				FeeSink:                   g.feeSink,
				RewardsPool:               g.rewardsPool,
				RewardsLevel:              0,
				RewardsRate:               0,
				RewardsResidue:            0,
				RewardsRecalculationRound: 0,
			},
			UpgradeState: types.UpgradeState{
				CurrentProtocol: "future",
			},
			//UpgradeVote:  types.UpgradeVote{},
			TxnCounter:  g.txnCounter,
			CompactCert: nil,
		},
		Payset: types.Payset(transactions),
	}

	cert := types.EncodedBlockCert{
		Block:       block,
		Certificate: types.Certificate{},
	}

	g.txnCounter += g.config.TxnPerBlock
	g.timestamp += 4500
	g.round++

	fmt.Println(g.txnCounter)
	output.Write(msgpack.Encode(cert))
}

func indexToAccount(i uint64) (addr sdk_types.Address) {
	binary.LittleEndian.PutUint64(addr[:], i)
	return
}

// initializeAccounting creates the genesis accounts.
func (g *generator) initializeAccounting() {
	if g.config.NumGenesisAccounts == 0 {
		panic("Number of genesis accounts must be > 0.")
	}

	g.numPayments = 0
	g.numAccounts = g.config.NumGenesisAccounts
	for i := uint64(0); i < g.config.NumGenesisAccounts; i++ {
		g.balances = append(g.balances, g.config.GenesisAccountInitialBalance)
	}
}

// generatePaymentTxn creates a new payment transaction.
func (g *generator) generatePaymentTxn(round uint64, intra uint64) (types.SignedTxnWithAD, error) {
	var receiveIndex uint64
	if g.numPayments%g.config.NewAccountFrequency == 0 {
		g.balances = append(g.balances, 0)
		g.numAccounts++
		receiveIndex = g.numAccounts - 1
	} else {
		receiveIndex = rand.Uint64() % g.numAccounts
	}

	sendIndex := g.numPayments % g.numAccounts
	sender := indexToAccount(sendIndex)
	receiver := indexToAccount(receiveIndex)

	if g.balances[sendIndex] < 2 {
		panic(fmt.Sprintf("the sender account does not have two microalgos to rub together. idx %d, payment number %d", sendIndex, g.numPayments))
	}

	amount := g.balances[sendIndex] / 2
	g.balances[sendIndex] -= amount
	g.balances[receiveIndex] += amount

	g.numPayments++
	txn := sdk_types.Transaction{
		Type: "pay",
		Header: sdk_types.Header{
			Sender:     sender,
			Fee:        1000,
			FirstValid: sdk_types.Round(round),
			LastValid:  sdk_types.Round(round + 1000),
			//Note:        nil,
			GenesisID:   g.config.GenesisID,
			GenesisHash: g.config.GenesisHash,
			//Group:       sdk_types.Digest{},
			//Lease:       [32]byte{},
			//RekeyTo:     sdk_types.Address{},
		},
		//KeyregTxnFields:        sdk_types.KeyregTxnFields{},
		PaymentTxnFields: sdk_types.PaymentTxnFields{
			Receiver: receiver,
			Amount:   sdk_types.MicroAlgos(amount),
			//CloseRemainderTo: sdk_types.Address{},
		},
		//AssetConfigTxnFields:   sdk_types.AssetConfigTxnFields{},
		//AssetTransferTxnFields: sdk_types.AssetTransferTxnFields{},
		//AssetFreezeTxnFields:   sdk_types.AssetFreezeTxnFields{},
		//ApplicationFields:      sdk_types.ApplicationFields{},
	}

	stxn := sdk_types.SignedTxn{
		Sig:      sdk_types.Signature{},
		Msig:     sdk_types.MultisigSig{},
		Lsig:     sdk_types.LogicSig{},
		Txn:      txn,
		AuthAddr: sdk_types.Address{},
	}
	stxn.Sig[32] = 50
	/*
		// Would it be useful to generate a random signature?
		_, err := rand.Read(stxn.Sig[:])
		if err != nil {
			fmt.Println("Failed to generate a random signature")
		}
	*/

	withAd := types.SignedTxnWithAD{
		SignedTxn: stxn,
		ApplyData: types.ApplyData{},
	}
	return withAd, nil
}
