package generator

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/committee"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
)

var errOutOfRange = fmt.Errorf("selection is out of weighted range")

// TxTypeID is the transaction type.
type TxTypeID string

const (
	genesis TxTypeID = "genesis"

	// Payment Tx IDs
	paymentTx           TxTypeID = "pay"
	paymentAcctCreateTx TxTypeID = "pay_create"
	assetTx             TxTypeID = "asset"
	//keyRegistrationTx TxTypeID = "keyreg"
	//applicationCallTx TxTypeID = "appl"

	// Asset Tx IDs
	assetCreate  TxTypeID = "asset_create"
	assetOptin   TxTypeID = "asset_optin"
	assetXfer    TxTypeID = "asset_xfer"
	assetClose   TxTypeID = "asset_close"
	assetDestroy TxTypeID = "asset_destroy"

	assetTotal = uint64(100000000000000000)
	fee        = uint64(1000)
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
	PaymentFraction           float32 `mapstructure:"pay_xfer_fraction"`

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

	if !sumIsCloseToOne(config.PaymentNewAccountFraction, config.PaymentFraction) {
		return nil, fmt.Errorf("payment configuration ratios should equal 1")
	}

	if !sumIsCloseToOne(config.AssetCreateFraction, config.AssetDestroyFraction, config.AssetOptinFraction, config.AssetCloseFraction, config.AssetXferFraction) {
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
		reportData:                make(map[TxTypeID]TxData),
	}

	gen.feeSink[31] = 1
	gen.rewardsPool[31] = 2

	gen.initializeAccounting()

	for _, val := range getTransactionOptions() {
		switch val {
		case paymentTx:
			gen.transactionWeights = append(gen.transactionWeights, config.PaymentTransactionFraction)
		case assetTx:
			gen.transactionWeights = append(gen.transactionWeights, config.AssetTransactionFraction)
		}
	}

	for _, val := range getPaymentTxOptions() {
		switch val {
		case paymentTx:
			gen.payTxWeights = append(gen.payTxWeights, config.PaymentFraction)
		case paymentAcctCreateTx:
			gen.payTxWeights = append(gen.payTxWeights, config.PaymentNewAccountFraction)
		}
	}

	for _, val := range getAssetTxOptions() {
		switch val {
		case assetCreate:
			gen.assetTxWeights = append(gen.assetTxWeights, config.AssetCreateFraction)
		case assetDestroy:
			gen.assetTxWeights = append(gen.assetTxWeights, config.AssetDestroyFraction)
		case assetOptin:
			gen.assetTxWeights = append(gen.assetTxWeights, config.AssetOptinFraction)
		case assetXfer:
			gen.assetTxWeights = append(gen.assetTxWeights, config.AssetXferFraction)
		case assetClose:
			gen.assetTxWeights = append(gen.assetTxWeights, config.AssetCloseFraction)
		}
	}

	return gen, nil
}

// Generator is the interface needed to generate blocks.
type Generator interface {
	WriteReport(output io.Writer)
	WriteGenesis(output io.Writer)
	WriteBlock(output io.Writer, round uint64)
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
	protocol      protocol.ConsensusVersion
	genesisID     string
	genesisHash   crypto.Digest

	// Rewards stuff
	feeSink                   basics.Address
	rewardsPool               basics.Address
	rewardsLevel              uint64
	rewardsResidue            uint64
	rewardsRate               uint64
	rewardsRecalculationRound uint64

	// balances for all accounts. To avoid crypto and reduce storage, accounts are faked.
	// The account is based on the index into the balances array.
	balances []uint64

	// assets is a minimal representation of the asset holdings, it doesn't
	// include the frozen state.
	assets []*assetData

	transactionWeights []float32
	payTxWeights       []float32
	assetTxWeights     []float32

	// Reporting information from transaction type to data
	reportData Report
}

type assetData struct {
	assetID uint64
	creator uint64
	// Holding at index 0 is the creator.
	holdings []assetHolding
	// Set of holders in the holdings array for easy reference.
	holders map[uint64]bool
}

type assetHolding struct {
	acctIndex uint64
	balance   uint64
}

// Report is the generation report.
type Report map[TxTypeID]TxData

// TxData is the generator report data.
type TxData struct {
	GenerationTimeMilli time.Duration `json:"generation_time_milli"`
	GenerationCount     uint64        `json:"num_generated"`
}

