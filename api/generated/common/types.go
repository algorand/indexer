// Package common provides primitives to interact the openapi HTTP API.
//
// Code generated by github.com/algorand/oapi-codegen DO NOT EDIT.
package common

import (
	"time"
)

// Account defines model for Account.
type Account struct {

	// the account public key
	Address string `json:"address"`

	// \[algo\] total number of MicroAlgos in the account
	Amount uint64 `json:"amount"`

	// specifies the amount of MicroAlgos in the account, without the pending rewards.
	AmountWithoutPendingRewards uint64 `json:"amount-without-pending-rewards"`

	// \[appl\] applications local data stored in this account.
	//
	// Note the raw object uses `map[int] -> AppLocalState` for this type.
	AppsLocalState *[]ApplicationLocalState `json:"apps-local-state,omitempty"`

	// \[teap\] the sum of all extra application program pages for this account.
	AppsTotalExtraPages *uint64 `json:"apps-total-extra-pages,omitempty"`

	// Specifies maximums on the number of each type that may be stored.
	AppsTotalSchema *ApplicationStateSchema `json:"apps-total-schema,omitempty"`

	// \[asset\] assets held by this account.
	//
	// Note the raw object uses `map[int] -> AssetHolding` for this type.
	Assets *[]AssetHolding `json:"assets,omitempty"`

	// \[spend\] the address against which signing should be checked. If empty, the address of the current account is used. This field can be updated in any transaction by setting the RekeyTo field.
	AuthAddr *string `json:"auth-addr,omitempty"`

	// Round during which this account was most recently closed.
	ClosedAtRound *uint64 `json:"closed-at-round,omitempty"`

	// \[appp\] parameters of applications created by this account including app global data.
	//
	// Note: the raw account uses `map[int] -> AppParams` for this type.
	CreatedApps *[]Application `json:"created-apps,omitempty"`

	// \[apar\] parameters of assets created by this account.
	//
	// Note: the raw account uses `map[int] -> Asset` for this type.
	CreatedAssets *[]Asset `json:"created-assets,omitempty"`

	// Round during which this account first appeared in a transaction.
	CreatedAtRound *uint64 `json:"created-at-round,omitempty"`

	// Whether or not this account is currently closed.
	Deleted *bool `json:"deleted,omitempty"`

	// AccountParticipation describes the parameters used by this account in consensus protocol.
	Participation *AccountParticipation `json:"participation,omitempty"`

	// amount of MicroAlgos of pending rewards in this account.
	PendingRewards uint64 `json:"pending-rewards"`

	// \[ebase\] used as part of the rewards computation. Only applicable to accounts which are participating.
	RewardBase *uint64 `json:"reward-base,omitempty"`

	// \[ern\] total rewards of MicroAlgos the account has received, including pending rewards.
	Rewards uint64 `json:"rewards"`

	// The round for which this information is relevant.
	Round uint64 `json:"round"`

	// Indicates what type of signature is used by this account, must be one of:
	// * sig
	// * msig
	// * lsig
	// * or null if unknown
	SigType *string `json:"sig-type,omitempty"`

	// \[onl\] delegation status of the account's MicroAlgos
	// * Offline - indicates that the associated account is delegated.
	// *  Online  - indicates that the associated account used as part of the delegation pool.
	// *   NotParticipating - indicates that the associated account is neither a delegator nor a delegate.
	Status string `json:"status"`
}

// AccountParticipation defines model for AccountParticipation.
type AccountParticipation struct {

	// \[sel\] Selection public key (if any) currently registered for this round.
	SelectionParticipationKey []byte `json:"selection-participation-key"`

	// \[state\] root of the state proof key (if any)
	StateProofKey *[]byte `json:"state-proof-key,omitempty"`

	// \[voteFst\] First round for which this participation is valid.
	VoteFirstValid uint64 `json:"vote-first-valid"`

	// \[voteKD\] Number of subkeys in each batch of participation keys.
	VoteKeyDilution uint64 `json:"vote-key-dilution"`

	// \[voteLst\] Last round for which this participation is valid.
	VoteLastValid uint64 `json:"vote-last-valid"`

	// \[vote\] root participation public key (if any) currently registered for this round.
	VoteParticipationKey []byte `json:"vote-participation-key"`
}

// AccountStateDelta defines model for AccountStateDelta.
type AccountStateDelta struct {
	Address string `json:"address"`

	// Application state delta.
	Delta StateDelta `json:"delta"`
}

