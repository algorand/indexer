package dummy

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/ledger/ledgercore"
)

type dummyIndexerDb struct {
	log *log.Logger
}

// IndexerDb is a mock implementation of IndexerDb
func IndexerDb() idb.IndexerDb {
	return &dummyIndexerDb{}
}

func (db *dummyIndexerDb) Close() {
}

func (db *dummyIndexerDb) AddBlock(block *ledgercore.ValidatedBlock) error {
	db.log.Printf("AddBlock")
	return nil
}

// LoadGenesis is part of idb.IndexerDB
func (db *dummyIndexerDb) LoadGenesis(genesis sdk.Genesis) (err error) {
	return nil
}

// GetNextRoundToAccount is part of idb.IndexerDB
func (db *dummyIndexerDb) GetNextRoundToAccount() (uint64, error) {
	return 0, nil
}

// GetNextRoundToLoad is part of idb.IndexerDB
func (db *dummyIndexerDb) GetNextRoundToLoad() (uint64, error) {
	return 0, nil
}

// GetSpecialAccounts is part of idb.IndexerDb
func (db *dummyIndexerDb) GetSpecialAccounts(ctx context.Context) (types.SpecialAddresses, error) {
	return types.SpecialAddresses{}, nil
}

// GetBlock is part of idb.IndexerDB
func (db *dummyIndexerDb) GetBlock(ctx context.Context, round uint64, options idb.GetBlockOptions) (blockHeader sdk.BlockHeader, transactions []idb.TxnRow, err error) {
	return sdk.BlockHeader{}, nil, nil
}

// Transactions is part of idb.IndexerDB
func (db *dummyIndexerDb) Transactions(ctx context.Context, tf idb.TransactionFilter) (<-chan idb.TxnRow, uint64) {
	return nil, 0
}

// GetAccounts is part of idb.IndexerDB
func (db *dummyIndexerDb) GetAccounts(ctx context.Context, opts idb.AccountQueryOptions) (<-chan idb.AccountRow, uint64) {
	return nil, 0
}

// Assets is part of idb.IndexerDB
func (db *dummyIndexerDb) Assets(ctx context.Context, filter idb.AssetsQuery) (<-chan idb.AssetRow, uint64) {
	return nil, 0
}

// AssetBalances is part of idb.IndexerDB
func (db *dummyIndexerDb) AssetBalances(ctx context.Context, abq idb.AssetBalanceQuery) (<-chan idb.AssetBalanceRow, uint64) {
	return nil, 0
}

// Applications is part of idb.IndexerDB
func (db *dummyIndexerDb) Applications(ctx context.Context, filter idb.ApplicationQuery) (<-chan idb.ApplicationRow, uint64) {
	return nil, 0
}

// AppLocalState is part of idb.IndexerDB
func (db *dummyIndexerDb) AppLocalState(ctx context.Context, filter idb.ApplicationQuery) (<-chan idb.AppLocalStateRow, uint64) {
	return nil, 0
}

// ApplicationBoxes isn't currently implemented
func (db *dummyIndexerDb) ApplicationBoxes(ctx context.Context, filter idb.ApplicationBoxQuery) (<-chan idb.ApplicationBoxRow, uint64) {
	panic("not implemented")
}

// Health is part of idb.IndexerDB
func (db *dummyIndexerDb) Health(ctx context.Context) (state idb.Health, err error) {
	return idb.Health{}, nil
}

// GetNetworkState is part of idb.IndexerDB
func (db *dummyIndexerDb) GetNetworkState() (state idb.NetworkState, err error) {
	return idb.NetworkState{GenesisHash: sdk.Genesis{}.Hash()}, nil
}

// SetNetworkState is part of idb.IndexerDB
func (db *dummyIndexerDb) SetNetworkState(genesis sdk.Digest) error {
	return nil
}

// SetNetworkState is part of idb.IndexerDB
func (db *dummyIndexerDb) DeleteTransactions(ctx context.Context, keep uint64) error {
	return nil
}
