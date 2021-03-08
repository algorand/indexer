package idb

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
	"time"

	atypes "github.com/algorand/go-algorand-sdk/types"
	log "github.com/sirupsen/logrus"

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/types"
)

// DummyIndexerDb is a mock implementation of IndexerDb
func DummyIndexerDb() IndexerDb {
	return &dummyIndexerDb{}
}

type dummyIndexerDb struct {
	log *log.Logger
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

// AlreadyImported is part of idb.IndexerDB
func (db *dummyIndexerDb) AlreadyImported(path string) (imported bool, err error) {
	return false, nil
}

// MarkImported is part of idb.IndexerDB
func (db *dummyIndexerDb) MarkImported(path string) (err error) {
	return nil
}

// LoadGenesis is part of idb.IndexerDB
func (db *dummyIndexerDb) LoadGenesis(genesis types.Genesis) (err error) {
	return nil
}

// SetProto is part of idb.IndexerDB
func (db *dummyIndexerDb) SetProto(version string, proto types.ConsensusParams) (err error) {
	return nil
}

// GetProto is part of idb.IndexerDB
func (db *dummyIndexerDb) GetProto(version string) (proto types.ConsensusParams, err error) {
	err = nil
	return
}

// GetImportState is part of idb.IndexerDB
func (db *dummyIndexerDb) GetImportState() (is *ImportState, err error) {
	return nil, nil
}

// SetImportState is part of idb.IndexerDB
func (db *dummyIndexerDb) SetImportState(is ImportState) (err error) {
	return nil
}

// GetMaxRoundAccounted is part of idb.IndexerDB
func (db *dummyIndexerDb) GetMaxRoundAccounted() (round uint64, err error) {
	return 0, nil
}

// GetMaxRoundLoaded is part of idb.IndexerDB
func (db *dummyIndexerDb) GetMaxRoundLoaded() (round uint64, err error) {
	return 0, nil
}

// GetSpecialAccounts is part of idb.IndexerDb
func (db *dummyIndexerDb) GetSpecialAccounts() (SpecialAccounts, error) {
	return SpecialAccounts{}, nil
}

// GetDefaultFrozen is part of idb.IndexerDb
func (db *dummyIndexerDb) GetDefaultFrozen() (defaultFrozen map[uint64]bool, err error) {
	return make(map[uint64]bool), nil
}

// YieldTxns is part of idb.IndexerDB
func (db *dummyIndexerDb) YieldTxns(ctx context.Context, prevRound int64) <-chan TxnRow {
	return nil
}

// CommitRoundAccounting is part of idb.IndexerDB
func (db *dummyIndexerDb) CommitRoundAccounting(updates RoundUpdates, round uint64, blockPtr *types.Block) (err error) {
	return nil
}

// GetBlock is part of idb.IndexerDB
func (db *dummyIndexerDb) GetBlock(ctx context.Context, round uint64, options GetBlockOptions) (block types.Block, transactions []TxnRow, err error) {
	return types.Block{}, nil, nil
}

// Transactions is part of idb.IndexerDB
func (db *dummyIndexerDb) Transactions(ctx context.Context, tf TransactionFilter) (<-chan TxnRow, uint64) {
	return nil, 0
}

// GetAccounts is part of idb.IndexerDB
func (db *dummyIndexerDb) GetAccounts(ctx context.Context, opts AccountQueryOptions) (<-chan AccountRow, uint64) {
	return nil, 0
}

// Assets is part of idb.IndexerDB
func (db *dummyIndexerDb) Assets(ctx context.Context, filter AssetsQuery) (<-chan AssetRow, uint64) {
	return nil, 0
}

// AssetBalances is part of idb.IndexerDB
func (db *dummyIndexerDb) AssetBalances(ctx context.Context, abq AssetBalanceQuery) (<-chan AssetBalanceRow, uint64) {
	return nil, 0
}

// Applications is part of idb.IndexerDB
func (db *dummyIndexerDb) Applications(ctx context.Context, filter *models.SearchForApplicationsParams) (<-chan ApplicationRow, uint64) {
	return nil, 0
}

// Health is part of idb.IndexerDB
func (db *dummyIndexerDb) Health() (state Health, err error) {
	return Health{}, nil
}

// IndexerFactory is used to install an IndexerDb implementation.
type IndexerFactory interface {
	Name() string
	Build(arg string, opts *IndexerDbOptions, log *log.Logger) (IndexerDb, error)
}

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
	AssetCloseAmount   uint64          `codec:"aca,omitempty"`
	GlobalReverseDelta AppReverseDelta `codec:"agr,omitempty"`
	LocalReverseDelta  AppReverseDelta `codec:"alr,omitempty"`
}