// Application defines model for Application.
type Application struct {

	// Round when this application was created.
	CreatedAtRound *uint64 `json:"created-at-round,omitempty"`

	// Whether or not this application is currently deleted.
	Deleted *bool `json:"deleted,omitempty"`

	// Round when this application was deleted.
	DeletedAtRound *uint64 `json:"deleted-at-round,omitempty"`

	// \[appidx\] application index.
	Id uint64 `json:"id"`

	// Stores the global information associated with an application.
	Params ApplicationParams `json:"params"`
}

// ApplicationLocalState defines model for ApplicationLocalState.
type ApplicationLocalState struct {

	// Round when account closed out of the application.
	ClosedOutAtRound *uint64 `json:"closed-out-at-round,omitempty"`

	// Whether or not the application local state is currently deleted from its account.
	Deleted *bool `json:"deleted,omitempty"`

	// The application which this local state is for.
	Id uint64 `json:"id"`

	// Represents a key-value store for use in an application.
	KeyValue *TealKeyValueStore `json:"key-value,omitempty"`

	// Round when the account opted into the application.
	OptedInAtRound *uint64 `json:"opted-in-at-round,omitempty"`

	// Specifies maximums on the number of each type that may be stored.
	Schema ApplicationStateSchema `json:"schema"`
}

// ApplicationLogData defines model for ApplicationLogData.
type ApplicationLogData struct {

	// \[lg\] Logs for the application being executed by the transaction.
	Logs [][]byte `json:"logs"`

	// Transaction ID
	Txid string `json:"txid"`
}

// ApplicationParams defines model for ApplicationParams.
type ApplicationParams struct {

	// \[approv\] approval program.
	ApprovalProgram []byte `json:"approval-program"`

	// \[clearp\] approval program.
	ClearStateProgram []byte `json:"clear-state-program"`

	// The address that created this application. This is the address where the parameters and global state for this application can be found.
	Creator *string `json:"creator,omitempty"`

	// \[epp\] the amount of extra program pages available to this app.
	ExtraProgramPages *uint64 `json:"extra-program-pages,omitempty"`

	// Represents a key-value store for use in an application.
	GlobalState *TealKeyValueStore `json:"global-state,omitempty"`

	// Specifies maximums on the number of each type that may be stored.
	GlobalStateSchema *ApplicationStateSchema `json:"global-state-schema,omitempty"`

	// Specifies maximums on the number of each type that may be stored.
	LocalStateSchema *ApplicationStateSchema `json:"local-state-schema,omitempty"`
}

// ApplicationStateSchema defines model for ApplicationStateSchema.
type ApplicationStateSchema struct {

	// \[nbs\] num of byte slices.
	NumByteSlice uint64 `json:"num-byte-slice"`

	// \[nui\] num of uints.
	NumUint uint64 `json:"num-uint"`
}

// Asset defines model for Asset.
type Asset struct {

	// Round during which this asset was created.
	CreatedAtRound *uint64 `json:"created-at-round,omitempty"`

	// Whether or not this asset is currently deleted.
	Deleted *bool `json:"deleted,omitempty"`

	// Round during which this asset was destroyed.
	DestroyedAtRound *uint64 `json:"destroyed-at-round,omitempty"`

	// unique asset identifier
	Index uint64 `json:"index"`

	// AssetParams specifies the parameters for an asset.
	//
	// \[apar\] when part of an AssetConfig transaction.
	//
	// Definition:
	// data/transactions/asset.go : AssetParams
	Params AssetParams `json:"params"`
}

// AssetHolding defines model for AssetHolding.
type AssetHolding struct {

	// \[a\] number of units held.
	Amount uint64 `json:"amount"`

	// Asset ID of the holding.
	AssetId uint64 `json:"asset-id"`

	// Address that created this asset. This is the address where the parameters for this asset can be found, and also the address where unwanted asset units can be sent in the worst case.
	Creator string `json:"creator"`

	// Whether or not the asset holding is currently deleted from its account.
	Deleted *bool `json:"deleted,omitempty"`

	// \[f\] whether or not the holding is frozen.
	IsFrozen bool `json:"is-frozen"`

	// Round during which the account opted into this asset holding.
	OptedInAtRound *uint64 `json:"opted-in-at-round,omitempty"`

	// Round during which the account opted out of this asset holding.
	OptedOutAtRound *uint64 `json:"opted-out-at-round,omitempty"`
}

