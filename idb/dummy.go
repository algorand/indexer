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

	models "github.com/algorand/indexer/api/generated/v2"
	"github.com/algorand/indexer/types"
)

func DummyIndexerDb() IndexerDb {
	return &dummyIndexerDb{}
}

type dummyIndexerDb struct {
}

func (db *dummyIndexerDb) StartBlock() (err error) {
	fmt.Printf("StartBlock\n")
	return nil
}
func (db *dummyIndexerDb) AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error {
	fmt.Printf("\ttxn %d %d %d %d\n", round, intra, txtypeenum, assetid)
	return nil
}
func (db *dummyIndexerDb) CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error {
	fmt.Printf("CommitBlock %d %d %d header bytes\n", round, timestamp, len(headerbytes))
	return nil
}

func (db *dummyIndexerDb) AlreadyImported(path string) (imported bool, err error) {
	return false, nil
}
func (db *dummyIndexerDb) MarkImported(path string) (err error) {
	return nil
}

func (db *dummyIndexerDb) LoadGenesis(genesis types.Genesis) (err error) {
	return nil
}

func (db *dummyIndexerDb) SetProto(version string, proto types.ConsensusParams) (err error) {
	return nil
}

func (db *dummyIndexerDb) GetProto(version string) (proto types.ConsensusParams, err error) {
	err = nil
	return
}

func (db *dummyIndexerDb) GetMetastate(key string) (jsonStrValue string, err error) {
	return "", nil
}

func (db *dummyIndexerDb) SetMetastate(key, jsonStrValue string) (err error) {
	return nil
}

func (db *dummyIndexerDb) GetMaxRound() (round uint64, err error) {
	return 0, nil
}

func (db *dummyIndexerDb) YieldTxns(ctx context.Context, prevRound int64) <-chan TxnRow {
	return nil
}

func (db *dummyIndexerDb) CommitRoundAccounting(updates RoundUpdates, round, rewardsBase uint64) (err error) {
	return nil
}

func (db *dummyIndexerDb) GetBlock(round uint64) (block types.Block, err error) {
	err = nil
	return
}
func (db *dummyIndexerDb) Transactions(ctx context.Context, tf TransactionFilter) <-chan TxnRow {
	return nil
}

func (db *dummyIndexerDb) GetAccounts(ctx context.Context, opts AccountQueryOptions) <-chan AccountRow {
	return nil
}

func (db *dummyIndexerDb) Assets(ctx context.Context, filter AssetsQuery) <-chan AssetRow {
	return nil
}

func (db *dummyIndexerDb) AssetBalances(ctx context.Context, abq AssetBalanceQuery) <-chan AssetBalanceRow {
	return nil
}

func (db *dummyIndexerDb) Applications(ctx context.Context, filter *models.SearchForApplicationsParams) <-chan ApplicationRow {
	return nil
}

type IndexerFactory interface {
	Name() string
	Build(arg string, opts *IndexerDbOptions) (IndexerDb, error)
}

