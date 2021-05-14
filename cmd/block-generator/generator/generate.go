package generator

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"os"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/future"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/types"
	"github.com/algorand/indexer/util"
)

const (
	consensusTime int64 = 4500
)

var outOfRangeError = fmt.Errorf("selection is out of weighted range")

const (
	// Generator types
	paymentTx = "pay"
	assetTx   = "acfg"
	//keyRegistrationTx = "keyreg"
	//applicationCallTx = "appl"

	// Asset Tx Types
	assetCreate  = "create"
	assetOptin   = "optin"
	assetXfer    = "xfer"
	assetClose   = "close"
	assetDestroy = "destroy"

	assetTotal = uint64(100000000000000000)
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

	// Assets to asset holder balances.
	assets []*assetData

	transactionWeights []float32
	assetTxWeights     []float32
}

type assetData struct {
	assetID  uint64
	creator  uint64
	holdings []assetHolding
	holders  map[uint64]bool
}

type assetHolding struct {
	acctIndex uint64
	balance   uint64
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

func getTransactionOptions() []interface{} {
	return []interface{}{paymentTx, assetTx}
}

func (g *generator) generateTransaction(sp sdk_types.SuggestedParams, round uint64, intra uint64) (types.SignedTxnInBlock, error) {
	if len(g.transactionWeights) == 0 {
		g.transactionWeights = append(g.transactionWeights, g.config.PaymentTransactionFraction)
		g.transactionWeights = append(g.transactionWeights, g.config.AssetTransactionFraction)
	}

	selection, err := weightedSelection(g.transactionWeights, getTransactionOptions())
	if err != nil {
		if err == outOfRangeError {
			// Default type
			selection = paymentTx
			err = nil
		} else {
			return types.SignedTxnInBlock{}, err
		}
	}

	switch selection {
	case paymentTx:
		return g.generatePaymentTxn(sp, round, intra)
	case assetTx:
		return g.generateAssetTxn(sp, round, intra)
	default:
		return types.SignedTxnInBlock{}, fmt.Errorf("no generator available for %s", selection)
	}
}

// WriteBlock generates a block full of new transactions and writes it to the writer.
func (g *generator) WriteBlock(output io.Writer, round uint64) {
	if round != g.round {
		fmt.Printf("Generator only supports sequential block access. Expected %d but received request for %d.", g.round, round)
	}

	// Generate the transactions
	sp := g.getSuggestedParams(round)
	transactions := make([]types.SignedTxnInBlock, 0, g.config.TxnPerBlock)
	for i := uint64(0); i < g.config.TxnPerBlock; i++ {
		txn, err := g.generateTransaction(sp, g.round, i)
		if err != nil {
			panic(fmt.Sprintf("failed to generate transaction: %v\n", err))
		}
		transactions = append(transactions, txn)
	}

	g.txnCounter += g.config.TxnPerBlock

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

	g.timestamp += consensusTime
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
func convert(txn sdk_types.Transaction, ad types.ApplyData) types.SignedTxnInBlock {
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

	stxnib := types.SignedTxnInBlock{
		SignedTxnWithAD: withAd,
		HasGenesisID:    true,
		HasGenesisHash:  true,
	}
	return stxnib
}

func (g *generator) getSuggestedParams(round uint64) sdk_types.SuggestedParams {
	return sdk_types.SuggestedParams{
		Fee:              1000,
		GenesisID:        g.genesisID,
		GenesisHash:      g.genesisHash[:],
		FirstRoundValid:  sdk_types.Round(round),
		LastRoundValid:   sdk_types.Round(round + 1000),
		ConsensusVersion: "",
		FlatFee:          true,
		MinFee:           1000,
	}
}

// generatePaymentTxn creates a new payment transaction. The sender is always a genesis account, the receiver is random,
// or a new account.
func (g *generator) generatePaymentTxn(sp sdk_types.SuggestedParams, _ uint64, _ uint64) (types.SignedTxnInBlock, error) {
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

	// TODO: Unnecessary serialize/deserialize of addresses.
	// Easy to optimize by creating the struct ourselves if needed.
	txn, err := future.MakePaymentTxn(sender.String(), receiver.String(), amount, nil, "", sp)
	util.MaybeFail(err, "unable to make transaction")
	return convert(txn, types.ApplyData{}), nil
}

func getAssetTxOptions() []interface{} {
	return []interface{}{assetCreate, assetDestroy, assetOptin, assetXfer, assetClose}
}

func (g *generator) generateAssetTxnInternal(txType interface{}, sp sdk_types.SuggestedParams, intra uint64) (txn types.Transaction) {
	return g.generateAssetTxnInternalHint(txType, sp, intra, 0, nil)
}

func (g *generator) generateAssetTxnInternalHint(txType interface{}, sp sdk_types.SuggestedParams, intra uint64, hintIndex uint64, hint *assetData) (txn types.Transaction) {
	// If there are no assets the next operation needs to be a create.
	if len(g.assets) == 0 {
		txType = assetCreate
	}

	var err error
	numAssets := uint64(len(g.assets))
	var senderIndex uint64

	if txType == assetCreate {
		senderIndex = numAssets % g.numAccounts
		senderAcct := indexToAccount(senderIndex)
		senderAcctStr := senderAcct.String()

		total := assetTotal
		assetID := g.txnCounter + intra + 1
		assetName := fmt.Sprintf("asset #%d", assetID)
		txn, err = future.MakeAssetCreateTxn(senderAcctStr, nil, sp, total, 0, false, senderAcctStr, senderAcctStr, senderAcctStr, senderAcctStr, "tokens", assetName, "https://algorand.com", "metadata!")
		util.MaybeFail(err, "unable to make asset create transaction")

		// Compute asset ID and initialize holdings
		a := assetData{
			assetID:  assetID,
			creator:  senderIndex,
			holdings: []assetHolding{{acctIndex: senderIndex, balance: total}},
			holders: map[uint64]bool{senderIndex: true},
		}

		g.assets = append(g.assets, &a)
	} else {
		assetIndex := rand.Uint64() % numAssets
		asset := g.assets[assetIndex]
		if hint != nil {
			assetIndex = hintIndex
			asset = hint
		}

		switch txType {
		case assetDestroy:
			// delete asset

			// If the creator doesn't have all of them, close instead
			if asset.holdings[0].balance != assetTotal {
				return g.generateAssetTxnInternalHint(assetClose, sp, intra, assetIndex, asset)
			}

			senderIndex = asset.creator
			creator := indexToAccount(senderIndex)
			creatorString := creator.String()

			txn, err = future.MakeAssetDestroyTxn(creatorString, nil, sp, asset.assetID)
			util.MaybeFail(err, "unable to make asset destroy transaction")

			// Remove asset by swapping the element to delete then trimming the last
			g.assets[numAssets-1], g.assets[assetIndex] = g.assets[assetIndex], g.assets[numAssets-1]
			g.assets = g.assets[:numAssets-1]
		case assetOptin:
			// select a random account from asset to optin

			// If every account holds the asset, close instead of optin
			if uint64(len(asset.holdings)) == g.numAccounts {
				return g.generateAssetTxnInternalHint(assetClose, sp, intra, assetIndex, asset)
			}

			// look for an account that does not hold the asset
			exists := true
			for exists {
				senderIndex = rand.Uint64() % g.numAccounts
				exists = asset.holders[senderIndex]
			}
			account := indexToAccount(senderIndex)
			accountString := account.String()

			txn, err = future.MakeAssetAcceptanceTxn(accountString, nil, sp, asset.assetID)
			util.MaybeFail(err, "unable to make asset destroy transaction")

			asset.holdings = append(asset.holdings, assetHolding{
				acctIndex: senderIndex,
				balance:   0,
			})
			asset.holders[senderIndex] = true
		case assetXfer:
			// send from creator (holder[0]) to another random holder (same address is valid)

			// If there aren't enough assets to close one, optin an account instead
			if len(asset.holdings) == 1 {
				return g.generateAssetTxnInternalHint(assetOptin, sp, intra, assetIndex, asset)
			}

			senderIndex = asset.holdings[0].acctIndex
			senderString := indexToAccount(senderIndex).String()

			receiverIndex := (rand.Uint64() % (uint64(len(asset.holdings)) - uint64(1))) + uint64(1)
			receiver := indexToAccount(asset.holdings[receiverIndex].acctIndex)
			receiverString := receiver.String()

			txn, err = future.MakeAssetTransferTxn(senderString, receiverString, 10, nil, sp, "", asset.assetID)
			util.MaybeFail(err, "unable to make asset destroy transaction")

			if asset.holdings[0].balance < 10 {
				fmt.Printf(fmt.Sprintf("\n\ncreator doesn't have enough funds for asset %d\n\n", asset.assetID))
				os.Exit(1)
			}

			asset.holdings[0].balance -= 10
			asset.holdings[receiverIndex].balance += 10
		case assetClose:
			// select a holder of a random asset to close out
			// If there aren't enough assets to close one, optin an account instead
			if len(asset.holdings) == 1 {
				return g.generateAssetTxnInternalHint(assetOptin, sp, intra, assetIndex, asset)
			}

			numHoldings := uint64(len(asset.holdings))
			closeIndex := (rand.Uint64() % (numHoldings - 1)) + uint64(1)
			senderIndex = asset.holdings[closeIndex].acctIndex
			senderString := indexToAccount(senderIndex).String()

			closeToAcctIndex := asset.holdings[0].acctIndex
			closeToAcct := indexToAccount(closeToAcctIndex)
			closeToAcctString := closeToAcct.String()

			txn, err = future.MakeAssetTransferTxn(senderString, closeToAcctString, 0, nil, sp, closeToAcctString, asset.assetID)
			util.MaybeFail(err, "unable to make asset destroy transaction")

			asset.holdings[0].balance += asset.holdings[closeIndex].balance

			// Remove asset by swapping the element to delete then trimming the last
			asset.holdings[numHoldings-1], asset.holdings[closeIndex] = asset.holdings[closeIndex], asset.holdings[numHoldings-1]
			asset.holdings = asset.holdings[:numHoldings-1]
			delete(asset.holders, senderIndex)
		default:
		}
	}

	if indexToAccount(senderIndex) != txn.Sender {
		fmt.Printf("failed to properly set sender index.")
		os.Exit(1)
	}

	g.balances[senderIndex] -= uint64(txn.Fee)
	return
}

func (g *generator) generateAssetTxn(sp sdk_types.SuggestedParams, _ uint64, intra uint64) (types.SignedTxnInBlock, error) {
	if len(g.assetTxWeights) == 0 {
		g.assetTxWeights = append(g.assetTxWeights, g.config.AssetCreateFraction)
		g.assetTxWeights = append(g.assetTxWeights, g.config.AssetDestroyFraction)
		g.assetTxWeights = append(g.assetTxWeights, g.config.AssetOptinFraction)
		g.assetTxWeights = append(g.assetTxWeights, g.config.AssetXferFraction)
		g.assetTxWeights = append(g.assetTxWeights, g.config.AssetCloseFraction)
	}

	selection, err := weightedSelection(g.assetTxWeights, getAssetTxOptions())
	if err != nil {
		if err == outOfRangeError {
			// Default type
			selection = assetXfer
			err = nil
		} else {
			return types.SignedTxnInBlock{}, err
		}
	}

	txn := g.generateAssetTxnInternal(selection, sp, intra)

	if txn.Type == "" {
		fmt.Println("Empty asset transaction.")
		os.Exit(1)
	}

	return convert(txn, types.ApplyData{}), nil
}