// AssetParams defines model for AssetParams.
type AssetParams struct {

	// \[c\] Address of account used to clawback holdings of this asset.  If empty, clawback is not permitted.
	Clawback *string `json:"clawback,omitempty"`

	// The address that created this asset. This is the address where the parameters for this asset can be found, and also the address where unwanted asset units can be sent in the worst case.
	Creator string `json:"creator"`

	// \[dc\] The number of digits to use after the decimal point when displaying this asset. If 0, the asset is not divisible. If 1, the base unit of the asset is in tenths. If 2, the base unit of the asset is in hundredths, and so on. This value must be between 0 and 19 (inclusive).
	Decimals uint64 `json:"decimals"`

	// \[df\] Whether holdings of this asset are frozen by default.
	DefaultFrozen *bool `json:"default-frozen,omitempty"`

	// \[f\] Address of account used to freeze holdings of this asset.  If empty, freezing is not permitted.
	Freeze *string `json:"freeze,omitempty"`

	// \[m\] Address of account used to manage the keys of this asset and to destroy it.
	Manager *string `json:"manager,omitempty"`

	// \[am\] A commitment to some unspecified asset metadata. The format of this metadata is up to the application.
	MetadataHash *[]byte `json:"metadata-hash,omitempty"`

	// \[an\] Name of this asset, as supplied by the creator. Included only when the asset name is composed of printable utf-8 characters.
	Name *string `json:"name,omitempty"`

	// Base64 encoded name of this asset, as supplied by the creator.
	NameB64 *[]byte `json:"name-b64,omitempty"`

	// \[r\] Address of account holding reserve (non-minted) units of this asset.
	Reserve *string `json:"reserve,omitempty"`

	// \[t\] The total number of units of this asset.
	Total uint64 `json:"total"`

	// \[un\] Name of a unit of this asset, as supplied by the creator. Included only when the name of a unit of this asset is composed of printable utf-8 characters.
	UnitName *string `json:"unit-name,omitempty"`

	// Base64 encoded name of a unit of this asset, as supplied by the creator.
	UnitNameB64 *[]byte `json:"unit-name-b64,omitempty"`

	// \[au\] URL where more information about the asset can be retrieved. Included only when the URL is composed of printable utf-8 characters.
	Url *string `json:"url,omitempty"`

	// Base64 encoded URL where more information about the asset can be retrieved.
	UrlB64 *[]byte `json:"url-b64,omitempty"`
}

// Block defines model for Block.
type Block struct {

	// \[gh\] hash to which this block belongs.
	GenesisHash []byte `json:"genesis-hash"`

	// \[gen\] ID to which this block belongs.
	GenesisId string `json:"genesis-id"`

	// \[prev\] Previous block hash.
	PreviousBlockHash []byte `json:"previous-block-hash"`

	// Fields relating to rewards,
	Rewards *BlockRewards `json:"rewards,omitempty"`

	// \[rnd\] Current round on which this block was appended to the chain.
	Round uint64 `json:"round"`

	// \[seed\] Sortition seed.
	Seed []byte `json:"seed"`

	// \[ts\] Block creation timestamp in seconds since eposh
	Timestamp uint64 `json:"timestamp"`

	// \[txns\] list of transactions corresponding to a given round.
	Transactions *[]Transaction `json:"transactions,omitempty"`

	// \[txn\] TransactionsRoot authenticates the set of transactions appearing in the block. More specifically, it's the root of a merkle tree whose leaves are the block's Txids, in lexicographic order. For the empty block, it's 0. Note that the TxnRoot does not authenticate the signatures on the transactions, only the transactions themselves. Two blocks with the same transactions but in a different order and with different signatures will have the same TxnRoot.
	TransactionsRoot []byte `json:"transactions-root"`

	// \[tc\] TxnCounter counts the number of transactions committed in the ledger, from the time at which support for this feature was introduced.
	//
	// Specifically, TxnCounter is the number of the next transaction that will be committed after this block.  It is 0 when no transactions have ever been committed (since TxnCounter started being supported).
	TxnCounter *uint64 `json:"txn-counter,omitempty"`

	// Fields relating to a protocol upgrade.
	UpgradeState *BlockUpgradeState `json:"upgrade-state,omitempty"`

	// Fields relating to voting for a protocol upgrade.
	UpgradeVote *BlockUpgradeVote `json:"upgrade-vote,omitempty"`
}

// BlockRewards defines model for BlockRewards.
type BlockRewards struct {

	// \[fees\] accepts transaction fees, it can only spend to the incentive pool.
	FeeSink string `json:"fee-sink"`

	// \[rwcalr\] number of leftover MicroAlgos after the distribution of rewards-rate MicroAlgos for every reward unit in the next round.
	RewardsCalculationRound uint64 `json:"rewards-calculation-round"`

	// \[earn\] How many rewards, in MicroAlgos, have been distributed to each RewardUnit of MicroAlgos since genesis.
	RewardsLevel uint64 `json:"rewards-level"`

	// \[rwd\] accepts periodic injections from the fee-sink and continually redistributes them as rewards.
	RewardsPool string `json:"rewards-pool"`

	// \[rate\] Number of new MicroAlgos added to the participation stake from rewards at the next round.
	RewardsRate uint64 `json:"rewards-rate"`

	// \[frac\] Number of leftover MicroAlgos after the distribution of RewardsRate/rewardUnits MicroAlgos for every reward unit in the next round.
	RewardsResidue uint64 `json:"rewards-residue"`
}

