package generator

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"

	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
)

type GenerationConfig struct {
	TxnPerBlock uint64
	NewAccountFrequency uint64

	Protocol string
	NumGenesisAccounts uint64
	GenesisAccountInitialBalance uint64
	GenesisID string
	GenesisHash [32]byte
}

func MakeGenerator(config GenerationConfig) *Generator {
	feeSink := [32]byte{}
	feeSink[31] = 1
	rewardsPool := [32]byte{}
	rewardsPool[31] = 2

	gen := &Generator{
		config:                    config,
		feeSink:                   feeSink,
		rewardsPool:               rewardsPool,
		prevBlockHash:             "",
		round:                     0,
		txnCounter:                0,
		timestamp:                 0,
		rewardsLevel:              0,
		rewardsResidue:            0,
		rewardsRate:               0,
		rewardsRecalculationRound: 0,
	}

	gen.initializeAccounting()

	return gen
}

type Generator struct {
	config GenerationConfig

	// Block generation features
	//txnPerBlock uint64

	// payment transaction metadata
	//newAccountFrequency uint64
	numPayments uint64

	// Also included in numAccounts, but needed for generateGenesis
	//numGenesisAccounts           uint64
	//genesisAccountInitialBalance uint64
	numAccounts        uint64

	// Blockchain stuff
	round           uint64
	txnCounter      uint64
	prevBlockHash   string
	//currentProtocol string
	timestamp       int64
	//genesisID       string
	//genesisHash		[32]byte

	// Rewards stuff
	feeSink                   [32]byte
	rewardsPool               [32]byte
	rewardsLevel              uint64
	rewardsResidue            uint64
	rewardsRate               uint64
	rewardsRecalculationRound uint64

	// balances To avoid crypto and reduce storage, accounts are faked.
	// The account is based on the index into the balances array.
	balances    []uint64

	assetBalances map[uint32][]uint64
	numAssets   uint64
}

type SimpleBlock struct {
	RewardsLevel              uint64                `codec:"earn"`
	FeeSink                   [32]byte              `codec:"fees""`
	RewardsResidue            uint64                `codec:"frac"`
	GenesisID                 string                `codec:"gen"`
	GenesisHash               [32]byte              `codec:"gh"`
	PrevBlockHash             string                `codec:"prev"`
	CurrentProtocol           string                `codec:"proto"`
	RewardsRate               uint64                `codec:"rate"`
	Round                     uint64                `codec:"rnd"`
	RewardsRecalculationRound uint64                `codec:"rwcalr"`
	RewardsPool               [32]byte              `codec:"rwd"`
	Seed                      [32]byte              `codec:"seed"`
	TxnCounter                uint64                `codec:"tc"`
	TimeStamp                 int64                 `codec:"ts"`
	TxnRoot                   [32]byte              `codec:"txn"`
	Payset                    []sdk_types.SignedTxn `codec:"txns"` // Hopefully msgpack works if you embed other msgpack objects, may need to adjust this part.
}

func (g *Generator) WriteGenesis(output io.Writer) {

}

func (g *Generator) generateTransaction(round uint64, intra uint64) (sdk_types.SignedTxn, error) {
	// TODO: Distribute transactions according to configuration
	return g.generatePaymentTxn(round, intra)
}

// WriteBlock generates a block full of new transactions and writes it to the writer.
func (g *Generator) WriteBlock(output io.Writer) {
	// Generate the transactions
	transactions := make([]sdk_types.SignedTxn, 0, g.config.TxnPerBlock)
	for i := uint64(0); i < g.config.TxnPerBlock; i++ {
		txn, err := g.generateTransaction(g.round, i)
		if err != nil {
			panic(fmt.Sprintf("failed to generate transaction: %v\n", err))
		}
		transactions = append(transactions, txn)
	}

	block := SimpleBlock{
		// Special accounts
		RewardsPool:               g.rewardsPool,
		FeeSink:                   [32]byte{0x01},

		// Rewards
		RewardsLevel:              0,
		RewardsResidue:            0,
		RewardsRate:               0,
		RewardsRecalculationRound: 0,

		// Bookkeeping
		Round:                     g.round,
		GenesisID:                 g.config.GenesisID,
		GenesisHash:               g.config.GenesisHash,
		PrevBlockHash:             "prev-block",
		CurrentProtocol:           g.config.Protocol,
		TimeStamp:                 g.timestamp,

		// Transactions
		TxnCounter:                g.txnCounter,
		Seed:                      [32]byte{},
		TxnRoot:                   [32]byte{},
		Payset:                    transactions,
	}

	g.txnCounter += g.config.TxnPerBlock
	g.timestamp  += 4500
	g.round      += 1

	fmt.Println(g.txnCounter)
	output.Write(msgpack.Encode(block))
}

func indexToAccount(i uint64) (result [32]byte) {
	binary.LittleEndian.PutUint64(result[:], i)
	return
}

// initializeAccounting creates the genesis accounts.
func (g *Generator) initializeAccounting() {
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
func (g *Generator) generatePaymentTxn(round uint64, intra uint64) (sdk_types.SignedTxn, error) {
	var receiveIndex uint64
	if g.numPayments % g.config.NewAccountFrequency == 0 {
		g.balances = append(g.balances, 0)
		g.numAccounts++
		receiveIndex = g.numAccounts - 1
	} else {
		receiveIndex = rand.Uint64() % g.numAccounts
	}

	sendIndex    := g.numPayments % g.numAccounts
	sender       := indexToAccount(sendIndex)
	receiver     := indexToAccount(receiveIndex)

	amount := g.balances[sendIndex] / 2
	g.balances[sendIndex] -= amount
	g.balances[receiveIndex] += amount

	if g.balances[sendIndex] < 0 {
		panic(fmt.Sprintf("the balance fell below zero for idx %d", sendIndex))
	}

	g.numPayments++
	txn := sdk_types.Transaction{
		Type:                   "pay",
		Header:                 sdk_types.Header{
			Sender:      sender,
			Fee:         1000,
			FirstValid:  sdk_types.Round(round),
			LastValid:   sdk_types.Round(round + 1000),
			//Note:        nil,
			GenesisID:   g.config.GenesisID,
			GenesisHash: g.config.GenesisHash,
			//Group:       sdk_types.Digest{},
			//Lease:       [32]byte{},
			//RekeyTo:     sdk_types.Address{},
		},
		//KeyregTxnFields:        sdk_types.KeyregTxnFields{},
		PaymentTxnFields:       sdk_types.PaymentTxnFields{
			Receiver:         receiver,
			Amount:           sdk_types.MicroAlgos(amount),
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
	_, err := rand.Read(stxn.Sig[:])
	if err != nil {
		fmt.Println("Failed to generate a random signature")
	}
	 */
	return stxn, nil
}