package accounting

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

// State is used to record accounting changes as a result of processing transactions.
type State struct {
	defaultFrozen map[uint64]bool

	currentRound uint64

	idb.RoundUpdates

	feeAddr    types.Address
	rewardAddr types.Address

	rewardsLevel uint64

	accountTypes accountTypeCache
}

// New creates a new State object.
func New(defaultFrozenCache map[uint64]bool) *State {
	result := &State{defaultFrozen: defaultFrozenCache}
	result.Clear()
	return result
}

// InitRound should be called before each round to initialize the accounting state.
func (accounting *State) InitRound(blockHeader *types.BlockHeader) error {
	return accounting.InitRoundParts(uint64(blockHeader.Round), blockHeader.FeeSink, blockHeader.RewardsPool, blockHeader.RewardsLevel)
}

// InitRoundParts are the specific parts from a block needed to initialize accounting. Used for testing, normally you would pass in the block.
func (accounting *State) InitRoundParts(round uint64, feeSink, rewardsPool sdk_types.Address, rewardsLevel uint64) error {
	accounting.RoundUpdates.Clear()
	accounting.feeAddr = feeSink
	accounting.rewardAddr = rewardsPool
	accounting.rewardsLevel = rewardsLevel
	accounting.currentRound = round
	return nil
}

func bytesAreZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

func (accounting *State) closeAccount(addr types.Address) {
	update, hasKey := accounting.AlgoUpdates[addr]

	if !hasKey {
		update = &idb.AlgoUpdate{
			Balance: 0,
			Rewards: 0,
			Closed:  true,
		}
		accounting.AlgoUpdates[addr] = update
		return
	}

	delete(accounting.AccountDataUpdates, addr)

	// If the key was there, override the rewards by setting to zero.
	// The balance will be updated with the delta as usual.
	update.Rewards = 0
	update.Closed = true
}

func (accounting *State) updateRewards(rewardAddr, acctAddr types.Address, amount types.MicroAlgos) {
	accounting.updateAlgoAndRewards(acctAddr, int64(amount), int64(amount))
	// Note: rewardAddr is also available as accounting.rewardAddr, but all of the other accounting is done
	// explicitly in AddTransaction.
	accounting.updateAlgoAndRewards(rewardAddr, -int64(amount), 0)
}

func (accounting *State) updateAlgo(addr types.Address, amount int64) {
	accounting.updateAlgoAndRewards(addr, amount, 0)
}

func (accounting *State) updateAlgoAndRewards(addr types.Address, amount, rewards int64) {
	update, hasKey := accounting.AlgoUpdates[addr]

	if !hasKey {
		update = &idb.AlgoUpdate{
			Balance: amount,
			Rewards: rewards,
		}
		accounting.AlgoUpdates[addr] = update
		return
	}

	update.Balance += amount
	update.Rewards += rewards
}

func (accounting *State) updateAccountType(addr types.Address, ktype string) {
	accounting.AccountTypes[addr] = ktype
}

func (accounting *State) updateAccountData(addr types.Address, key string, field interface{}) {
	au, ok := accounting.AccountDataUpdates[addr]
	if !ok {
		au = make(map[string]idb.AccountDataUpdate)
		accounting.AccountDataUpdates[addr] = au
	}
	au[key] = idb.AccountDataUpdate{Delete: false, Value: field}
}

func (accounting *State) removeAccountData(addr types.Address, key string) {
	au, ok := accounting.AccountDataUpdates[addr]
	if !ok {
		au = make(map[string]idb.AccountDataUpdate)
		accounting.AccountDataUpdates[addr] = au
	}
	au[key] = idb.AccountDataUpdate{Delete: true, Value: struct{}{}}
}