// BlockUpgradeState defines model for BlockUpgradeState.
type BlockUpgradeState struct {

	// \[proto\] The current protocol version.
	CurrentProtocol string `json:"current-protocol"`

	// \[nextproto\] The next proposed protocol version.
	NextProtocol *string `json:"next-protocol,omitempty"`

	// \[nextyes\] Number of blocks which approved the protocol upgrade.
	NextProtocolApprovals *uint64 `json:"next-protocol-approvals,omitempty"`

	// \[nextswitch\] Round on which the protocol upgrade will take effect.
	NextProtocolSwitchOn *uint64 `json:"next-protocol-switch-on,omitempty"`

	// \[nextbefore\] Deadline round for this protocol upgrade (No votes will be consider after this round).
	NextProtocolVoteBefore *uint64 `json:"next-protocol-vote-before,omitempty"`
}

// BlockUpgradeVote defines model for BlockUpgradeVote.
type BlockUpgradeVote struct {

	// \[upgradeyes\] Indicates a yes vote for the current proposal.
	UpgradeApprove *bool `json:"upgrade-approve,omitempty"`

	// \[upgradedelay\] Indicates the time between acceptance and execution.
	UpgradeDelay *uint64 `json:"upgrade-delay,omitempty"`

	// \[upgradeprop\] Indicates a proposed upgrade.
	UpgradePropose *string `json:"upgrade-propose,omitempty"`
}

// EvalDelta defines model for EvalDelta.
type EvalDelta struct {

	// \[at\] delta action.
	Action uint64 `json:"action"`

	// \[bs\] bytes value.
	Bytes *string `json:"bytes,omitempty"`

	// \[ui\] uint value.
	Uint *uint64 `json:"uint,omitempty"`
}

// EvalDeltaKeyValue defines model for EvalDeltaKeyValue.
type EvalDeltaKeyValue struct {
	Key string `json:"key"`

	// Represents a TEAL value delta.
	Value EvalDelta `json:"value"`
}

// HealthCheck defines model for HealthCheck.
type HealthCheck struct {
	Data        *map[string]interface{} `json:"data,omitempty"`
	DbAvailable bool                    `json:"db-available"`
	Errors      *[]string               `json:"errors,omitempty"`
	IsMigrating bool                    `json:"is-migrating"`
	Message     string                  `json:"message"`
	Round       uint64                  `json:"round"`

	// Current version.
	Version string `json:"version"`
}

// MiniAssetHolding defines model for MiniAssetHolding.
type MiniAssetHolding struct {
	Address string `json:"address"`
	Amount  uint64 `json:"amount"`

	// Whether or not this asset holding is currently deleted from its account.
	Deleted  *bool `json:"deleted,omitempty"`
	IsFrozen bool  `json:"is-frozen"`

	// Round during which the account opted into the asset.
	OptedInAtRound *uint64 `json:"opted-in-at-round,omitempty"`

	// Round during which the account opted out of the asset.
	OptedOutAtRound *uint64 `json:"opted-out-at-round,omitempty"`
}

// OnCompletion defines model for OnCompletion.
type OnCompletion string

// StateDelta defines model for StateDelta.
type StateDelta []EvalDeltaKeyValue

// StateSchema defines model for StateSchema.
type StateSchema struct {

	// Maximum number of TEAL byte slices that may be stored in the key/value store.
	NumByteSlice uint64 `json:"num-byte-slice"`

	// Maximum number of TEAL uints that may be stored in the key/value store.
	NumUint uint64 `json:"num-uint"`
}

// TealKeyValue defines model for TealKeyValue.
type TealKeyValue struct {
	Key string `json:"key"`

	// Represents a TEAL value.
	Value TealValue `json:"value"`
}

// TealKeyValueStore defines model for TealKeyValueStore.
type TealKeyValueStore []TealKeyValue

// TealValue defines model for TealValue.
type TealValue struct {

	// \[tb\] bytes value.
	Bytes string `json:"bytes"`

	// \[tt\] value type. Value `1` refers to **bytes**, value `2` refers to **uint**
	Type uint64 `json:"type"`

	// \[ui\] uint value.
	Uint uint64 `json:"uint"`
}

