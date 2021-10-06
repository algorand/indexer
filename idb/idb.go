package idb

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"

	models "github.com/algorand/indexer/api/generated/v2"
)

// TxnRow is metadata relating to one transaction in a transaction query.
type TxnRow struct {
	// Round is the round where the transaction was committed.
	Round uint64

	// Round time  is the block time when the block was confirmed.
	RoundTime time.Time

	// Intra is the offset into the block where this transaction was placed.
	Intra int

	// TxnBytes is the raw signed transaction with apply data object.
	TxnBytes []byte

	// AssetID is the ID of any asset or application created by this transaction.
	AssetID uint64

	// Extra are some additional fields which might be related to to the transaction.
	Extra TxnExtra

	// Error indicates that there was an internal problem processing the expected transaction.
	Error error
}

// Next returns what should be an opaque string to be returned in the next query to resume where a previous limit left off.
func (tr TxnRow) Next() string {
	var b [12]byte
	binary.LittleEndian.PutUint64(b[:8], tr.Round)
	binary.LittleEndian.PutUint32(b[8:], uint32(tr.Intra))
	return base64.URLEncoding.EncodeToString(b[:])
}

// DecodeTxnRowNext unpacks opaque string returned from TxnRow.Next()
func DecodeTxnRowNext(s string) (round uint64, intra uint32, err error) {
	var b []byte
	b, err = base64.URLEncoding.DecodeString(s)
	if err != nil {
		return
	}
	round = binary.LittleEndian.Uint64(b[:8])
	intra = binary.LittleEndian.Uint32(b[8:])
	return
}

// TxnExtra is some additional metadata needed for a transaction.
type TxnExtra struct {
	AssetCloseAmount uint64 `codec:"aca,omitempty"`
	RootTxid         string `codec:"root-txid,omitempty"`
}

// ErrorNotInitialized is used when requesting something that can't be returned
// because initialization has not been completed.
var ErrorNotInitialized error = errors.New("accounting not initialized")

// ErrorBlockNotFound is used when requesting a block that isn't in the DB.
var ErrorBlockNotFound = errors.New("block not found")

// IndexerDb is the interface used to define alternative Indexer backends.
// TODO: sqlite3 impl
// TODO: cockroachdb impl
type IndexerDb interface {
	// Import a block and do the accounting.
	AddBlock(block *bookkeeping.Block) error

	LoadGenesis(genesis bookkeeping.Genesis) (err error)

	// GetNextRoundToAccount returns ErrorNotInitialized if genesis is not loaded.
	GetNextRoundToAccount() (uint64, error)
	GetSpecialAccounts() (transactions.SpecialAddresses, error)

	GetBlock(ctx context.Context, round uint64, options GetBlockOptions) (blockHeader bookkeeping.BlockHeader, transactions []TxnRow, err error)

	// The next multiple functions return a channel with results as well as the latest round
	// accounted.
	Transactions(ctx context.Context, tf TransactionFilter) (<-chan TxnRow, uint64)
	GetAccounts(ctx context.Context, opts AccountQueryOptions) (<-chan AccountRow, uint64)
	Assets(ctx context.Context, filter AssetsQuery) (<-chan AssetRow, uint64)
	AssetBalances(ctx context.Context, abq AssetBalanceQuery) (<-chan AssetBalanceRow, uint64)
	Applications(ctx context.Context, filter *models.SearchForApplicationsParams) (<-chan ApplicationRow, uint64)

	Health() (status Health, err error)
}

// GetBlockOptions contains the options when requesting to load a block from the database.
type GetBlockOptions struct {
	// setting Transactions to true suggests requesting to receive the trasnactions themselves from the GetBlock query
	Transactions bool
}