func (accounting *State) updateAsset(addr types.Address, assetID uint64, add, sub uint64, frozen bool) {
	// in-place optimization in case an asset is modified by multiple transactions.
	// Get update list from final subround. When a subround ends an empty subround is appended
	// so there is no need to check whether an account has closed.
	updatelist := accounting.AssetUpdates[len(accounting.AssetUpdates)-1][addr]
	for i, up := range updatelist {
		if up.Transfer != nil && up.AssetID == assetID {
			if add != 0 {
				var xa big.Int
				xa.SetUint64(add)
				up.Transfer.Delta.Add(&xa, &up.Transfer.Delta)
			}
			if sub != 0 {
				var xa big.Int
				xa.SetUint64(sub)
				up.Transfer.Delta.Sub(&up.Transfer.Delta, &xa)
			}
			updatelist[i] = up
			return
		}
	}

	au := idb.AssetUpdate{AssetID: assetID, DefaultFrozen: frozen, Transfer: &idb.AssetTransfer{}}
	if add != 0 {
		var xa big.Int
		xa.SetUint64(add)
		au.Transfer.Delta.Add(&au.Transfer.Delta, &xa)
	}
	if sub != 0 {
		var xa big.Int
		xa.SetUint64(sub)
		au.Transfer.Delta.Sub(&au.Transfer.Delta, &xa)
	}

	accounting.addAssetAccounting(addr, au, false)
}

func (accounting *State) addAssetAccounting(addr types.Address, update idb.AssetUpdate, finalizeSubround bool) {
	// Add the final subround update
	updatelist := accounting.AssetUpdates[len(accounting.AssetUpdates)-1][addr]
	accounting.AssetUpdates[len(accounting.AssetUpdates)-1][addr] = append(updatelist, update)

	// Put an empty subround for any subsequent updates.
	if finalizeSubround {
		accounting.AssetUpdates = append(accounting.AssetUpdates, make(map[[32]byte][]idb.AssetUpdate))
	}
}

func (accounting *State) configAsset(assetID uint64, isNew bool, creator types.Address, params sdk_types.AssetParams) {
	update := idb.AssetUpdate{
		AssetID:       assetID,
		DefaultFrozen: accounting.defaultFrozen[assetID],
		Config: &idb.AcfgUpdate{
			IsNew:   isNew,
			Creator: creator,
			Params:  params,
		},
	}
	// This probably doesn't need to finalize the subround, but it is an uncommon transaction so lets play it safe.
	accounting.addAssetAccounting(creator, update, true)
}

func (accounting *State) closeAsset(from types.Address, assetID uint64, to types.Address, round uint64, offset int) {
	update := idb.AssetUpdate{
		AssetID:       assetID,
		DefaultFrozen: accounting.defaultFrozen[assetID],
		Close: &idb.AssetClose{
			CloseTo: to,
			Sender:  from,
			Round:   round,
			Offset:  uint64(offset),
		},
	}
	accounting.addAssetAccounting(from, update, true)
}

func (accounting *State) freezeAsset(addr types.Address, assetID uint64, frozen bool) {
	update := idb.AssetUpdate{
		AssetID:       assetID,
		DefaultFrozen: accounting.defaultFrozen[assetID],
		Freeze:        &idb.FreezeUpdate{Frozen: frozen},
	}
	accounting.addAssetAccounting(addr, update, false)
}

func (accounting *State) destroyAsset(assetID uint64) {
	accounting.AssetDestroys = append(accounting.AssetDestroys, assetID)
}

// ErrWrongRound is returned when adding a transaction which belongs to a different round
// than what the State is configured for.
var ErrWrongRound error = errors.New("wrong round")