// Transaction defines model for Transaction.
type Transaction struct {

	// Fields for application transactions.
	//
	// Definition:
	// data/transactions/application.go : ApplicationCallTxnFields
	ApplicationTransaction *TransactionApplication `json:"application-transaction,omitempty"`

	// Fields for asset allocation, re-configuration, and destruction.
	//
	//
	// A zero value for asset-id indicates asset creation.
	// A zero value for the params indicates asset destruction.
	//
	// Definition:
	// data/transactions/asset.go : AssetConfigTxnFields
	AssetConfigTransaction *TransactionAssetConfig `json:"asset-config-transaction,omitempty"`

	// Fields for an asset freeze transaction.
	//
	// Definition:
	// data/transactions/asset.go : AssetFreezeTxnFields
	AssetFreezeTransaction *TransactionAssetFreeze `json:"asset-freeze-transaction,omitempty"`

	// Fields for an asset transfer transaction.
	//
	// Definition:
	// data/transactions/asset.go : AssetTransferTxnFields
	AssetTransferTransaction *TransactionAssetTransfer `json:"asset-transfer-transaction,omitempty"`

	// \[sgnr\] this is included with signed transactions when the signing address does not equal the sender. The backend can use this to ensure that auth addr is equal to the accounts auth addr.
	AuthAddr *string `json:"auth-addr,omitempty"`

	// \[rc\] rewards applied to close-remainder-to account.
	CloseRewards *uint64 `json:"close-rewards,omitempty"`

	// \[ca\] closing amount for transaction.
	ClosingAmount *uint64 `json:"closing-amount,omitempty"`

	// Round when the transaction was confirmed.
	ConfirmedRound *uint64 `json:"confirmed-round,omitempty"`

	// Specifies an application index (ID) if an application was created with this transaction.
	CreatedApplicationIndex *uint64 `json:"created-application-index,omitempty"`

	// Specifies an asset index (ID) if an asset was created with this transaction.
	CreatedAssetIndex *uint64 `json:"created-asset-index,omitempty"`

	// \[fee\] Transaction fee.
	Fee uint64 `json:"fee"`

	// \[fv\] First valid round for this transaction.
	FirstValid uint64 `json:"first-valid"`

	// \[gh\] Hash of genesis block.
	GenesisHash *[]byte `json:"genesis-hash,omitempty"`

	// \[gen\] genesis block ID.
	GenesisId *string `json:"genesis-id,omitempty"`

	// Application state delta.
	GlobalStateDelta *StateDelta `json:"global-state-delta,omitempty"`

	// \[grp\] Base64 encoded byte array of a sha512/256 digest. When present indicates that this transaction is part of a transaction group and the value is the sha512/256 hash of the transactions in that group.
	Group *[]byte `json:"group,omitempty"`

	// Transaction ID
	Id *string `json:"id,omitempty"`

	// Inner transactions produced by application execution.
	InnerTxns *[]Transaction `json:"inner-txns,omitempty"`

	// Offset into the round where this transaction was confirmed.
	IntraRoundOffset *uint64 `json:"intra-round-offset,omitempty"`

	// Fields for a keyreg transaction.
	//
	// Definition:
	// data/transactions/keyreg.go : KeyregTxnFields
	KeyregTransaction *TransactionKeyreg `json:"keyreg-transaction,omitempty"`

	// \[lv\] Last valid round for this transaction.
	LastValid uint64 `json:"last-valid"`

	// \[lx\] Base64 encoded 32-byte array. Lease enforces mutual exclusion of transactions.  If this field is nonzero, then once the transaction is confirmed, it acquires the lease identified by the (Sender, Lease) pair of the transaction until the LastValid round passes.  While this transaction possesses the lease, no other transaction specifying this lease can be confirmed.
	Lease *[]byte `json:"lease,omitempty"`

	// \[ld\] Local state key/value changes for the application being executed by this transaction.
	LocalStateDelta *[]AccountStateDelta `json:"local-state-delta,omitempty"`

	// \[lg\] Logs for the application being executed by this transaction.
	Logs *[][]byte `json:"logs,omitempty"`

	// \[note\] Free form data.
	Note *[]byte `json:"note,omitempty"`

	// Fields for a payment transaction.
	//
	// Definition:
	// data/transactions/payment.go : PaymentTxnFields
	PaymentTransaction *TransactionPayment `json:"payment-transaction,omitempty"`

	// \[rr\] rewards applied to receiver account.
	ReceiverRewards *uint64 `json:"receiver-rewards,omitempty"`

	// \[rekey\] when included in a valid transaction, the accounts auth addr will be updated with this value and future signatures must be signed with the key represented by this address.
	RekeyTo *string `json:"rekey-to,omitempty"`

	// Time when the block this transaction is in was confirmed.
	RoundTime *uint64 `json:"round-time,omitempty"`

	// \[snd\] Sender's address.
	Sender string `json:"sender"`

	// \[rs\] rewards applied to sender account.
	SenderRewards *uint64 `json:"sender-rewards,omitempty"`

	// Validation signature associated with some data. Only one of the signatures should be provided.
	Signature *TransactionSignature `json:"signature,omitempty"`

	// \[type\] Indicates what type of transaction this is. Different types have different fields.
	//
	// Valid types, and where their fields are stored:
	// * \[pay\] payment-transaction
	// * \[keyreg\] keyreg-transaction
	// * \[acfg\] asset-config-transaction
	// * \[axfer\] asset-transfer-transaction
	// * \[afrz\] asset-freeze-transaction
	// * \[appl\] application-transaction
	TxType string `json:"tx-type"`
}