type TxnRow struct {
	Round     uint64
	RoundTime time.Time
	Intra     int
	TxnBytes  []byte
	AssetId   uint64
	Extra     TxnExtra
	Error     error
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

type TxnExtra struct {
	AssetCloseAmount   uint64          `codec:"aca,omitempty"`
	GlobalReverseDelta AppReverseDelta `codec:"agr,omitempty"`
	LocalReverseDelta  AppReverseDelta `codec:"alr,omitempty"`
}

// TODO: sqlite3 impl
// TODO: cockroachdb impl
type IndexerDb interface {
	// The next few functions define the import interface, functions for loading data into the database. StartBlock() through Get/SetMetastate().

	StartBlock() error
	AddTransaction(round uint64, intra int, txtypeenum int, assetid uint64, txn types.SignedTxnWithAD, participation [][]byte) error
	CommitBlock(round uint64, timestamp int64, rewardslevel uint64, headerbytes []byte) error

	AlreadyImported(path string) (imported bool, err error)
	MarkImported(path string) (err error)

	LoadGenesis(genesis types.Genesis) (err error)
	SetProto(version string, proto types.ConsensusParams) (err error)
	GetProto(version string) (proto types.ConsensusParams, err error)

	GetMetastate(key string) (jsonStrValue string, err error)
	SetMetastate(key, jsonStrValue string) (err error)
	GetMaxRound() (round uint64, err error)

	// YieldTxns returns a channel that produces the whole transaction stream after some round forward
	YieldTxns(ctx context.Context, prevRound int64) <-chan TxnRow

	CommitRoundAccounting(updates RoundUpdates, round, rewardsBase uint64) (err error)

	GetBlock(round uint64) (block types.Block, err error)

	Transactions(ctx context.Context, tf TransactionFilter) <-chan TxnRow
	GetAccounts(ctx context.Context, opts AccountQueryOptions) <-chan AccountRow
	Assets(ctx context.Context, filter AssetsQuery) <-chan AssetRow
	AssetBalances(ctx context.Context, abq AssetBalanceQuery) <-chan AssetBalanceRow
	Applications(ctx context.Context, filter *models.SearchForApplicationsParams) <-chan ApplicationRow
}

func GetAccount(idb IndexerDb, addr []byte) (account models.Account, err error) {
	for ar := range idb.GetAccounts(context.Background(), AccountQueryOptions{EqualToAddress: addr}) {
		return ar.Account, ar.Error
	}
	return models.Account{}, nil
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

	AssetId       uint64 // filter transactions relevant to an asset
	AssetAmountGT uint64
	AssetAmountLT uint64

	ApplicationId uint64 // filter transactions relevant to an application

	EffectiveAmountGt uint64 // Algo: Amount + CloseAmount > x
	EffectiveAmountLt uint64 // Algo: Amount + CloseAmount < x

	// pointer to last returned object of previous query
	NextToken string

	Limit uint64
}

type AccountQueryOptions struct {
	GreaterThanAddress []byte // for paging results
	EqualToAddress     []byte // return exactly this one account

	// return any accounts with this auth addr
	EqualToAuthAddr []byte

	// Filter on accounts with current balance greater than x
	AlgosGreaterThan uint64
	// Filter on accounts with current balance less than x.
	AlgosLessThan uint64

	// HasAssetId, AssetGT, and AssetLT are implemented in Go code
	// after data has returned from Postgres and thus are slightly
	// less efficient. They will turn on IncludeAssetHoldings.
	HasAssetId uint64
	AssetGT    uint64
	AssetLT    uint64

	HasAppId uint64

	IncludeAssetHoldings bool
	IncludeAssetParams   bool

	Limit uint64
}

type AccountRow struct {
	Account models.Account
	Error   error
}

type AssetsQuery struct {
	AssetId            uint64
	AssetIdGreaterThan uint64

	Creator []byte

	// Name is a case insensitive substring comparison of the asset name
	Name string
	// Unit is a case insensitive substring comparison of the asset unit
	Unit string
	// Query checks for fuzzy match against either asset name or unit name
	// (assetname ILIKE '%?%' OR unitname ILIKE '%?%')
	Query string

	Limit uint64
}

type AssetRow struct {
	AssetId uint64
	Creator []byte
	Params  types.AssetParams
	Error   error
}

type AssetBalanceQuery struct {
	AssetId  uint64
	AmountGT uint64 // only rows > this
	AmountLT uint64 // only rows < this

	Limit uint64 // max rows to return

	// PrevAddress for paging, the last item from the previous
	// query (items returned in address order)
	PrevAddress []byte
}

type AssetBalanceRow struct {
	Address []byte
	AssetId uint64
	Amount  uint64
	Frozen  bool
	Error   error
}

type ApplicationRow struct {
	Application models.Application
	Error       error
}

type dummyFactory struct {
}

func (df dummyFactory) Name() string {
	return "dummy"
}
func (df dummyFactory) Build(arg string, opts *IndexerDbOptions) (IndexerDb, error) {
	return &dummyIndexerDb{}, nil
}

// This layer of indirection allows for different db integrations to be compiled in or compiled out by `go build --tags ...`
var indexerFactories []IndexerFactory

func init() {
	indexerFactories = append(indexerFactories, &dummyFactory{})
}

type IndexerDbOptions struct {
	ReadOnly bool
}

func IndexerDbByName(factoryname, arg string, opts *IndexerDbOptions) (IndexerDb, error) {
	for _, ifac := range indexerFactories {
		if ifac.Name() == factoryname {
			return ifac.Build(arg, opts)
		}
	}
	return nil, fmt.Errorf("no IndexerDb factory for %s", factoryname)
}

type AccountDataUpdate struct {
	Addr            types.Address
	Status          int // {Offline:0, Online:1, NotParticipating: 2}
	VoteID          [32]byte
	SelectionID     [32]byte
	VoteFirstValid  uint64
	VoteLastValid   uint64
	VoteKeyDilution uint64
}

type AcfgUpdate struct {
	AssetId uint64
	IsNew   bool
	Creator types.Address
	Params  types.AssetParams
}

type AssetUpdate struct {
	AssetId       uint64
	Delta         big.Int
	DefaultFrozen bool
}

type FreezeUpdate struct {
	Addr    types.Address
	AssetId uint64
	Frozen  bool
}

type AssetClose struct {
	CloseTo       types.Address
	AssetId       uint64
	Sender        types.Address
	Round         uint64
	Offset        uint64
	DefaultFrozen bool
}

type TxnAssetUpdate struct {
	Round   uint64
	Offset  int
	AssetId uint64
}

type RoundUpdates struct {
	AlgoUpdates  map[[32]byte]int64
	AccountTypes map[[32]byte]string

	// AccountDataUpdates is explicitly a map so that we can
	// explicitly set values or have not set values. Instead of
	// using msgpack or JSON serialization of a struct, each field
	// is explicitly present or not. This makes it easier to set a
	// field to 0 and not have the serializer helpfully drop the
	// zero value. A 0 value may need to be sent to the database
	// to overlay onto a JSON struct there and replace a value
	// with a 0 value.
	AccountDataUpdates map[[32]byte]map[string]interface{}

	AcfgUpdates     []AcfgUpdate
	TxnAssetUpdates []TxnAssetUpdate
	AssetUpdates    map[[32]byte][]AssetUpdate
	FreezeUpdates   []FreezeUpdate
	AssetCloses     []AssetClose
	AssetDestroys   []uint64

	AppGlobalDeltas []AppDelta
	AppLocalDeltas  []AppDelta
}

func (ru *RoundUpdates) Clear() {
	ru.AlgoUpdates = nil
	ru.AccountTypes = nil
	ru.AccountDataUpdates = nil
	ru.AcfgUpdates = nil
	ru.TxnAssetUpdates = nil
	ru.AssetUpdates = nil
	ru.FreezeUpdates = nil
	ru.AssetCloses = nil
	ru.AssetDestroys = nil
	ru.AppGlobalDeltas = nil
	ru.AppLocalDeltas = nil
}

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
		ds = string(JsonOneLine(ad.Delta))
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

type StateDelta struct {
	Key   []byte
	Delta types.ValueDelta
}

// extra data attached to transactions
type AppReverseDelta struct {
	Delta             []StateDelta        `codec:"d,omitempty"`
	OnCompletion      atypes.OnCompletion `codec:"oc,omitempty"`
	ApprovalProgram   []byte              `codec:"approv,omitempty"`
	ClearStateProgram []byte              `codec:"clearp,omitempty"`
	LocalStateSchema  atypes.StateSchema  `codec:"lsch,omitempty"`
	GlobalStateSchema atypes.StateSchema  `codec:"gsch,omitempty"`
}

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