func track(id TxTypeID) (TxTypeID, time.Time) {
	return id, time.Now()
}
func (g *generator) recordData(id TxTypeID, start time.Time) {
	data := g.reportData[id]
	data.GenerationCount++
	data.GenerationTimeMilli += time.Since(start)
	g.reportData[id] = data
}

func (g *generator) WriteReport(output io.Writer) {
	data := protocol.EncodeJSON(g.reportData)
	var err error
	if err != nil {
		fmt.Fprintf(output, "Problem indenting data: %v", err)
	} else {
		output.Write(data)
	}
}

func (g *generator) WriteGenesis(output io.Writer) {
	defer g.recordData(track(genesis))
	var allocations []bookkeeping.GenesisAllocation

	for i := uint64(0); i < g.config.NumGenesisAccounts; i++ {
		addr := indexToAccount(i)
		allocations = append(allocations, bookkeeping.GenesisAllocation{
			Address: addr.String(),
			State: basics.AccountData{
				MicroAlgos: basics.MicroAlgos{Raw: g.config.GenesisAccountInitialBalance},
			},
		})
	}

	gen := bookkeeping.Genesis{
		SchemaID:    "v1",
		Network:     "generated-network",
		Proto:       g.protocol,
		Allocation:  allocations,
		RewardsPool: g.rewardsPool.String(),
		FeeSink:     g.feeSink.String(),
		Timestamp:   g.timestamp,
	}

	output.Write(protocol.EncodeJSON(gen))
}

func getTransactionOptions() []interface{} {
	return []interface{}{paymentTx, assetTx}
}

func (g *generator) generateTransaction(round uint64, intra uint64) (transactions.SignedTxnInBlock, error) {
	selection, err := weightedSelection(g.transactionWeights, getTransactionOptions(), paymentTx)
	if err != nil {
		return transactions.SignedTxnInBlock{}, err
	}

	switch selection {
	case paymentTx:
		return g.generatePaymentTxn(round, intra)
	case assetTx:
		return g.generateAssetTxn(round, intra)
	default:
		return transactions.SignedTxnInBlock{}, fmt.Errorf("no generator available for %s", selection)
	}
}

// WriteBlock generates a block full of new transactions and writes it to the writer.
func (g *generator) WriteBlock(output io.Writer, round uint64) {
	if round != g.round {
		fmt.Printf("Generator only supports sequential block access. Expected %d but received request for %d.", g.round, round)
	}

	// Generate the transactions
	transactions := make([]transactions.SignedTxnInBlock, 0, g.config.TxnPerBlock)
	// Do not put transactions in round 0
	if round != 0 {
		for i := uint64(0); i < g.config.TxnPerBlock; i++ {
			txn, err := g.generateTransaction(g.round, i)
			if err != nil {
				panic(fmt.Sprintf("failed to generate transaction: %v\n", err))
			}
			transactions = append(transactions, txn)
		}
	}

	g.txnCounter += g.config.TxnPerBlock

	block := bookkeeping.Block{
		BlockHeader: bookkeeping.BlockHeader{
			Round:       basics.Round(g.round),
			Branch:      bookkeeping.BlockHash{},
			Seed:        committee.Seed{},
			TxnRoot:     crypto.Digest{},
			TimeStamp:   g.timestamp,
			GenesisID:   g.genesisID,
			GenesisHash: g.genesisHash,
			RewardsState: bookkeeping.RewardsState{
				FeeSink:                   g.feeSink,
				RewardsPool:               g.rewardsPool,
				RewardsLevel:              0,
				RewardsRate:               0,
				RewardsResidue:            0,
				RewardsRecalculationRound: 0,
			},
			UpgradeState: bookkeeping.UpgradeState{
				CurrentProtocol: g.protocol,
			},
			UpgradeVote: bookkeeping.UpgradeVote{},
			TxnCounter:  g.txnCounter,
			CompactCert: nil,
		},
		Payset: transactions,
	}

	cert := rpcs.EncodedBlockCert{
		Block:       block,
		Certificate: agreement.Certificate{},
	}

	g.timestamp += consensusTimeMilli
	g.round++

	output.Write(protocol.Encode(&cert))
}

