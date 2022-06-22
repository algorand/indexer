package util

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	algodConfig "github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/ledger"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/protocol"
)

// ReadGenesis converts a reader into a Genesis file.
func ReadGenesis(in io.Reader) (bookkeeping.Genesis, error) {
	var genesis bookkeeping.Genesis
	if in == nil {
		return bookkeeping.Genesis{}, fmt.Errorf("readGenesis() err: reader is nil")
	}
	gbytes, err := ioutil.ReadAll(in)
	if err != nil {
		return bookkeeping.Genesis{}, fmt.Errorf("error reading genesis, %v", err)
	}
	err = protocol.DecodeJSON(gbytes, &genesis)
	if err != nil {
		return bookkeeping.Genesis{}, fmt.Errorf("error decoding genesis, %v", err)
	}
	return genesis, nil
}

// CreateInitState makes an initState
func CreateInitState(genesis *bookkeeping.Genesis) (ledgercore.InitState, error) {
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
func MakeLedger(logger *log.Logger, genesis *bookkeeping.Genesis, dataDir string) (*ledger.Ledger, error) {
	const prefix = "ledger"
	dbPrefix := filepath.Join(dataDir, prefix)
	initState, err := CreateInitState(genesis)
	if err != nil {
		return nil, fmt.Errorf("MakeProcessor() err: %w", err)
	}
	return ledger.OpenLedger(logging.NewLogger(), dbPrefix, false, initState, algodConfig.GetDefaultLocal())

	// TODO: Adding a new logger constructor to go-algorand so that we can wrap the indexer logger.
	//return ledger.OpenLedger(logging.NewWrappedLogger(logger), dbPrefix, false, initState, algodConfig.GetDefaultLocal())
}
