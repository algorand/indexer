package accounting

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"

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

	// number of txns at the end of the previous block
	txnCounter      uint64
	txnCounterRound uint64

	accountTypes accountTypeCache
}

// New creates a new State object.
func New() *State {
	return &State{defaultFrozen: make(map[uint64]bool)}
}

// InitRound should be called before each round to initialize the accounting state.
func (accounting *State) InitRound(block types.Block) error {
	accounting.RoundUpdates.Clear()
	accounting.feeAddr = block.FeeSink
	accounting.rewardAddr = block.RewardsPool
	accounting.rewardsLevel = block.RewardsLevel
	accounting.currentRound = uint64(block.Round)
	return nil
}

var zeroAddr = [32]byte{}

func addrIsZero(a types.Address) bool {
	return bytes.Equal(a[:], zeroAddr[:])
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
	if accounting.AlgoUpdates == nil {
		accounting.AlgoUpdates = make(map[[32]byte]*idb.AlgoUpdate)
	}

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

	if accounting.AccountDataUpdates != nil {
		delete(accounting.AccountDataUpdates, addr)
	}

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
	if accounting.AlgoUpdates == nil {
		accounting.AlgoUpdates = make(map[[32]byte]*idb.AlgoUpdate)
	}

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
	if accounting.AccountTypes == nil {
		accounting.AccountTypes = make(map[[32]byte]string)
	}
	accounting.AccountTypes[addr] = ktype
}

func (accounting *State) updateAccountData(addr types.Address, key string, field interface{}) {
	if accounting.AccountDataUpdates == nil {
		accounting.AccountDataUpdates = make(map[[32]byte]map[string]interface{})
	}
	au, ok := accounting.AccountDataUpdates[addr]
	if !ok {
		au = make(map[string]interface{})
		accounting.AccountDataUpdates[addr] = au
	}
	au[key] = field
}

func (accounting *State) updateAsset(addr types.Address, assetID uint64, add, sub uint64) {
	// Get update list from final subround. When a subround ends an empty subround is appended
	// so there is no need to check whether an account has closed.
	updatelist := accounting.AssetUpdates[len(accounting.AssetUpdates)-1][addr]
	for i, up := range updatelist {
		if up.AssetID == assetID {
			if add != 0 {
				var xa big.Int
				xa.SetUint64(add)
				up.Delta.Add(&xa, &up.Delta)
			}
			if sub != 0 {
				var xa big.Int
				xa.SetUint64(sub)
				up.Delta.Sub(&up.Delta, &xa)
			}
			updatelist[i] = up
			return
		}
	}

	au := idb.AssetUpdate{AssetID: assetID, DefaultFrozen: accounting.defaultFrozen[assetID]}
	if add != 0 {
		var xa big.Int
		xa.SetUint64(add)
		au.Delta.Add(&au.Delta, &xa)
	}
	if sub != 0 {
		var xa big.Int
		xa.SetUint64(sub)
		au.Delta.Sub(&au.Delta, &xa)
	}

	// Add the AssetUpdate to the final subround
	accounting.AssetUpdates[len(accounting.AssetUpdates)-1][addr] = append(updatelist, au)
}

func (accounting *State) updateTxnAsset(round uint64, intra int, assetID uint64) {
	accounting.TxnAssetUpdates = append(accounting.TxnAssetUpdates, idb.TxnAssetUpdate{Round: round, Offset: intra, AssetID: assetID})
}

func (accounting *State) closeAsset(from types.Address, assetID uint64, to types.Address, round uint64, offset int) {
	updatelist := accounting.AssetUpdates[len(accounting.AssetUpdates)-1][from]

	assetClose := &idb.AssetClose{
			CloseTo:       to,
			AssetID:       assetID,
			Sender:        from,
			DefaultFrozen: accounting.defaultFrozen[assetID],
			Round:         round,
			Offset:        uint64(offset),
	}

	// Add an empty AssetUpdate with asset close reference.
	accounting.AssetUpdates[len(accounting.AssetUpdates)-1][from] = append(updatelist, idb.AssetUpdate{
		AssetID:       assetID,
		Delta:         *big.NewInt(0),
		DefaultFrozen: assetClose.DefaultFrozen,
		Closed:        assetClose,
	})

	// Put an empty subround for any subsequent updates.
	accounting.AssetUpdates = append(accounting.AssetUpdates, make(map[[32]byte][]idb.AssetUpdate))
}
func (accounting *State) freezeAsset(addr types.Address, assetID uint64, frozen bool) {
	accounting.FreezeUpdates = append(accounting.FreezeUpdates, idb.FreezeUpdate{Addr: addr, AssetID: assetID, Frozen: frozen})
}