// TransactionFilter is a parameter object with all the transaction filter options.
type TransactionFilter struct {
	// Address filtering transactions for one Address will
	// return transactions newest-first proceding into the
	// past. Paging through such results can be achieved by
	// setting a MaxRound to get results before.
	Address []byte

	AddressRole AddressRole // 0=Any, otherwise bitfields as defined in address_role.go

	MinRound   uint64
	MaxRound   uint64
	AfterTime  time.Time
	BeforeTime time.Time
	TypeEnum   TxnTypeEnum // ["","pay","keyreg","acfg","axfer","afrz"]
	Txid       string
	Round      *uint64 // nil for no filter
	Offset     *uint64 // nil for no filter
	OffsetLT   *uint64 // nil for no filter
	OffsetGT   *uint64 // nil for no filter
	SigType    SigType // ["", "sig", "msig", "lsig"]
	NotePrefix []byte
	AlgosGT    *uint64 // implictly filters on "pay" txns for Algos > this. This will be a slightly faster query than EffectiveAmountGT.
	AlgosLT    *uint64
	RekeyTo    *bool // nil for no filter

	AssetID       uint64 // filter transactions relevant to an asset
	AssetAmountGT *uint64
	AssetAmountLT *uint64

	ApplicationID uint64 // filter transactions relevant to an application

	EffectiveAmountGT *uint64 // Algo: Amount + CloseAmount > x
	EffectiveAmountLT *uint64 // Algo: Amount + CloseAmount < x

	// pointer to last returned object of previous query
	NextToken string

	Limit uint64
}

// AccountQueryOptions is a parameter object with all of the account filter options.
type AccountQueryOptions struct {
	GreaterThanAddress []byte // for paging results
	EqualToAddress     []byte // return exactly this one account

	// return any accounts with this auth addr
	EqualToAuthAddr []byte

	// Filter on accounts with current balance greater than x
	AlgosGreaterThan *uint64
	// Filter on accounts with current balance less than x.
	AlgosLessThan *uint64

	// HasAssetID, AssetGT, and AssetLT are implemented in Go code
	// after data has returned from Postgres and thus are slightly
	// less efficient. They will turn on IncludeAssetHoldings.
	HasAssetID uint64
	AssetGT    *uint64
	AssetLT    *uint64

	HasAppID uint64

	IncludeAssetHoldings bool
	IncludeAssetParams   bool

	// IncludeDeleted indicated whether to include deleted Assets, Applications, etc within the account.
	IncludeDeleted bool

	Limit uint64
}

// AccountRow is metadata relating to one account in a account query.
type AccountRow struct {
	Account models.Account
	Error   error
}

// AssetsQuery is a parameter object with all of the asset filter options.
type AssetsQuery struct {
	AssetID            uint64
	AssetIDGreaterThan uint64

	Creator []byte

	// Name is a case insensitive substring comparison of the asset name
	Name string
	// Unit is a case insensitive substring comparison of the asset unit
	Unit string
	// Query checks for fuzzy match against either asset name or unit name
	// (assetname ILIKE '%?%' OR unitname ILIKE '%?%')
	Query string

	// IncludeDeleted indicated whether to include deleted Assets in the results.
	IncludeDeleted bool

	Limit uint64
}

// AssetRow is metadata relating to one asset in a asset query.
type AssetRow struct {
	AssetID      uint64
	Creator      []byte
	Params       basics.AssetParams
	Error        error
	CreatedRound *uint64
	ClosedRound  *uint64
	Deleted      *bool
}

// AssetBalanceQuery is a parameter object with all of the asset balance filter options.
type AssetBalanceQuery struct {
	AssetID  uint64
	AmountGT *uint64 // only rows > this
	AmountLT *uint64 // only rows < this

	// IncludeDeleted indicated whether to include deleted AssetHoldingss in the results.
	IncludeDeleted bool

	Limit uint64 // max rows to return

	// PrevAddress for paging, the last item from the previous
	// query (items returned in address order)
	PrevAddress []byte
}

// AssetBalanceRow is metadata relating to one asset balance in an asset balance query.
type AssetBalanceRow struct {
	Address      []byte
	AssetID      uint64
	Amount       uint64
	Frozen       bool
	Error        error
	CreatedRound *uint64
	ClosedRound  *uint64
	Deleted      *bool
}

// ApplicationRow is metadata relating to one application in an application query.
type ApplicationRow struct {
	Application models.Application
	Error       error
}

// IndexerDbOptions are the options common to all indexer backends.
type IndexerDbOptions struct {
	ReadOnly bool
}

// Health is the response object that IndexerDb objects need to return from the Health method.
type Health struct {
	Data        *map[string]interface{} `json:"data,omitempty"`
	Round       uint64                  `json:"round"`
	IsMigrating bool                    `json:"is-migrating"`
	DBAvailable bool                    `json:"db-available"`
	Error       string                  `json:"error"`
}
