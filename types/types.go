package types

// copied from github.com/algorand/go-algorand/data/bookkeeping/block.go
import (
	"time"

	atypes "github.com/algorand/go-algorand-sdk/types"
)

type (
	Address = atypes.Address // [32]byte
	Digest  = atypes.Digest  // [32]byte

	Seed                     [32]byte
	Signature                [64]byte
	PublicKey                [32]byte
	OneTimeSignatureVerifier [32]byte
	VRFVerifier              [32]byte
	Round                    uint64
	ConsensusVersion         string
	MicroAlgos               uint64
	AssetIndex               uint64

	// BlockHash represents the hash of a block
	BlockHash Digest

	// A BlockHeader represents the metadata and commitments to the state of a Block.
	// The Algorand Ledger may be defined minimally as a cryptographically authenticated series of BlockHeader objects.
	BlockHeader struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		Round Round `codec:"rnd"`

		// The hash of the previous block
		Branch BlockHash `codec:"prev"`

		// Sortition seed
		Seed Seed `codec:"seed"`

		// TxnRoot authenticates the set of transactions appearing in the block.
		// More specifically, it's the root of a merkle tree whose leaves are the block's Txids.
		// Note that the TxnRoot does not authenticate the signatures on the transactions, only the transactions themselves.
		// Two blocks with the same transactions but with different signatures will have the same TxnRoot.
		TxnRoot Digest `codec:"txn"`

		// TimeStamp in seconds since epoch
		TimeStamp int64 `codec:"ts"`

		// Genesis ID to which this block belongs.
		GenesisID string `codec:"gen"`

		// Genesis hash to which this block belongs.
		GenesisHash Digest `codec:"gh"`

		// Rewards.
		//
		// When a block is applied, some amount of rewards are accrued to
		// every account with AccountData.Status=/=NotParticipating.  The
		// amount is (thisBlock.RewardsLevel-prevBlock.RewardsLevel) of
		// MicroAlgos for every whole config.Protocol.RewardUnit of MicroAlgos in
		// that account's AccountData.MicroAlgos.
		//
		// Rewards are not compounded (i.e., not added to AccountData.MicroAlgos)
		// until some other transaction is executed on that account.
		//
		// Not compounding rewards allows us to precisely know how many algos
		// of rewards will be distributed without having to examine every
		// account to determine if it should get one more algo of rewards
		// because compounding formed another whole config.Protocol.RewardUnit
		// of algos.
		RewardsState

		// Consensus protocol versioning.
		//
		// Each block is associated with a version of the consensus protocol,
		// stored under UpgradeState.CurrentProtocol.  The protocol version
		// for a block can be determined without having to first decode the
		// block and its CurrentProtocol field, and this field is present for
		// convenience and explicitness.  Block.Valid() checks that this field
		// correctly matches the expected protocol version.
		//
		// Each block is associated with at most one active upgrade proposal
		// (a new version of the protocol).  An upgrade proposal can be made
		// by a block proposer, as long as no other upgrade proposal is active.
		// The upgrade proposal lasts for many rounds (UpgradeVoteRounds), and
		// in each round, that round's block proposer votes to support (or not)
		// the proposed upgrade.
		//
		// If enough votes are collected, the proposal is approved, and will
		// definitely take effect.  The proposal lingers for some number of
		// rounds to give clients a chance to notify users about an approved
		// upgrade, if the client doesn't support it, so the user has a chance
		// to download updated client software.
		//
		// Block proposers influence this upgrade machinery through two fields
		// in UpgradeVote: UpgradePropose, which proposes an upgrade to a new
		// protocol, and UpgradeApprove, which signals approval of the current
		// proposal.
		//
		// Once a block proposer determines its UpgradeVote, then UpdateState
		// is updated deterministically based on the previous UpdateState and
		// the new block's UpgradeVote.
		UpgradeState
		UpgradeVote

		// TxnCounter counts the number of transactions committed in the
		// ledger, from the time at which support for this feature was
		// introduced.
		//
		// Specifically, TxnCounter is the number of the next transaction
		// that will be committed after this block.  It is 0 when no
		// transactions have ever been committed (since TxnCounter
		// started being supported).
		TxnCounter uint64 `codec:"tc"`
	}
	// RewardsState represents the global parameters controlling the rate
	// at which accounts accrue rewards.
	RewardsState struct {
		// The FeeSink accepts transaction fees. It can only spend to
		// the incentive pool.
		FeeSink Address `codec:"fees"`

		// The RewardsPool accepts periodic injections from the
		// FeeSink and continually redistributes them to adresses as
		// rewards.
		RewardsPool Address `codec:"rwd"`

		// RewardsLevel specifies how many rewards, in MicroAlgos,
		// have been distributed to each config.Protocol.RewardUnit
		// of MicroAlgos since genesis.
		RewardsLevel uint64 `codec:"earn"`

		// The number of new MicroAlgos added to the participation stake from rewards at the next round.
		RewardsRate uint64 `codec:"rate"`

		// The number of leftover MicroAlgos after the distribution of RewardsRate/rewardUnits
		// MicroAlgos for every reward unit in the next round.
		RewardsResidue uint64 `codec:"frac"`

		// The round at which the RewardsRate will be recalculated.
		RewardsRecalculationRound Round `codec:"rwcalr"`
	}

	// UpgradeVote represents the vote of the block proposer with
	// respect to protocol upgrades.
	UpgradeVote struct {
		// UpgradePropose indicates a proposed upgrade
		UpgradePropose ConsensusVersion `codec:"upgradeprop"`

		// UpgradeDelay indicates the time between acceptance and execution
		UpgradeDelay Round `codec:"upgradedelay"`

		// UpgradeApprove indicates a yes vote for the current proposal
		UpgradeApprove bool `codec:"upgradeyes"`
	}

	// UpgradeState tracks the protocol upgrade state machine.  It is,
	// strictly speaking, computable from the history of all UpgradeVotes
	// but we keep it in the block for explicitness and convenience
	// (instead of materializing it separately, like balances).
	UpgradeState struct {
		CurrentProtocol        ConsensusVersion `codec:"proto"`
		NextProtocol           ConsensusVersion `codec:"nextproto"`
		NextProtocolApprovals  uint64           `codec:"nextyes"`
		NextProtocolVoteBefore Round            `codec:"nextbefore"`
		NextProtocolSwitchOn   Round            `codec:"nextswitch"`
	}

	// A Block contains the Payset and metadata corresponding to a given Round.
	Block struct {
		BlockHeader
		Payset Payset `codec:"txns"`
	}

	Payset []SignedTxnInBlock

	// SignedTxnInBlock is how a signed transaction is encoded in a block.
	SignedTxnInBlock struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		SignedTxnWithAD

		HasGenesisID   bool `codec:"hgi"`
		HasGenesisHash bool `codec:"hgh"`
	}

	// SignedTxnWithAD is a (decoded) SignedTxn with associated ApplyData
	SignedTxnWithAD struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		atypes.SignedTxn
		ApplyData
	}

	ApplyData struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		// Closing amount for transaction.
		ClosingAmount MicroAlgos `codec:"ca"`

		// Rewards applied to the Sender, Receiver, and CloseRemainderTo accounts.
		SenderRewards   MicroAlgos `codec:"rs"`
		ReceiverRewards MicroAlgos `codec:"rr"`
		CloseRewards    MicroAlgos `codec:"rc"`
	}

	Transaction      = atypes.Transaction
	AssetParams      = atypes.AssetParams
	EncodedBlockCert struct {
		Block       Block       `codec:"block"`
		Certificate Certificate `codec:"cert"`
	}

	Certificate struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		Round    Round         `codec:"rnd"`
		Period   uint64        `codec:"per"`
		Step     uint64        `codec:"step"`
		Proposal proposalValue `codec:"prop"`

		Votes             []voteAuthenticator             `codec:"vote"`
		EquivocationVotes []equivocationVoteAuthenticator `codec:"eqv"`
	}

	proposalValue struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		OriginalPeriod   uint64  `codec:"oper"`
		OriginalProposer Address `codec:"oprop"`
		BlockDigest      Digest  `codec:"dig"`    // = proposal.Block.Digest()
		EncodingDigest   Digest  `codec:"encdig"` // = crypto.HashObj(proposal)
	}

	voteAuthenticator struct {
		Sender Address                   `codec:"snd"`
		Cred   UnauthenticatedCredential `codec:"cred"`
		Sig    OneTimeSignature          `codec:"sig,omitempty,omitemptycheckstruct"`
	}

	// An UnauthenticatedCredential is a Credential which has not yet been
	// authenticated.
	UnauthenticatedCredential struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`
		Proof   VrfProof `codec:"pf"`
	}
	VrfProof [80]byte

	OneTimeSignature struct {
		// Sig is a signature of msg under the key PK.
		Sig Signature `codec:"s"`
		PK  PublicKey `codec:"p"`

		// Old-style signature that does not use proper domain separation.
		// PKSigOld is unused; however, unfortunately we forgot to mark it
		// `codec:omitempty` and so it appears (with zero value) in certs.
		// This means we can't delete the field without breaking catchup.
		PKSigOld Signature `codec:"ps"`

		// Used to verify a new-style two-level ephemeral signature.
		// PK1Sig is a signature of OneTimeSignatureSubkeyOffsetID(PK, Batch, Offset) under the key PK2.
		// PK2Sig is a signature of OneTimeSignatureSubkeyBatchID(PK2, Batch) under the master key (OneTimeSignatureVerifier).
		PK2    PublicKey `codec:"p2"`
		PK1Sig Signature `codec:"p1s"`
		PK2Sig Signature `codec:"p2s"`
	}

	equivocationVoteAuthenticator struct {
		Sender    Address                   `codec:"snd"`
		Cred      UnauthenticatedCredential `codec:"cred"`
		Sigs      [2]OneTimeSignature       `codec:"sig,omitempty,omitemptycheckstruct"`
		Proposals [2]proposalValue          `codec:"props"`
	}

	// from github.com/algorand/go-algorand/data/bookkeeping/genesis.go

	// A Genesis object defines an Algorand "universe" -- a set of nodes that can
	// talk to each other, agree on the ledger contents, etc.  This is defined
	// by the initial account states (GenesisAllocation), the initial
	// consensus protocol (GenesisProto), and the schema of the ledger.
	Genesis struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		// The SchemaID allows nodes to store data specific to a particular
		// universe (in case of upgrades at development or testing time),
		// and as an optimization to quickly check if two nodes are in
		// the same universe.
		SchemaID string `codec:"id"`

		// Network identifies the unique algorand network for which the ledger
		// is valid.
		// Note the Network name should not include a '-', as we generate the
		// GenesisID from "<Network>-<SchemaID>"; the '-' makes it easy
		// to distinguish between the network and schema.
		Network string `codec:"network"`

		// Proto is the consensus protocol in use at the genesis block.
		Proto ConsensusVersion `codec:"proto"`

		// Allocation determines the initial accounts and their state.
		Allocation []GenesisAllocation `codec:"alloc"`

		// RewardsPool is the address of the rewards pool.
		RewardsPool string `codec:"rwd"`

		// FeeSink is the address of the fee sink.
		FeeSink string `codec:"fees"`

		// Timestamp for the genesis block
		Timestamp int64 `codec:"timestamp"`

		// Arbitrary genesis comment string - will be excluded from file if empty
		Comment string `codec:"comment"`
	}

	// A GenesisAllocation object represents an allocation of algos to
	// an address in the genesis block.  Address is the checksummed
	// short address.  Comment is a note about what this address is
	// representing, and is purely informational.  State is the initial
	// account state.
	GenesisAllocation struct {
		Address string      `codec:"addr"`
		Comment string      `codec:"comment"`
		State   AccountData `codec:"state"`
	}

	// from github.com/algorand/go-algorand/data/basics/userBalance.go

	// AccountData contains the data associated with a given address.
	//
	// This includes the account balance, delegation keys, delegation status, and a custom note.
	AccountData struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		Status     byte       `codec:"onl"`
		MicroAlgos MicroAlgos `codec:"algo"`

		// RewardsBase is used to implement rewards.
		// This is not meaningful for accounts with Status=NotParticipating.
		//
		// Every block assigns some amount of rewards (algos) to every
		// participating account.  The amount is the product of how much
		// block.RewardsLevel increased from the previous block and
		// how many whole config.Protocol.RewardUnit algos this
		// account holds.
		//
		// For performance reasons, we do not want to walk over every
		// account to apply these rewards to AccountData.MicroAlgos.  Instead,
		// we defer applying the rewards until some other transaction
		// touches that participating account, and at that point, apply all
		// of the rewards to the account's AccountData.MicroAlgos.
		//
		// For correctness, we need to be able to determine how many
		// total algos are present in the system, including deferred
		// rewards (deferred in the sense that they have not been
		// reflected in the account's AccountData.MicroAlgos, as described
		// above).  To compute this total efficiently, we avoid
		// compounding rewards (i.e., no rewards on rewards) until
		// they are applied to AccountData.MicroAlgos.
		//
		// Mechanically, RewardsBase stores the block.RewardsLevel
		// whose rewards are already reflected in AccountData.MicroAlgos.
		// If the account is Status=Offline or Status=Online, its
		// effective balance (if a transaction were to be issued
		// against this account) may be higher, as computed by
		// AccountData.Money().  That function calls
		// AccountData.WithUpdatedRewards() to apply the deferred
		// rewards to AccountData.MicroAlgos.
		RewardsBase uint64 `codec:"ebase"`

		// RewardedMicroAlgos is used to track how many algos were given
		// to this account since the account was first created.
		//
		// This field is updated along with RewardBase; note that
		// it won't answer the question "how many algos did I make in
		// the past week".
		RewardedMicroAlgos MicroAlgos `codec:"ern"`

		VoteID      OneTimeSignatureVerifier `codec:"vote"`
		SelectionID VRFVerifier              `codec:"sel"`

		VoteFirstValid  Round  `codec:"voteFst"`
		VoteLastValid   Round  `codec:"voteLst"`
		VoteKeyDilution uint64 `codec:"voteKD"`

		// If this account created an asset, AssetParams stores
		// the parameters defining that asset.  The params are indexed
		// by the Index of the AssetID; the Creator is this account's address.
		//
		// An account with any asset in AssetParams cannot be
		// closed, until the asset is destroyed.  An asset can
		// be destroyed if this account holds AssetParams.Total units
		// of that asset (in the Assets array below).
		//
		// NOTE: do not modify this value in-place in existing AccountData
		// structs; allocate a copy and modify that instead.  AccountData
		// is expected to have copy-by-value semantics.
		AssetParams map[AssetIndex]AssetParams `codec:"apar"`

		// Assets is the set of assets that can be held by this
		// account.  Assets (i.e., slots in this map) are explicitly
		// added and removed from an account by special transactions.
		// The map is keyed by the AssetID, which is the address of
		// the account that created the asset plus a unique counter
		// to distinguish re-created assets.
		//
		// Each asset bumps the required MinBalance in this account.
		//
		// An account that creates an asset must have its own asset
		// in the Assets map until that asset is destroyed.
		//
		// NOTE: do not modify this value in-place in existing AccountData
		// structs; allocate a copy and modify that instead.  AccountData
		// is expected to have copy-by-value semantics.
		Assets map[AssetIndex]AssetHolding `codec:"asset"`
	}

	// AssetHolding describes an asset held by an account.
	AssetHolding struct {
		Amount uint64 `codec:"a"`
		Frozen bool   `codec:"f"`
	}
)

// from github.com/algorand/go-algorand/config/config.go
// ConsensusParams specifies settings that might vary based on the
// particular version of the consensus protocol.
type ConsensusParams struct {
	// Consensus protocol upgrades.  Votes for upgrades are collected for
	// UpgradeVoteRounds.  If the number of positive votes is over
	// UpgradeThreshold, the proposal is accepted.
	//
	// UpgradeVoteRounds needs to be long enough to collect an
	// accurate sample of participants, and UpgradeThreshold needs
	// to be high enough to ensure that there are sufficient participants
	// after the upgrade.
	//
	// A consensus protocol upgrade may specify the delay between its
	// acceptance and its execution.  This gives clients time to notify
	// users.  This delay is specified by the upgrade proposer and must
	// be between MinUpgradeWaitRounds and MaxUpgradeWaitRounds (inclusive)
	// in the old protocol's parameters.  Note that these parameters refer
	// to the representation of the delay in a block rather than the actual
	// delay: if the specified delay is zero, it is equivalent to
	// DefaultUpgradeWaitRounds.
	//
	// The maximum length of a consensus version string is
	// MaxVersionStringLen.
	UpgradeVoteRounds        uint64
	UpgradeThreshold         uint64
	DefaultUpgradeWaitRounds uint64
	MinUpgradeWaitRounds     uint64
	MaxUpgradeWaitRounds     uint64
	MaxVersionStringLen      int

	// MaxTxnBytesPerBlock determines the maximum number of bytes
	// that transactions can take up in a block.  Specifically,
	// the sum of the lengths of encodings of each transaction
	// in a block must not exceed MaxTxnBytesPerBlock.
	MaxTxnBytesPerBlock int

	// MaxTxnBytesPerBlock is the maximum size of a transaction's Note field.
	MaxTxnNoteBytes int

	// MaxTxnLife is how long a transaction can be live for:
	// the maximum difference between LastValid and FirstValid.
	//
	// Note that in a protocol upgrade, the ledger must first be upgraded
	// to hold more past blocks for this value to be raised.
	MaxTxnLife uint64

	// ApprovedUpgrades describes the upgrade proposals that this protocol
	// implementation will vote for, along with their delay value
	// (in rounds).  A delay value of zero is the same as a delay of
	// DefaultUpgradeWaitRounds.
	ApprovedUpgrades map[ConsensusVersion]uint64

	// SupportGenesisHash indicates support for the GenesisHash
	// fields in transactions (and requires them in blocks).
	SupportGenesisHash bool

	// RequireGenesisHash indicates that GenesisHash must be present
	// in every transaction.
	RequireGenesisHash bool

	// DefaultKeyDilution specifies the granularity of top-level ephemeral
	// keys. KeyDilution is the number of second-level keys in each batch,
	// signed by a top-level "batch" key.  The default value can be
	// overriden in the account state.
	DefaultKeyDilution uint64

	// MinBalance specifies the minimum balance that can appear in
	// an account.  To spend money below MinBalance requires issuing
	// an account-closing transaction, which transfers all of the
	// money from the account, and deletes the account state.
	MinBalance uint64

	// MinTxnFee specifies the minimum fee allowed on a transaction.
	// A minimum fee is necessary to prevent DoS. In some sense this is
	// a way of making the spender subsidize the cost of storing this transaction.
	MinTxnFee uint64

	// RewardUnit specifies the number of MicroAlgos corresponding to one reward
	// unit.
	//
	// Rewards are received by whole reward units.  Fractions of
	// RewardUnits do not receive rewards.
	RewardUnit uint64

	// RewardsRateRefreshInterval is the number of rounds after which the
	// rewards level is recomputed for the next RewardsRateRefreshInterval rounds.
	RewardsRateRefreshInterval uint64

	// seed-related parameters
	SeedLookback        uint64 // how many blocks back we use seeds from in sortition. delta_s in the spec
	SeedRefreshInterval uint64 // how often an old block hash is mixed into the seed. delta_r in the spec

	// ledger retention policy
	MaxBalLookback uint64 // (current round - MaxBalLookback) is the oldest round the ledger must answer balance queries for

	// sortition threshold factors
	NumProposers           uint64
	SoftCommitteeSize      uint64
	SoftCommitteeThreshold uint64
	CertCommitteeSize      uint64
	CertCommitteeThreshold uint64
	NextCommitteeSize      uint64 // for any non-FPR votes >= deadline step, committee sizes and thresholds are constant
	NextCommitteeThreshold uint64
	LateCommitteeSize      uint64
	LateCommitteeThreshold uint64
	RedoCommitteeSize      uint64
	RedoCommitteeThreshold uint64
	DownCommitteeSize      uint64
	DownCommitteeThreshold uint64

	FastRecoveryLambda    time.Duration // time between fast recovery attempts
	FastPartitionRecovery bool          // set when fast partition recovery is enabled

	// commit to payset using a hash of entire payset,
	// instead of txid merkle tree
	PaysetCommitFlat bool

	MaxTimestampIncrement int64 // maximum time between timestamps on successive blocks

	// support for the efficient encoding in SignedTxnInBlock
	SupportSignedTxnInBlock bool

	// force the FeeSink address to be non-participating in the genesis balances.
	ForceNonParticipatingFeeSink bool

	// support for ApplyData in SignedTxnInBlock
	ApplyData bool

	// track reward distributions in ApplyData
	RewardsInApplyData bool

	// domain-separated credentials
	CredentialDomainSeparationEnabled bool

	// support for transactions that mark an account non-participating
	SupportBecomeNonParticipatingTransactions bool

	// fix the rewards calculation by avoiding subtracting too much from the rewards pool
	PendingResidueRewards bool

	// asset support
	Asset bool

	// max number of assets per account
	MaxAssetsPerAccount int

	// max length of asset name
	MaxAssetNameBytes int

	// max length of asset unit name
	MaxAssetUnitNameBytes int

	// max length of asset url
	MaxAssetURLBytes int

	// support sequential transaction counter TxnCounter
	TxnCounter bool

	// transaction groups
	SupportTxGroups bool

	// max group size
	MaxTxGroupSize int

	// support for transaction leases
	SupportTransactionLeases bool

	// 0 for no support, otherwise highest version supported
	LogicSigVersion uint64

	// len(LogicSig.Logic) + len(LogicSig.Args[*]) must be less than this
	LogicSigMaxSize uint64

	// sum of estimated op cost must be less than this
	LogicSigMaxCost uint64

	// max decimal precision for assets
	MaxAssetDecimals uint32

	// whether to use the old buggy Credential.lowestOutput function
	// TODO(upgrade): Please remove as soon as the upgrade goes through
	UseBuggyProposalLowestOutput bool
}