// IndexerDb is the interface used to define alternative Indexer backends.
// TODO: sqlite3 impl
// TODO: cockroachdb impl
type IndexerDb interface {
	// The next few functions define the import interface, functions for loading data into the database. StartBlock() through Get/SetImportState().
	StartBlock() error
	AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error
	CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error

	AlreadyImported(path string) (imported bool, err error)
	MarkImported(path string) (err error)

	LoadGenesis(genesis types.Genesis) (err error)
	SetProto(version string, proto types.ConsensusParams) (err error)
	GetProto(version string) (proto types.ConsensusParams, err error)

	GetImportState() (is *ImportState, err error)
	SetImportState(ImportState) (err error)
	GetMaxRoundAccounted() (round uint64, err error)
	GetMaxRoundLoaded() (round uint64, err error)
	GetSpecialAccounts() (SpecialAccounts, error)
	GetDefaultFrozen() (defaultFrozen map[uint64]bool, err error)

	// YieldTxns returns a channel that produces the whole transaction stream after some round forward
	YieldTxns(ctx context.Context, prevRound int64) <-chan TxnRow

	CommitRoundAccounting(updates RoundUpdates, round uint64, blockPtr *types.Block) (err error)

	GetBlock(ctx context.Context, round uint64, options GetBlockOptions) (block types.Block, transactions []TxnRow, err error)

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

// TransactionFilter.AddressRole bitfield values
const (
	AddressRoleSender           = 0x01
	AddressRoleReceiver         = 0x02
	AddressRoleCloseRemainderTo = 0x04
	AddressRoleAssetSender      = 0x08
	AddressRoleAssetReceiver    = 0x10
	AddressRoleAssetCloseTo     = 0x20
	AddressRoleFreeze           = 0x40
)

// TransactionFilter.TypeEnum and also AddTransaction(,,txtypeenum,,,)
const (
	TypeEnumPay           = 1
	TypeEnumKeyreg        = 2
	TypeEnumAssetConfig   = 3
	TypeEnumAssetTransfer = 4
	TypeEnumAssetFreeze   = 5
	TypeEnumApplication   = 6
)

// TransactionFilter is a parameter object with all the transaction filter options.
type TransactionFilter struct {
	// Address filtering transactions for one Address will
	// return transactions newest-first proceding into the
	// past. Paging through such results can be achieved by
	// setting a MaxRound to get results before.
	Address []byte

	AddressRole uint64 // 0=Any, otherwise AddressRole* bitfields above

	MinRound   uint64
	MaxRound   uint64
	AfterTime  time.Time
	BeforeTime time.Time
	TypeEnum   int // ["","pay","keyreg","acfg","axfer","afrz"]
	Txid       string
	Round      *uint64 // nil for no filter
	Offset     *uint64 // nil for no filter
	OffsetLT   *uint64 // nil for no filter
	OffsetGT   *uint64 // nil for no filter
	SigType    string  // ["", "sig", "msig", "lsig"]
	NotePrefix []byte
	AlgosGT    uint64 // implictly filters on "pay" txns for Algos > this. This will be a slightly faster query than EffectiveAmountGt.
	AlgosLT    uint64
	RekeyTo    *bool // nil for no filter

	AssetID       uint64 // filter transactions relevant to an asset
	AssetAmountGT uint64
	AssetAmountLT uint64

	ApplicationID uint64 // filter transactions relevant to an application

	EffectiveAmountGt uint64 // Algo: Amount + CloseAmount > x
	EffectiveAmountLt uint64 // Algo: Amount + CloseAmount < x

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
	AlgosGreaterThan uint64
	// Filter on accounts with current balance less than x.
	AlgosLessThan uint64

	// HasAssetID, AssetGT, and AssetLT are implemented in Go code
	// after data has returned from Postgres and thus are slightly
	// less efficient. They will turn on IncludeAssetHoldings.
	HasAssetID uint64
	AssetGT    uint64
	AssetLT    uint64

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
	Params       types.AssetParams
	Error        error
	CreatedRound *uint64
	ClosedRound  *uint64
	Deleted      *bool
}

// AssetBalanceQuery is a parameter object with all of the asset balance filter options.
type AssetBalanceQuery struct {
	AssetID  uint64
	AmountGT uint64 // only rows > this
	AmountLT uint64 // only rows < this

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

type dummyFactory struct {
}

// Name is part of the IndexerFactory interface.
func (df dummyFactory) Name() string {
	return "dummy"
}

// Build is part of the IndexerFactory interface.
func (df dummyFactory) Build(arg string, opts *IndexerDbOptions, log *log.Logger) (IndexerDb, error) {
	return &dummyIndexerDb{log: log}, nil
}

// This layer of indirection allows for different db integrations to be compiled in or compiled out by `go build --tags ...`
var indexerFactories map[string]IndexerFactory

func init() {
	indexerFactories = make(map[string]IndexerFactory)
	RegisterFactory("dummy", &dummyFactory{})
}

// IndexerDbOptions are the options common to all indexer backends.
type IndexerDbOptions struct {
	ReadOnly bool
}

// RegisterFactory is used by IndexerDb implementations to register their implementations. This mechanism allows
// for loose coupling between the configuration and the implementation. It is extremely similar to the way sql.DB
// driver's are configured and used.
func RegisterFactory(name string, factory IndexerFactory) {
	indexerFactories[name] = factory
}

// IndexerDbByName is used to construct an IndexerDb object by name.
func IndexerDbByName(name, arg string, opts *IndexerDbOptions, log *log.Logger) (IndexerDb, error) {
	if val, ok := indexerFactories[name]; ok {
		return val.Build(arg, opts, log)
	}
	return nil, fmt.Errorf("no IndexerDb factory for %s", name)
}

// AssetUpdate is used by the accounting and IndexerDb implementations to share modifications in a block.
type AssetUpdate struct {
	AssetID       uint64
	DefaultFrozen bool
	Transfer      *AssetTransfer
	Close         *AssetClose
	Config        *AcfgUpdate
	Freeze        *FreezeUpdate
}

// AcfgUpdate is used by the accounting and IndexerDb implementations to share modifications in a block.
type AcfgUpdate struct {
	IsNew   bool
	Creator types.Address
	Params  types.AssetParams
}

// AssetTransfer is used by the accounting and IndexerDb implementations to share modifications in a block.
type AssetTransfer struct {
	Delta big.Int
}

// FreezeUpdate is used by the accounting and IndexerDb implementations to share modifications in a block.
type FreezeUpdate struct {
	Frozen bool
}

// AssetClose is used by the accounting and IndexerDb implementations to share modifications in a block.
type AssetClose struct {
	CloseTo types.Address
	Sender  types.Address
	Round   uint64
	Offset  uint64
}

// AlgoUpdate is used by the accounting and IndexerDb implementations to share modifications in a block.
// When the update does not include closing the account, the values are a delta applied to the account.
// If the update does include closing the account the rewards must be SET directly instead of applying a delta.
type AlgoUpdate struct {
	Balance int64
	Rewards int64
	// Closed changes the nature of the Rewards field. Balance and Rewards are normally deltas added to the
	// microalgos and totalRewards columns, but if an account has been Closed then Rewards becomes a new value
	// that replaces the old value (always zero by current reward logic)
	Closed bool
}

// AccountDataUpdate encodes an update or remove operation on an account_data json key.
type AccountDataUpdate struct {
	Delete bool        // false if update, true if delete
	Value  interface{} // value to write if `Delete` is false
}

// RoundUpdates is used by the accounting and IndexerDb implementations to share modifications in a block.
type RoundUpdates struct {
	AlgoUpdates  map[[32]byte]*AlgoUpdate
	AccountTypes map[[32]byte]string

	// AccountDataUpdates is explicitly a map so that we can
	// explicitly set values or have not set values. Instead of
	// using msgpack or JSON serialization of a struct, each field
	// is explicitly present or not. This makes it easier to set a
	// field to 0 and not have the serializer helpfully drop the
	// zero value. A 0 value may need to be sent to the database
	// to overlay onto a JSON struct there and replace a value
	// with a 0 value.
	AccountDataUpdates map[[32]byte]map[string]AccountDataUpdate

	// AssetUpdates is more complicated than AlgoUpdates because there
	// are no apply data values to work with in the event of a close.
	// The way we handle this is by breaking the round into sub-rounds,
	// which is represented by the overall slice.
	// Updates should be processed one subround at a time, the updates
	// within a subround can be processed in order for each addresses
	// updates, which have already been grouped together in the event
	// of multiple transactions between two accounts.
	// The next subround starts when an account close has been detected
	// Once a subround has been processed, move to the next subround and
	// apply the updates.
	// AssetConfig transactions also trigger the end of a subround.
	AssetUpdates  []map[[32]byte][]AssetUpdate
	AssetDestroys []uint64

	AppGlobalDeltas []AppDelta
	AppLocalDeltas  []AppDelta
}

// Clear is used to set a RoundUpdates object back to it's default values.
func (ru *RoundUpdates) Clear() {
	ru.AlgoUpdates = make(map[[32]byte]*AlgoUpdate)
	ru.AccountTypes = make(map[[32]byte]string)
	ru.AccountDataUpdates = make(map[[32]byte]map[string]AccountDataUpdate)
	ru.AssetUpdates = nil
	ru.AssetUpdates = append(ru.AssetUpdates, make(map[[32]byte][]AssetUpdate, 0))
	ru.AssetDestroys = nil
	ru.AppGlobalDeltas = nil
	ru.AppLocalDeltas = nil
}

// AppDelta used by the accounting and IndexerDb implementations to share modifications in a block.
type AppDelta struct {
	AppIndex     int64
	Round        uint64
	Intra        int
	Address      []byte
	AddrIndex    uint64 // 0=Sender, otherwise stxn.Txn.Accounts[i-1]
	Creator      []byte
	Delta        types.StateDelta
	OnCompletion atypes.OnCompletion

	// AppParams settings coppied from Txn, only for AppGlobalDeltas
	ApprovalProgram   []byte             `codec:"approv"`
	ClearStateProgram []byte             `codec:"clearp"`
	LocalStateSchema  atypes.StateSchema `codec:"lsch"`
	GlobalStateSchema atypes.StateSchema `codec:"gsch"`
}

// String is part of the Stringer interface.
func (ad AppDelta) String() string {
	parts := make([]string, 0, 10)
	if len(ad.Address) > 0 {
		parts = append(parts, b32np(ad.Address))
	}
	parts = append(parts, fmt.Sprintf("%d:%d app=%d", ad.Round, ad.Intra, ad.AppIndex))
	if len(ad.Creator) > 0 {
		parts = append(parts, "creator", b32np(ad.Creator))
	}
	ds := ""
	if ad.Delta != nil {
		ds = string(JSONOneLine(ad.Delta))
	}
	parts = append(parts, fmt.Sprintf("ai=%d oc=%v d=%s", ad.AddrIndex, ad.OnCompletion, ds))
	if len(ad.ApprovalProgram) > 0 {
		parts = append(parts, fmt.Sprintf("ap prog=%d bytes", len(ad.ApprovalProgram)))
	}
	if len(ad.ClearStateProgram) > 0 {
		parts = append(parts, fmt.Sprintf("cs prog=%d bytes", len(ad.ClearStateProgram)))
	}
	if ad.GlobalStateSchema.NumByteSlice != 0 || ad.GlobalStateSchema.NumUint != 0 {
		parts = append(parts, fmt.Sprintf("gss(b=%d, i=%d)", ad.GlobalStateSchema.NumByteSlice, ad.GlobalStateSchema.NumUint))
	}
	if ad.LocalStateSchema.NumByteSlice != 0 || ad.LocalStateSchema.NumUint != 0 {
		parts = append(parts, fmt.Sprintf("lss(b=%d, i=%d)", ad.LocalStateSchema.NumByteSlice, ad.LocalStateSchema.NumUint))
	}

	return strings.Join(parts, " ")
}

// StateDelta used by the accounting and IndexerDb implementations to share modifications in a block.
type StateDelta struct {
	Key   []byte
	Delta types.ValueDelta
}

// AppReverseDelta extra data attached to transactions relating to applications
type AppReverseDelta struct {
	Delta             []StateDelta        `codec:"d,omitempty"`
	OnCompletion      atypes.OnCompletion `codec:"oc,omitempty"`
	ApprovalProgram   []byte              `codec:"approv,omitempty"`
	ClearStateProgram []byte              `codec:"clearp,omitempty"`
	LocalStateSchema  atypes.StateSchema  `codec:"lsch,omitempty"`
	GlobalStateSchema atypes.StateSchema  `codec:"gsch,omitempty"`
}

// SetDelta adds delta values to the AppReverseDelta object.
func (ard *AppReverseDelta) SetDelta(key []byte, delta types.ValueDelta) {
	for i, sd := range ard.Delta {
		if bytes.Equal(key, sd.Key) {
			ard.Delta[i].Delta = delta
			return
		}
	}
	ard.Delta = append(ard.Delta, StateDelta{Key: key, Delta: delta})
}

// base32 no padding
func b32np(data []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(data)
}

// Health is the response object that IndexerDb objects need to return from the Health method.
type Health struct {
	Data        *map[string]interface{} `json:"data,omitempty"`
	Round       uint64                  `json:"round"`
	IsMigrating bool                    `json:"is-migrating"`
	DBAvailable bool                    `json:"db-available"`
	Error       string                  `json:"error"`
}

// SpecialAccounts are the accounts which have special accounting rules.
type SpecialAccounts struct {
	FeeSink     types.Address
	RewardsPool types.Address
}

// ImportState is some metadata kept around to help the import helper.
type ImportState struct {
	AccountRound int64 `codec:"account_round"`
}
