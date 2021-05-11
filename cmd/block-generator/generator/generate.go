package generator

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"os"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/types"
)

const (
	consensusTimeMilli int64 = 4500
)

// GenerationConfig defines the tunable parameters for block generation.
type GenerationConfig struct {
	Name                         string `mapstructure:"name"`
	NumGenesisAccounts           uint64 `mapstructure:"genesis_accounts"`
	GenesisAccountInitialBalance uint64 `mapstructure:"genesis_account_balance"`

	// Block generation
	TxnPerBlock uint64 `mapstructure:"tx_per_block"`

	// TX Distribution
	PaymentTransactionFraction float32 `mapstructure:"tx_pay_fraction"`
	AssetTransactionFraction   float32 `mapstructure:"tx_asset_fraction"`

	// Payment configuration
	PaymentNewAccountFraction float32 `mapstructure:"pay_acct_create_fraction"`
	PaymentTransferFraction   float32 `mapstructure:"pay_xfer_fraction"`

	// Asset configuration
	AssetCreateFraction  float32 `mapstructure:"asset_create_fraction"`
	AssetDestroyFraction float32 `mapstructure:"asset_destroy_fraction"`
	AssetOptinFraction   float32 `mapstructure:"asset_optin_fraction"`
	AssetCloseFraction   float32 `mapstructure:"asset_close_fraction"`
	AssetXferFraction    float32 `mapstructure:"asset_xfer_fraction"`
}

func sumIsCloseToOne(numbers ...float32) bool {
	var sum float32
	for _, num := range numbers {
		sum += num
	}
	return sum > 0.99 && sum < 1.01
}

// MakeGenerator initializes the Generator object.
func MakeGenerator(config GenerationConfig) (Generator, error) {
	if !sumIsCloseToOne(config.PaymentTransactionFraction, config.AssetTransactionFraction) {
		return nil, fmt.Errorf("transaction distribution ratios should equal 1")
	}

	if !sumIsCloseToOne(config.PaymentNewAccountFraction, config.PaymentTransferFraction) {
		return nil, fmt.Errorf("payment configuration ratios should equal 1")
	}

	if !sumIsCloseToOne(config.AssetCreateFraction, config.AssetOptinFraction, config.AssetCloseFraction, config.AssetXferFraction) {
		return nil, fmt.Errorf("asset configuration ratios should equal 1")
	}

	gen := &generator{
		config:                    config,
		protocol:                  "future",
		genesisHash:               [32]byte{},
		genesisID:                 "blockgen-test",
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

	return gen, nil
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

	// Block stuff
	round         uint64
	txnCounter    uint64
	prevBlockHash string
	timestamp     int64
	protocol      types.ConsensusVersion
	genesisID     string
	genesisHash   sdk_types.Digest

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
		Proto:       g.protocol,
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
func (g *generator) WriteBlock(output io.Writer, round uint64) {
	if round != g.round {
		fmt.Printf("Generator only supports sequential block access. Expected %d but received request for %d.", g.round, round)
	}

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
			Branch:      types.BlockHash{},
			Seed:        types.Seed{},
			TxnRoot:     types.Digest{},
			TimeStamp:   g.timestamp,
			GenesisID:   g.genesisID,
			GenesisHash: g.genesisHash,
			RewardsState: types.RewardsState{
				FeeSink:                   g.feeSink,
				RewardsPool:               g.rewardsPool,
				RewardsLevel:              0,
				RewardsRate:               0,
				RewardsResidue:            0,
				RewardsRecalculationRound: 0,
			},
			UpgradeState: types.UpgradeState{
				CurrentProtocol: g.protocol,
			},
			UpgradeVote: types.UpgradeVote{},
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
	g.timestamp += consensusTimeMilli
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

// convert wraps the transaction as needed for including in the block.
func convert(txn sdk_types.Transaction, ad types.ApplyData) types.SignedTxnWithAD {
	stxn := sdk_types.SignedTxn{
		Sig:      sdk_types.Signature{},
		Msig:     sdk_types.MultisigSig{},
		Lsig:     sdk_types.LogicSig{},
		Txn:      txn,
		AuthAddr: sdk_types.Address{},
	}

	// TODO: Would it be useful to generate a random signature?
	stxn.Sig[32] = 50

	withAd := types.SignedTxnWithAD{
		SignedTxn: stxn,
		// TODO: Add close-amount to apply data
		ApplyData: ad,
	}

	return withAd
}

// generatePaymentTxn creates a new payment transaction. The sender is always a genesis account, the receiver is random,
// or a new account.
func (g *generator) generatePaymentTxn(round uint64, intra uint64) (types.SignedTxnWithAD, error) {
	var receiveIndex uint64
	if g.numPayments%uint64(100*g.config.PaymentNewAccountFraction) == 0 {
		g.balances = append(g.balances, 0)
		g.numAccounts++
		receiveIndex = g.numAccounts - 1
	} else {
		receiveIndex = rand.Uint64() % g.numAccounts
	}

	// Always send from a genesis account.
	sendIndex := g.numPayments % g.config.NumGenesisAccounts

	sender := indexToAccount(sendIndex)
	receiver := indexToAccount(receiveIndex)

	amount := uint64(10000000)
	fee := uint64(1000)
	if g.balances[sendIndex] < 2 {
		fmt.Printf(fmt.Sprintf("\n\nthe sender account does not enough algos for the transfer. idx %d, payment number %d\n\n", sendIndex, g.numPayments))
		os.Exit(1)
	}

	g.balances[sendIndex] -= amount
	g.balances[sendIndex] -= fee
	g.balances[receiveIndex] += amount

	g.numPayments++
	txn := sdk_types.Transaction{
		Type: "pay",
		Header: sdk_types.Header{
			Sender:      sender,
			Fee:         sdk_types.MicroAlgos(fee),
			FirstValid:  sdk_types.Round(round),
			LastValid:   sdk_types.Round(round + 1000),
			Note:        nil,
			GenesisID:   g.genesisID,
			GenesisHash: g.genesisHash,
			Group:       sdk_types.Digest{},
			Lease:       [32]byte{},
			RekeyTo:     sdk_types.Address{},
		},
		KeyregTxnFields: sdk_types.KeyregTxnFields{},
		PaymentTxnFields: sdk_types.PaymentTxnFields{
			Receiver:         receiver,
			Amount:           sdk_types.MicroAlgos(amount),
			CloseRemainderTo: sdk_types.Address{},
		},
		AssetConfigTxnFields:   sdk_types.AssetConfigTxnFields{},
		AssetTransferTxnFields: sdk_types.AssetTransferTxnFields{},
		AssetFreezeTxnFields:   sdk_types.AssetFreezeTxnFields{},
		ApplicationFields:      sdk_types.ApplicationFields{},
	}

	return convert(txn, types.ApplyData{}), nil
}

func (g *generator) generateAssetTxn(round uint64, intra uint64) (types.SignedTxnWithAD, error) {
	return convert(sdk_types.Transaction{}, types.ApplyData{}), nil
}
