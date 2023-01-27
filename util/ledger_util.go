package util

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	algodConfig "github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/protocol"
)

// ReadGenesis converts a reader into a Genesis file.
func ReadGenesis(in io.Reader) (sdk.Genesis, error) {
	var genesis sdk.Genesis
	if in == nil {
		return sdk.Genesis{}, fmt.Errorf("ReadGenesis() err: reader is nil")
	}
	gbytes, err := ioutil.ReadAll(in)
	if err != nil {
		return sdk.Genesis{}, fmt.Errorf("ReadGenesis() err: %w", err)
	}
	err = protocol.DecodeJSON(gbytes, &genesis)
	if err != nil {
		return sdk.Genesis{}, fmt.Errorf("ReadGenesis() decode err: %w", err)
	}
	return genesis, nil
}

// CreateInitState makes an initState
func createInitState(genesis *bookkeeping.Genesis) (ledgercore.InitState, error) {
	balances, err := genesis.Balances()
	if err != nil {
		return ledgercore.InitState{}, fmt.Errorf("MakeProcessor() err: %w", err)
	}
	genesisBlock, err := bookkeeping.MakeGenesisBlock(genesis.Proto, balances, genesis.ID(), genesis.Hash())
	if err != nil {
		return ledgercore.InitState{}, fmt.Errorf("MakeProcessor() err: %w", err)
	}

	accounts := make(map[basics.Address]basics.AccountData)
	for _, alloc := range genesis.Allocation {
		address, err := basics.UnmarshalChecksumAddress(alloc.Address)
		if err != nil {
			return ledgercore.InitState{}, fmt.Errorf("openLedger() decode address err: %w", err)
		}
		accounts[address] = alloc.State
	}
	initState := ledgercore.InitState{
		Block:       genesisBlock,
		Accounts:    accounts,
		GenesisHash: genesisBlock.GenesisHash(),
	}
	return initState, nil
}

// MakeLedger opens a ledger, initializing if necessary.
func MakeLedger(logger *log.Logger, inMemory bool, genesis *bookkeeping.Genesis, dataDir string) (*ledger.Ledger, error) {
	const prefix = "ledger"
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("MakeProcessor() failed to create '%s': %w", dataDir, err)
	}
	dbPrefix := filepath.Join(dataDir, prefix)
	initState, err := createInitState(genesis)
	if err != nil {
		return nil, fmt.Errorf("MakeProcessor() err: %w", err)
	}
	return ledger.OpenLedger(logging.NewWrappedLogger(logger), dbPrefix, inMemory, initState, algodConfig.GetDefaultLocal())
}