// TransactionApplication defines model for TransactionApplication.
type TransactionApplication struct {

	// \[apat\] List of accounts in addition to the sender that may be accessed from the application's approval-program and clear-state-program.
	Accounts *[]string `json:"accounts,omitempty"`

	// \[apaa\] transaction specific arguments accessed from the application's approval-program and clear-state-program.
	ApplicationArgs *[]string `json:"application-args,omitempty"`

	// \[apid\] ID of the application being configured or empty if creating.
	ApplicationId uint64 `json:"application-id"`

	// \[apap\] Logic executed for every application transaction, except when on-completion is set to "clear". It can read and write global state for the application, as well as account-specific local state. Approval programs may reject the transaction.
	ApprovalProgram *[]byte `json:"approval-program,omitempty"`

	// \[apsu\] Logic executed for application transactions with on-completion set to "clear". It can read and write global state for the application, as well as account-specific local state. Clear state programs cannot reject the transaction.
	ClearStateProgram *[]byte `json:"clear-state-program,omitempty"`

	// \[epp\] specifies the additional app program len requested in pages.
	ExtraProgramPages *uint64 `json:"extra-program-pages,omitempty"`

	// \[apfa\] Lists the applications in addition to the application-id whose global states may be accessed by this application's approval-program and clear-state-program. The access is read-only.
	ForeignApps *[]uint64 `json:"foreign-apps,omitempty"`

	// \[apas\] lists the assets whose parameters may be accessed by this application's ApprovalProgram and ClearStateProgram. The access is read-only.
	ForeignAssets *[]uint64 `json:"foreign-assets,omitempty"`

	// Represents a \[apls\] local-state or \[apgs\] global-state schema. These schemas determine how much storage may be used in a local-state or global-state for an application. The more space used, the larger minimum balance must be maintained in the account holding the data.
	GlobalStateSchema *StateSchema `json:"global-state-schema,omitempty"`

	// Represents a \[apls\] local-state or \[apgs\] global-state schema. These schemas determine how much storage may be used in a local-state or global-state for an application. The more space used, the larger minimum balance must be maintained in the account holding the data.
	LocalStateSchema *StateSchema `json:"local-state-schema,omitempty"`

	// \[apan\] defines the what additional actions occur with the transaction.
	//
	// Valid types:
	// * noop
	// * optin
	// * closeout
	// * clear
	// * update
	// * update
	// * delete
	OnCompletion OnCompletion `json:"on-completion"`
}

// TransactionAssetConfig defines model for TransactionAssetConfig.
type TransactionAssetConfig struct {

	// \[xaid\] ID of the asset being configured or empty if creating.
	AssetId *uint64 `json:"asset-id,omitempty"`

	// AssetParams specifies the parameters for an asset.
	//
	// \[apar\] when part of an AssetConfig transaction.
	//
	// Definition:
	// data/transactions/asset.go : AssetParams
	Params *AssetParams `json:"params,omitempty"`
}

// TransactionAssetFreeze defines model for TransactionAssetFreeze.
type TransactionAssetFreeze struct {

	// \[fadd\] Address of the account whose asset is being frozen or thawed.
	Address string `json:"address"`

	// \[faid\] ID of the asset being frozen or thawed.
	AssetId uint64 `json:"asset-id"`

	// \[afrz\] The new freeze status.
	NewFreezeStatus bool `json:"new-freeze-status"`
}