// AddTransaction updates the State with the provided idb.TxnRow data.
func (accounting *State) AddTransaction(txnr *idb.TxnRow) (err error) {
	round := txnr.Round
	intra := txnr.Intra
	txnbytes := txnr.TxnBytes
	var stxn types.SignedTxnWithAD
	err = msgpack.Decode(txnbytes, &stxn)
	if err != nil {
		return fmt.Errorf("txn r=%d i=%d failed decode, %v", round, intra, err)
	}
	if accounting.currentRound != round {
		return ErrWrongRound
	}

	ktype, err := idb.SignatureType(&stxn.SignedTxn)
	if err != nil {
		ktype = ""
	}

	var isNew bool
	isNew, err = accounting.accountTypes.set(stxn.Txn.Sender, string(ktype))
	if err != nil {
		return fmt.Errorf("error in account type, %v", err)
	}
	if isNew {
		accounting.updateAccountType(stxn.Txn.Sender, string(ktype))
	}

	accounting.updateAlgo(stxn.Txn.Sender, -int64(stxn.Txn.Fee))
	accounting.updateAlgo(accounting.feeAddr, int64(stxn.Txn.Fee))

	if stxn.SenderRewards != 0 {
		accounting.updateRewards(accounting.rewardAddr, stxn.Txn.Sender, stxn.SenderRewards)
	}

	if !stxn.Txn.RekeyTo.IsZero() {
		if stxn.Txn.RekeyTo == stxn.Txn.Sender {
			accounting.removeAccountData(stxn.Txn.Sender, "spend")
		} else {
			accounting.updateAccountData(stxn.Txn.Sender, "spend", stxn.Txn.RekeyTo)
		}
	}

	switch stxn.Txn.Type {
	case sdk_types.PaymentTx:
		amount := int64(stxn.Txn.Amount)
		if amount != 0 {
			accounting.updateAlgo(stxn.Txn.Sender, -amount)
			accounting.updateAlgo(stxn.Txn.Receiver, amount)
		}
		if stxn.ClosingAmount != 0 {
			accounting.updateAlgo(stxn.Txn.Sender, -int64(stxn.ClosingAmount))
			accounting.updateAlgo(stxn.Txn.CloseRemainderTo, int64(stxn.ClosingAmount))
		}
		if stxn.ReceiverRewards != 0 {
			accounting.updateRewards(accounting.rewardAddr, stxn.Txn.Receiver, stxn.ReceiverRewards)
		}
		if stxn.CloseRewards != 0 {
			accounting.updateRewards(accounting.rewardAddr, stxn.Txn.CloseRemainderTo, stxn.CloseRewards)
		}

		// The sender account is being closed.
		if AccountCloseTxn(stxn.Txn.Sender, stxn) {
			accounting.closeAccount(stxn.Txn.Sender)
		}
	case sdk_types.KeyRegistrationTx:
		// see https://github.com/algorand/go-algorand/blob/master/data/transactions/keyreg.go
		accounting.updateAccountData(stxn.Txn.Sender, "vote", stxn.Txn.VotePK)
		accounting.updateAccountData(stxn.Txn.Sender, "sel", stxn.Txn.SelectionPK)
		if bytesAreZero(stxn.Txn.VotePK[:]) || bytesAreZero(stxn.Txn.SelectionPK[:]) {
			if stxn.Txn.Nonparticipation {
				accounting.updateAccountData(stxn.Txn.Sender, "onl", 2) // NotParticipating
			} else {
				accounting.updateAccountData(stxn.Txn.Sender, "onl", 0) // Offline
			}
			accounting.updateAccountData(stxn.Txn.Sender, "voteFst", 0)
			accounting.updateAccountData(stxn.Txn.Sender, "voteLst", 0)
			accounting.updateAccountData(stxn.Txn.Sender, "voteKD", 0)
		} else {
			accounting.updateAccountData(stxn.Txn.Sender, "onl", 1) // Online
			accounting.updateAccountData(stxn.Txn.Sender, "voteFst", uint64(stxn.Txn.VoteFirst))
			accounting.updateAccountData(stxn.Txn.Sender, "voteLst", uint64(stxn.Txn.VoteLast))
			accounting.updateAccountData(stxn.Txn.Sender, "voteKD", stxn.Txn.VoteKeyDilution)
		}
	case sdk_types.AssetConfigTx:
		assetID := uint64(stxn.Txn.ConfigAsset)
		isNew := AssetCreateTxn(stxn)
		if isNew {
			assetID = txnr.AssetID
		}
		if AssetDestroyTxn(stxn) {
			accounting.destroyAsset(assetID)
		} else {
			accounting.configAsset(assetID, isNew, stxn.Txn.Sender, stxn.Txn.AssetParams)
			if stxn.Txn.ConfigAsset == 0 {
				// Only update the cache when default-frozen = true.
				// DefaultFrozen is immutable, so only update the cache during creation.
				// It is technically possible to set DefaultFrozen in a transaction, algod will ignore it so indexer
				// must as well.
				if stxn.Txn.AssetParams.DefaultFrozen {
					accounting.defaultFrozen[assetID] = stxn.Txn.AssetParams.DefaultFrozen
				}

				// Initial creation, give all initial value to creator.
				// Ignore DefaultFrozen.
				// Total = 0 is valid.
				accounting.updateAsset(stxn.Txn.Sender, assetID, stxn.Txn.AssetParams.Total, 0, false)
			}
		}
	case sdk_types.AssetTransferTx:
		assetID := uint64(stxn.Txn.XferAsset)
		defaultFrozen := accounting.defaultFrozen[assetID]
		sender := stxn.Txn.AssetSender // clawback
		if sender.IsZero() {
			sender = stxn.Txn.Sender
		}
		if stxn.Txn.AssetAmount != 0 {
			accounting.updateAsset(sender, assetID, 0, stxn.Txn.AssetAmount, defaultFrozen)
			accounting.updateAsset(stxn.Txn.AssetReceiver, assetID, stxn.Txn.AssetAmount, 0, defaultFrozen)
		}
		if AssetOptInTxn(stxn) {
			// mark receivable accounts with the send-self-zero txn
			accounting.updateAsset(stxn.Txn.AssetReceiver, assetID, 0, 0, defaultFrozen)
		}
		if AssetOptOutTxn(stxn) {
			accounting.closeAsset(sender, assetID, stxn.Txn.AssetCloseTo, round, intra)
		}
	case sdk_types.AssetFreezeTx:
		accounting.freezeAsset(stxn.Txn.FreezeAccount, uint64(stxn.Txn.FreezeAsset), stxn.Txn.AssetFrozen)
	case sdk_types.ApplicationCallTx:
		hasGlobal := (len(stxn.EvalDelta.GlobalDelta) > 0) || (len(stxn.Txn.ApprovalProgram) > 0) || (len(stxn.Txn.ClearStateProgram) > 0) || stxn.Txn.OnCompletion == sdk_types.DeleteApplicationOC
		appid := uint64(stxn.Txn.ApplicationID)
		if appid == 0 {
			// creation
			appid = txnr.AssetID
		}
		if hasGlobal {
			agd := idb.AppDelta{
				AppIndex: int64(appid),
				Round:    round,
				Intra:    intra,
				//Address:           nil,
				Delta:             stxn.EvalDelta.GlobalDelta,
				OnCompletion:      stxn.Txn.OnCompletion,
				ApprovalProgram:   stxn.Txn.ApprovalProgram,
				ClearStateProgram: stxn.Txn.ClearStateProgram,
				LocalStateSchema:  stxn.Txn.LocalStateSchema,
				GlobalStateSchema: stxn.Txn.GlobalStateSchema,
				ExtraProgramPages: stxn.Txn.ExtraProgramPages,
			}
			if stxn.Txn.ApplicationID == 0 {
				// app creation
				agd.Creator = stxn.Txn.Sender[:]
			}
			//fmt.Printf("agset %d %s\n", appid, agd.String())
			accounting.AppGlobalDeltas = append(
				accounting.AppGlobalDeltas,
				agd,
			)
		}
		for accountIndex, ldelts := range stxn.EvalDelta.LocalDeltas {
			var addr []byte
			if accountIndex == 0 {
				addr = stxn.Txn.Sender[:]
			} else {
				addr = stxn.Txn.Accounts[accountIndex-1][:]
			}
			accounting.AppLocalDeltas = append(
				accounting.AppLocalDeltas,
				idb.AppDelta{
					AppIndex:     int64(appid),
					Round:        round,
					Intra:        intra,
					Address:      addr,
					AddrIndex:    accountIndex,
					Delta:        ldelts,
					OnCompletion: stxn.Txn.OnCompletion,
				},
			)
		}
		// if there's no other content change, but a state change of opt-in/close-out/clear-state, record that
		if len(stxn.EvalDelta.LocalDeltas) == 0 && (stxn.Txn.OnCompletion == sdk_types.OptInOC || stxn.Txn.OnCompletion == sdk_types.CloseOutOC || stxn.Txn.OnCompletion == sdk_types.ClearStateOC) {
			accounting.AppLocalDeltas = append(
				accounting.AppLocalDeltas,
				idb.AppDelta{
					AppIndex:     int64(appid),
					Address:      stxn.Txn.Sender[:],
					Round:        round,
					Intra:        intra,
					OnCompletion: stxn.Txn.OnCompletion,
				},
			)
		}
	default:
		return fmt.Errorf("txn r=%d i=%d UNKNOWN TYPE %#v", round, intra, stxn.Txn.Type)
	}
	return nil
}
