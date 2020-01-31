// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of the Algorand Indexer
//
// Algorand Indexer is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// Algorand Indexer is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with Algorand Indexer.  If not, see <https://www.gnu.org/licenses/>.

package types

// copied from github.com/algorand/go-algorand/data/bookkeeping/block.go

type (
	Address                  [32]byte
	Digest                   [32]byte
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

		SignedTxn
		ApplyData
	}

	MultisigSig struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		Version   uint8            `codec:"v"`
		Threshold uint8            `codec:"thr"`
		Subsigs   []MultisigSubsig `codec:"subsig"`
	}
	MultisigSubsig struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		Key PublicKey `codec:"pk"` // all public keys that are possible signers for this address
		Sig Signature `codec:"s"`  // may be either empty or a signature
	}
	LogicSig struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		// Logic signed by Sig or Msig, OR hashed to be the Address of an account.
		Logic []byte `codec:"l"`

		Sig  Signature   `codec:"sig"`
		Msig MultisigSig `codec:"msig"`

		// Args are not signed, but checked by Logic
		Args [][]byte `codec:"arg"`
	}
	SignedTxn struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		Sig  Signature   `codec:"sig"`
		Msig MultisigSig `codec:"msig"`
		Lsig LogicSig    `codec:"lsig"`
		Txn  Transaction `codec:"txn"`
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

	Transaction struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		// Type of transaction
		Type string `codec:"type"`

		// Common fields for all types of transactions
		TxnHeader

		// Fields for different types of transactions
		KeyregTxnFields
		PaymentTxnFields
		AssetConfigTxnFields
		AssetTransferTxnFields
		AssetFreezeTxnFields
	}

	TxnHeader struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		Sender      Address    `codec:"snd"`
		Fee         MicroAlgos `codec:"fee"`
		FirstValid  Round      `codec:"fv"`
		LastValid   Round      `codec:"lv"`
		Note        []byte     `codec:"note"` // Uniqueness or app-level data about txn
		GenesisID   string     `codec:"gen"`
		GenesisHash Digest     `codec:"gh"`

		// Group specifies that this transaction is part of a
		// transaction group (and, if so, specifies the hash
		// of a TxGroup).
		Group Digest `codec:"grp"`

		// Lease enforces mutual exclusion of transactions.  If this field is
		// nonzero, then once the transaction is confirmed, it acquires the
		// lease identified by the (Sender, Lease) pair of the transaction until
		// the LastValid round passes.  While this transaction possesses the
		// lease, no other transaction specifying this lease can be confirmed.
		Lease [32]byte `codec:"lx"`
	}

	KeyregTxnFields struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		VotePK           OneTimeSignatureVerifier `codec:"votekey"`
		SelectionPK      VRFVerifier              `codec:"selkey"`
		VoteFirst        Round                    `codec:"votefst"`
		VoteLast         Round                    `codec:"votelst"`
		VoteKeyDilution  uint64                   `codec:"votekd"`
		Nonparticipation bool                     `codec:"nonpart"`
	}

	PaymentTxnFields struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		Receiver Address    `codec:"rcv"`
		Amount   MicroAlgos `codec:"amt"`

		// When CloseRemainderTo is set, it indicates that the
		// transaction is requesting that the account should be
		// closed, and all remaining funds be transferred to this
		// address.
		CloseRemainderTo Address `codec:"close"`
	}

	AssetParams struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		// Total specifies the total number of units of this asset
		// created.
		Total uint64 `codec:"t"`

		// Decimals specifies the number of digits to display after the decimal
		// place when displaying this asset. A value of 0 represents an asset
		// that is not divisible, a value of 1 represents an asset divisible
		// into tenths, and so on. This value must be between 0 and 19
		// (inclusive).
		Decimals uint32 `codec:"dc"`

		// DefaultFrozen specifies whether slots for this asset
		// in user accounts are frozen by default or not.
		DefaultFrozen bool `codec:"df"`

		// UnitName specifies a hint for the name of a unit of
		// this asset.
		UnitName string `codec:"un"`

		// AssetName specifies a hint for the name of the asset.
		AssetName string `codec:"an"`

		// URL specifies a URL where more information about the asset can be
		// retrieved
		URL string `codec:"au"`

		// MetadataHash specifies a commitment to some unspecified asset
		// metadata. The format of this metadata is up to the application.
		MetadataHash [32]byte `codec:"am"`

		// Manager specifies an account that is allowed to change the
		// non-zero addresses in this AssetParams.
		Manager Address `codec:"m"`

		// Reserve specifies an account whose holdings of this asset
		// should be reported as "not minted".
		Reserve Address `codec:"r"`

		// Freeze specifies an account that is allowed to change the
		// frozen state of holdings of this asset.
		Freeze Address `codec:"f"`

		// Clawback specifies an account that is allowed to take units
		// of this asset from any account.
		Clawback Address `codec:"c"`
	}
	// AssetConfigTxnFields captures the fields used for asset
	// allocation, re-configuration, and destruction.
	AssetConfigTxnFields struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		// ConfigAsset is the asset being configured or destroyed.
		// A zero value means allocation
		ConfigAsset AssetIndex `codec:"caid"`

		// AssetParams are the parameters for the asset being
		// created or re-configured.  A zero value means destruction.
		AssetParams AssetParams `codec:"apar"`
	}

	// AssetTransferTxnFields captures the fields used for asset transfers.
	AssetTransferTxnFields struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		XferAsset AssetIndex `codec:"xaid"`

		// AssetAmount is the amount of asset to transfer.
		// A zero amount transferred to self allocates that asset
		// in the account's Assets map.
		AssetAmount uint64 `codec:"aamt"`

		// AssetSender is the sender of the transfer.  If this is not
		// a zero value, the real transaction sender must be the Clawback
		// address from the AssetParams.  If this is the zero value,
		// the asset is sent from the transaction's Sender.
		AssetSender Address `codec:"asnd"`

		// AssetReceiver is the recipient of the transfer.
		AssetReceiver Address `codec:"arcv"`

		// AssetCloseTo indicates that the asset should be removed
		// from the account's Assets map, and specifies where the remaining
		// asset holdings should be transferred.  It's always valid to transfer
		// remaining asset holdings to the creator account.
		AssetCloseTo Address `codec:"aclose"`
	}

	// AssetFreezeTxnFields captures the fields used for freezing asset slots.
	AssetFreezeTxnFields struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`

		// FreezeAccount is the address of the account whose asset
		// slot is being frozen or un-frozen.
		FreezeAccount Address `codec:"fadd"`

		// FreezeAsset is the asset ID being frozen or un-frozen.
		FreezeAsset AssetIndex `codec:"faid"`

		// AssetFrozen is the new frozen value.
		AssetFrozen bool `codec:"afrz"`
	}

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

var zeroAddr = [32]byte{}

func (a Address) IsZero() bool {
	return a == zeroAddr
}

var zeroAP = AssetParams{}

func (ap AssetParams) IsZero() bool {
	return ap == zeroAP
}