// TransactionAssetTransfer defines model for TransactionAssetTransfer.
type TransactionAssetTransfer struct {

	// \[aamt\] Amount of asset to transfer. A zero amount transferred to self allocates that asset in the account's Assets map.
	Amount uint64 `json:"amount"`

	// \[xaid\] ID of the asset being transferred.
	AssetId uint64 `json:"asset-id"`

	// Number of assets transfered to the close-to account as part of the transaction.
	CloseAmount *uint64 `json:"close-amount,omitempty"`

	// \[aclose\] Indicates that the asset should be removed from the account's Assets map, and specifies where the remaining asset holdings should be transferred.  It's always valid to transfer remaining asset holdings to the creator account.
	CloseTo *string `json:"close-to,omitempty"`

	// \[arcv\] Recipient address of the transfer.
	Receiver string `json:"receiver"`

	// \[asnd\] The effective sender during a clawback transactions. If this is not a zero value, the real transaction sender must be the Clawback address from the AssetParams.
	Sender *string `json:"sender,omitempty"`
}

// TransactionKeyreg defines model for TransactionKeyreg.
type TransactionKeyreg struct {

	// \[nonpart\] Mark the account as participating or non-participating.
	NonParticipation *bool `json:"non-participation,omitempty"`

	// \[selkey\] Public key used with the Verified Random Function (VRF) result during committee selection.
	SelectionParticipationKey *[]byte `json:"selection-participation-key,omitempty"`

	// \[sprfkey\] State proof key used in key registration transactions.
	StateProofKey *[]byte `json:"state-proof-key,omitempty"`

	// \[votefst\] First round this participation key is valid.
	VoteFirstValid *uint64 `json:"vote-first-valid,omitempty"`

	// \[votekd\] Number of subkeys in each batch of participation keys.
	VoteKeyDilution *uint64 `json:"vote-key-dilution,omitempty"`

	// \[votelst\] Last round this participation key is valid.
	VoteLastValid *uint64 `json:"vote-last-valid,omitempty"`

	// \[votekey\] Participation public key used in key registration transactions.
	VoteParticipationKey *[]byte `json:"vote-participation-key,omitempty"`
}

// TransactionPayment defines model for TransactionPayment.
type TransactionPayment struct {

	// \[amt\] number of MicroAlgos intended to be transferred.
	Amount uint64 `json:"amount"`

	// Number of MicroAlgos that were sent to the close-remainder-to address when closing the sender account.
	CloseAmount *uint64 `json:"close-amount,omitempty"`

	// \[close\] when set, indicates that the sending account should be closed and all remaining funds be transferred to this address.
	CloseRemainderTo *string `json:"close-remainder-to,omitempty"`

	// \[rcv\] receiver's address.
	Receiver string `json:"receiver"`
}

// TransactionSignature defines model for TransactionSignature.
type TransactionSignature struct {

	// \[lsig\] Programatic transaction signature.
	//
	// Definition:
	// data/transactions/logicsig.go
	Logicsig *TransactionSignatureLogicsig `json:"logicsig,omitempty"`

	// \[msig\] structure holding multiple subsignatures.
	//
	// Definition:
	// crypto/multisig.go : MultisigSig
	Multisig *TransactionSignatureMultisig `json:"multisig,omitempty"`

	// \[sig\] Standard ed25519 signature.
	Sig *[]byte `json:"sig,omitempty"`
}

// TransactionSignatureLogicsig defines model for TransactionSignatureLogicsig.
type TransactionSignatureLogicsig struct {

	// \[arg\] Logic arguments, base64 encoded.
	Args *[]string `json:"args,omitempty"`

	// \[l\] Program signed by a signature or multi signature, or hashed to be the address of ana ccount. Base64 encoded TEAL program.
	Logic []byte `json:"logic"`

	// \[msig\] structure holding multiple subsignatures.
	//
	// Definition:
	// crypto/multisig.go : MultisigSig
	MultisigSignature *TransactionSignatureMultisig `json:"multisig-signature,omitempty"`

	// \[sig\] ed25519 signature.
	Signature *[]byte `json:"signature,omitempty"`
}

// TransactionSignatureMultisig defines model for TransactionSignatureMultisig.
type TransactionSignatureMultisig struct {

	// \[subsig\] holds pairs of public key and signatures.
	Subsignature *[]TransactionSignatureMultisigSubsignature `json:"subsignature,omitempty"`

	// \[thr\]
	Threshold *uint64 `json:"threshold,omitempty"`

	// \[v\]
	Version *uint64 `json:"version,omitempty"`
}

// TransactionSignatureMultisigSubsignature defines model for TransactionSignatureMultisigSubsignature.
type TransactionSignatureMultisigSubsignature struct {

	// \[pk\]
	PublicKey *[]byte `json:"public-key,omitempty"`

	// \[s\]
	Signature *[]byte `json:"signature,omitempty"`
}

// AccountId defines model for account-id.
type AccountId string

// Address defines model for address.
type Address string