func indexToAccount(i uint64) (addr basics.Address) {
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
func convert(txn transactions.Transaction, ad transactions.ApplyData) transactions.SignedTxnInBlock {
	stxn := transactions.SignedTxn{
		Sig:      crypto.Signature{},
		Msig:     crypto.MultisigSig{},
		Lsig:     transactions.LogicSig{},
		Txn:      txn,
		AuthAddr: basics.Address{},
	}

	// TODO: Would it be useful to generate a random signature?
	stxn.Sig[32] = 50

	withAd := transactions.SignedTxnWithAD{
		SignedTxn: stxn,
		// TODO: Add close-amount to apply data
		ApplyData: ad,
	}

	stxnib := transactions.SignedTxnInBlock{
		SignedTxnWithAD: withAd,
		HasGenesisID:    true,
		HasGenesisHash:  true,
	}
	return stxnib
}

func getPaymentTxOptions() []interface{} {
	return []interface{}{paymentTx, paymentAcctCreateTx}
}

// generatePaymentTxn creates a new payment transaction. The sender is always a genesis account, the receiver is random,
// or a new account.
func (g *generator) generatePaymentTxn(round uint64, intra uint64) (transactions.SignedTxnInBlock, error) {
	selection, err := weightedSelection(g.payTxWeights, getPaymentTxOptions(), paymentTx)
	if err != nil {
		return transactions.SignedTxnInBlock{}, err
	}
	return g.generatePaymentTxnInternal(selection.(TxTypeID), round, intra)
}

func (g *generator) generatePaymentTxnInternal(selection TxTypeID, _ uint64 /*round*/, _ uint64 /*intra*/) (transactions.SignedTxnInBlock, error) {
	defer g.recordData(track(selection))

	var receiveIndex uint64
	switch selection {
	case paymentTx:
		receiveIndex = rand.Uint64() % g.numAccounts
	case paymentAcctCreateTx:
		g.balances = append(g.balances, 0)
		g.numAccounts++
		receiveIndex = g.numAccounts - 1
	}

	// Always send from a genesis account.
	sendIndex := g.numPayments % g.config.NumGenesisAccounts

	sender := indexToAccount(sendIndex)
	receiver := indexToAccount(receiveIndex)

	amount := uint64(1000000)
	fee := uint64(1000)
	total := amount + fee
	if g.balances[sendIndex] < total {
		fmt.Printf(fmt.Sprintf("\n\nthe sender account does not have enough algos for the transfer. idx %d, payment number %d\n\n", sendIndex, g.numPayments))
		os.Exit(1)
	}

	g.balances[sendIndex] -= total
	g.balances[receiveIndex] += amount

	g.numPayments++

	txn := g.makePaymentTxn(sender, receiver, amount, basics.Address{})
	return convert(txn, transactions.ApplyData{}), nil
}

func getAssetTxOptions() []interface{} {
	return []interface{}{assetCreate, assetDestroy, assetOptin, assetXfer, assetClose}
}

func (g *generator) generateAssetTxnInternal(txType TxTypeID, intra uint64) (actual TxTypeID, txn transactions.Transaction) {
	return g.generateAssetTxnInternalHint(txType, intra, 0, nil)
}

func (g *generator) generateAssetTxnInternalHint(txType TxTypeID, intra uint64, hintIndex uint64, hint *assetData) (actual TxTypeID, txn transactions.Transaction) {
	actual = txType
	// If there are no assets the next operation needs to be a create.
	if len(g.assets) == 0 {
		actual = assetCreate
	}

	numAssets := uint64(len(g.assets))
	var senderIndex uint64

	if actual == assetCreate {
		senderIndex = g.numPayments % g.config.NumGenesisAccounts
		senderAcct := indexToAccount(senderIndex)

		total := assetTotal
		assetID := g.txnCounter + intra + 1
		assetName := fmt.Sprintf("asset #%d", assetID)
		txn = g.makeAssetCreateTxn(senderAcct, total, false, assetName)

		// Compute asset ID and initialize holdings
		a := assetData{
			assetID:  assetID,
			creator:  senderIndex,
			holdings: []assetHolding{{acctIndex: senderIndex, balance: total}},
			holders:  map[uint64]bool{senderIndex: true},
		}

		g.assets = append(g.assets, &a)
	} else {
		assetIndex := rand.Uint64() % numAssets
		asset := g.assets[assetIndex]
		if hint != nil {
			assetIndex = hintIndex
			asset = hint
		}

		switch actual {
		case assetDestroy:
			// delete asset

			// If the creator doesn't have all of them, close instead
			if asset.holdings[0].balance != assetTotal {
				return g.generateAssetTxnInternalHint(assetClose, intra, assetIndex, asset)
			}

			senderIndex = asset.creator
			creator := indexToAccount(senderIndex)
			txn = g.makeAssetDestroyTxn(creator, asset.assetID)

			// Remove asset by moving the last element to the deleted index then trimming the slice.
			g.assets[assetIndex] = g.assets[numAssets-1]
			g.assets = g.assets[:numAssets-1]
		case assetOptin:
			// select a random account from asset to optin

			// If every account holds the asset, close instead of optin
			if uint64(len(asset.holdings)) == g.numAccounts {
				return g.generateAssetTxnInternalHint(assetClose, intra, assetIndex, asset)
			}

			// look for an account that does not hold the asset
			exists := true
			for exists {
				senderIndex = rand.Uint64() % g.numAccounts
				exists = asset.holders[senderIndex]
			}
			account := indexToAccount(senderIndex)
			txn = g.makeAssetAcceptanceTxn(account, asset.assetID)

			asset.holdings = append(asset.holdings, assetHolding{
				acctIndex: senderIndex,
				balance:   0,
			})
			asset.holders[senderIndex] = true
		case assetXfer:
			// send from creator (holder[0]) to another random holder (same address is valid)

			// If there aren't enough assets to close one, optin an account instead
			if len(asset.holdings) == 1 {
				return g.generateAssetTxnInternalHint(assetOptin, intra, assetIndex, asset)
			}

			senderIndex = asset.holdings[0].acctIndex
			sender := indexToAccount(senderIndex)

			receiverIndex := (rand.Uint64() % (uint64(len(asset.holdings)) - uint64(1))) + uint64(1)
			receiver := indexToAccount(asset.holdings[receiverIndex].acctIndex)

			amount := uint64(10)

			txn = g.makeAssetTransferTxn(
				sender, receiver, amount, basics.Address{}, asset.assetID)

			if asset.holdings[0].balance < amount {
				fmt.Printf(fmt.Sprintf("\n\ncreator doesn't have enough funds for asset %d\n\n", asset.assetID))
				os.Exit(1)
			}
			if g.balances[asset.holdings[0].acctIndex] < fee {
				fmt.Printf(fmt.Sprintf("\n\ncreator doesn't have enough funds for transaction %d\n\n", asset.assetID))
				os.Exit(1)
			}

			asset.holdings[0].balance -= amount
			asset.holdings[receiverIndex].balance += amount
		case assetClose:
			// select a holder of a random asset to close out
			// If there aren't enough assets to close one, optin an account instead
			if len(asset.holdings) == 1 {
				return g.generateAssetTxnInternalHint(assetOptin, intra, assetIndex, asset)
			}

			numHoldings := uint64(len(asset.holdings))
			closeIndex := (rand.Uint64() % (numHoldings - 1)) + uint64(1)
			senderIndex = asset.holdings[closeIndex].acctIndex
			sender := indexToAccount(senderIndex)

			closeToAcctIndex := asset.holdings[0].acctIndex
			closeToAcct := indexToAccount(closeToAcctIndex)

			txn = g.makeAssetTransferTxn(sender, closeToAcct, 0, closeToAcct, asset.assetID)

			asset.holdings[0].balance += asset.holdings[closeIndex].balance

			// Remove asset by moving the last element to the deleted index then trimming the slice.
			asset.holdings[closeIndex] = asset.holdings[numHoldings-1]
			asset.holdings = asset.holdings[:numHoldings-1]
			delete(asset.holders, senderIndex)
		default:
		}
	}

	if indexToAccount(senderIndex) != txn.Sender {
		fmt.Printf("failed to properly set sender index.")
		os.Exit(1)
	}

	if g.balances[senderIndex] < fee {
		fmt.Printf(fmt.Sprintf("\n\nthe sender account does not have enough algos for the transfer. idx %d, asset transaction type %v, num %d\n\n", senderIndex, actual, g.reportData[actual].GenerationCount))
		os.Exit(1)
	}
	g.balances[senderIndex] -= fee
	return
}

func (g *generator) generateAssetTxn(_ uint64 /*round*/, intra uint64) (transactions.SignedTxnInBlock, error) {
	start := time.Now()
	selection, err := weightedSelection(g.assetTxWeights, getAssetTxOptions(), assetXfer)
	if err != nil {
		return transactions.SignedTxnInBlock{}, err
	}

	actual, txn := g.generateAssetTxnInternal(selection.(TxTypeID), intra)
	defer g.recordData(actual, start)

	if txn.Type == "" {
		fmt.Println("Empty asset transaction.")
		os.Exit(1)
	}

	return convert(txn, transactions.ApplyData{}), nil
}
