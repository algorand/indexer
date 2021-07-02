package dummy

import (
	"context"

	log "github.com/sirupsen/logrus"

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

type dummyIndexerDb struct {
	log *log.Logger
}

// IndexerDb is a mock implementation of IndexerDb
func IndexerDb() idb.IndexerDb {
	return &dummyIndexerDb{}
}

// StartBlock is part of idb.IndexerDB
func (db *dummyIndexerDb) StartBlock() (err error) {
	db.log.Printf("StartBlock")
	return nil
}

// AddTransaction is part of idb.IndexerDB
func (db *dummyIndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error {
	db.log.Printf("\ttxn %d %d %d %d", round, intra, txtypeenum, assetid)
	return nil
}

// CommitBlock is part of idb.IndexerDB
func (db *dummyIndexerDb) CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
	db.log.Printf("CommitBlock %d %d %d header bytes", round, timestamp, len(headerbytes))
	return nil
}

// LoadGenesis is part of idb.IndexerDB
func (db *dummyIndexerDb) LoadGenesis(genesis types.Genesis) (err error) {
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
func (db *dummyIndexerDb) GetSpecialAccounts() (idb.SpecialAccounts, error) {
	return idb.SpecialAccounts{}, nil
}

// GetDefaultFrozen is part of idb.IndexerDb
func (db *dummyIndexerDb) GetDefaultFrozen() (defaultFrozen map[uint64]bool, err error) {
	return make(map[uint64]bool), nil
}

// YieldTxns is part of idb.IndexerDB
func (db *dummyIndexerDb) YieldTxns(ctx context.Context, firstRound uint64) <-chan idb.TxnRow {
	return nil
}

// CommitRoundAccounting is part of idb.IndexerDB
func (db *dummyIndexerDb) CommitRoundAccounting(updates idb.RoundUpdates, round uint64, blockHeader *types.BlockHeader) (err error) {
	return nil
}

// GetBlock is part of idb.IndexerDB
func (db *dummyIndexerDb) GetBlock(ctx context.Context, round uint64, options idb.GetBlockOptions) (blockHeader types.BlockHeader, transactions []idb.TxnRow, err error) {
	return types.BlockHeader{}, nil, nil
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
func (db *dummyIndexerDb) Applications(ctx context.Context, filter *models.SearchForApplicationsParams) (<-chan idb.ApplicationRow, uint64) {
	return nil, 0
}

// Health is part of idb.IndexerDB
func (db *dummyIndexerDb) Health() (state idb.Health, err error) {
	return idb.Health{}, nil
}

func (db *dummyIndexerDb) Reset() (err error) {
	return nil
}