// AddressRole defines model for address-role.
type AddressRole string

// AfterTime defines model for after-time.
type AfterTime time.Time

// ApplicationId defines model for application-id.
type ApplicationId uint64

// AssetId defines model for asset-id.
type AssetId uint64

// AuthAddr defines model for auth-addr.
type AuthAddr string

// BeforeTime defines model for before-time.
type BeforeTime time.Time

// CurrencyGreaterThan defines model for currency-greater-than.
type CurrencyGreaterThan uint64

// CurrencyLessThan defines model for currency-less-than.
type CurrencyLessThan uint64

// ExcludeCloseTo defines model for exclude-close-to.
type ExcludeCloseTo bool

// IncludeAll defines model for include-all.
type IncludeAll bool

// Limit defines model for limit.
type Limit uint64

// MaxRound defines model for max-round.
type MaxRound uint64

// MinRound defines model for min-round.
type MinRound uint64

// Next defines model for next.
type Next string

// NotePrefix defines model for note-prefix.
type NotePrefix string

// RekeyTo defines model for rekey-to.
type RekeyTo bool

// Round defines model for round.
type Round uint64

// RoundNumber defines model for round-number.
type RoundNumber uint64

// SenderAddress defines model for sender-address.
type SenderAddress string

// SigType defines model for sig-type.
type SigType string

// TxType defines model for tx-type.
type TxType string

// Txid defines model for txid.
type Txid string

// AccountResponse defines model for AccountResponse.
type AccountResponse struct {

	// Account information at a given round.
	//
	// Definition:
	// data/basics/userBalance.go : AccountData
	Account Account `json:"account"`

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`
}

// AccountsResponse defines model for AccountsResponse.
type AccountsResponse struct {
	Accounts []Account `json:"accounts"`

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`

	// Used for pagination, when making another request provide this token with the next parameter.
	NextToken *string `json:"next-token,omitempty"`
}

// ApplicationLogsResponse defines model for ApplicationLogsResponse.
type ApplicationLogsResponse struct {

	// \[appidx\] application index.
	ApplicationId uint64 `json:"application-id"`

	// Round at which the results were computed.
	CurrentRound uint64                `json:"current-round"`
	LogData      *[]ApplicationLogData `json:"log-data,omitempty"`

	// Used for pagination, when making another request provide this token with the next parameter.
	NextToken *string `json:"next-token,omitempty"`
}

// ApplicationResponse defines model for ApplicationResponse.
type ApplicationResponse struct {

	// Application index and its parameters
	Application *Application `json:"application,omitempty"`

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`
}

// ApplicationsResponse defines model for ApplicationsResponse.
type ApplicationsResponse struct {
	Applications []Application `json:"applications"`

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`

	// Used for pagination, when making another request provide this token with the next parameter.
	NextToken *string `json:"next-token,omitempty"`
}

// AssetBalancesResponse defines model for AssetBalancesResponse.
type AssetBalancesResponse struct {
	Balances []MiniAssetHolding `json:"balances"`

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`

	// Used for pagination, when making another request provide this token with the next parameter.
	NextToken *string `json:"next-token,omitempty"`
}

// AssetResponse defines model for AssetResponse.
type AssetResponse struct {

	// Specifies both the unique identifier and the parameters for an asset
	Asset Asset `json:"asset"`

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`
}

// AssetsResponse defines model for AssetsResponse.
type AssetsResponse struct {
	Assets []Asset `json:"assets"`

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`

	// Used for pagination, when making another request provide this token with the next parameter.
	NextToken *string `json:"next-token,omitempty"`
}

// BlockResponse defines model for BlockResponse.
type BlockResponse Block

// ErrorResponse defines model for ErrorResponse.
type ErrorResponse struct {
	Data    *map[string]interface{} `json:"data,omitempty"`
	Message string                  `json:"message"`
}

// HealthCheckResponse defines model for HealthCheckResponse.
type HealthCheckResponse HealthCheck

// TransactionResponse defines model for TransactionResponse.
type TransactionResponse struct {

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`

	// Contains all fields common to all transactions and serves as an envelope to all transactions type. Represents both regular and inner transactions.
	//
	// Definition:
	// data/transactions/signedtxn.go : SignedTxn
	// data/transactions/transaction.go : Transaction
	Transaction Transaction `json:"transaction"`
}

// TransactionsResponse defines model for TransactionsResponse.
type TransactionsResponse struct {

	// Round at which the results were computed.
	CurrentRound uint64 `json:"current-round"`

	// Used for pagination, when making another request provide this token with the next parameter.
	NextToken    *string       `json:"next-token,omitempty"`
	Transactions []Transaction `json:"transactions"`
}