func (accounting *State) destroyAsset(assetID uint64) {
	accounting.AssetDestroys = append(accounting.AssetDestroys, assetID)
}

// TODO: move to go-algorand-sdk as Signature.IsZero()
func zeroSig(sig atypes.Signature) bool {
	for _, b := range sig {
		if b != 0 {
			return false
		}
	}
	return true
}

// TODO: move to go-algorand-sdk as LogicSig.Blank()
func blankLsig(lsig atypes.LogicSig) bool {
	return len(lsig.Logic) == 0
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

	var ktype string
	if !zeroSig(stxn.Sig) {
		ktype = "sig"
	} else if !stxn.Msig.Blank() {
		ktype = "msig"
	} else if !blankLsig(stxn.Lsig) {
		if !zeroSig(stxn.Lsig.Sig) {
			ktype = "sig"
		} else if !stxn.Lsig.Msig.Blank() {
			ktype = "msig"
		} else {
			ktype = "lsig"
		}
	}
	var isNew bool
	isNew, err = accounting.accountTypes.set(stxn.Txn.Sender, ktype)
	if err != nil {
		return fmt.Errorf("error in account type, %v", err)
	}
	if isNew {
		accounting.updateAccountType(stxn.Txn.Sender, ktype)
	}

	accounting.updateAlgo(stxn.Txn.Sender, -int64(stxn.Txn.Fee))
	accounting.updateAlgo(accounting.feeAddr, int64(stxn.Txn.Fee))

	if stxn.SenderRewards != 0 {
		accounting.updateRewards(accounting.rewardAddr, stxn.Txn.Sender, stxn.SenderRewards)
	}

	if !stxn.Txn.RekeyTo.IsZero() {
		accounting.updateAccountData(stxn.Txn.Sender, "spend", stxn.Txn.RekeyTo)
	}

	switch stxn.Txn.Type {
	case atypes.PaymentTx:
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
	case atypes.KeyRegistrationTx:
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
	case atypes.AssetConfigTx:
		assetID := uint64(stxn.Txn.ConfigAsset)
		isNew := AssetCreateTxn(stxn)
		if isNew {
			assetID = txnr.AssetID
		}
		if AssetDestroyTxn(stxn) {
			accounting.destroyAsset(assetID)
		} else {
			accounting.AcfgUpdates = append(accounting.AcfgUpdates, idb.AcfgUpdate{AssetID: assetID, IsNew: isNew, Creator: stxn.Txn.Sender, Params: stxn.Txn.AssetParams})
			accounting.defaultFrozen[assetID] = stxn.Txn.AssetParams.DefaultFrozen
			if stxn.Txn.ConfigAsset == 0 {
				// initial creation, give all initial value to creator
				if stxn.Txn.AssetParams.Total != 0 {
					accounting.updateAsset(stxn.Txn.Sender, assetID, stxn.Txn.AssetParams.Total, 0)
				}
			}
		}
	case atypes.AssetTransferTx:
		sender := stxn.Txn.AssetSender // clawback
		if sender.IsZero() {
			sender = stxn.Txn.Sender
		}
		if stxn.Txn.AssetAmount != 0 {
			accounting.updateAsset(sender, uint64(stxn.Txn.XferAsset), 0, stxn.Txn.AssetAmount)
			accounting.updateAsset(stxn.Txn.AssetReceiver, uint64(stxn.Txn.XferAsset), stxn.Txn.AssetAmount, 0)
		}
		if AssetOptInTxn(stxn) {
			// mark receivable accounts with the send-self-zero txn
			accounting.updateAsset(stxn.Txn.AssetReceiver, uint64(stxn.Txn.XferAsset), 0, 0)
		}
		if AssetOptOutTxn(stxn) {
			accounting.closeAsset(sender, uint64(stxn.Txn.XferAsset), stxn.Txn.AssetCloseTo, round, intra)
		}
	case atypes.AssetFreezeTx:
		accounting.freezeAsset(stxn.Txn.FreezeAccount, uint64(stxn.Txn.FreezeAsset), stxn.Txn.AssetFrozen)
	case atypes.ApplicationCallTx:
		hasGlobal := (len(stxn.EvalDelta.GlobalDelta) > 0) || (len(stxn.Txn.ApprovalProgram) > 0) || (len(stxn.Txn.ClearStateProgram) > 0) || stxn.Txn.OnCompletion == atypes.DeleteApplicationOC
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
		if len(stxn.EvalDelta.LocalDeltas) == 0 && (stxn.Txn.OnCompletion == atypes.OptInOC || stxn.Txn.OnCompletion == atypes.CloseOutOC || stxn.Txn.OnCompletion == atypes.ClearStateOC) {
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
