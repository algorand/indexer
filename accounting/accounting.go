package accounting

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

type AccountingState struct {
	db idb.IndexerDb

	defaultFrozen map[uint64]bool

	currentRound uint64
	dirty        bool

	idb.RoundUpdates

	feeAddr    types.Address
	rewardAddr types.Address

	rewardsLevel uint64

	// number of txns at the end of the previous block
	txnCounter      uint64
	txnCounterRound uint64

	accountTypes accountTypeCache
}

func New(db idb.IndexerDb) *AccountingState {
	return &AccountingState{db: db, defaultFrozen: make(map[uint64]bool)}
}

func (accounting *AccountingState) getTxnCounter(round uint64) (txnCounter uint64, err error) {
	if round != accounting.txnCounterRound {
		block, err := accounting.db.GetBlock(round)
		if err != nil {
			return 0, err
		}
		accounting.txnCounter = block.TxnCounter
		accounting.txnCounterRound = round
	}
	return accounting.txnCounter, nil
}

func (accounting *AccountingState) initRound(round uint64) error {
	block, err := accounting.db.GetBlock(round)
	if err != nil {
		return err
	}
	accounting.feeAddr = block.FeeSink
	accounting.rewardAddr = block.RewardsPool
	accounting.rewardsLevel = block.RewardsLevel
	accounting.currentRound = round
	return nil
}

func (accounting *AccountingState) commitRound() error {
	if !accounting.dirty {
		return nil
	}
	err := accounting.db.CommitRoundAccounting(accounting.RoundUpdates, accounting.currentRound, accounting.rewardsLevel)
	if err != nil {
		return err
	}
	accounting.RoundUpdates.Clear()
	accounting.dirty = false
	return nil
}

func (accounting *AccountingState) Close() error {
	return accounting.commitRound()
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

func (accounting *AccountingState) updateAlgo(addr types.Address, d int64) {
	if accounting.AlgoUpdates == nil {
		accounting.AlgoUpdates = make(map[[32]byte]int64)
	}
	accounting.AlgoUpdates[addr] = accounting.AlgoUpdates[addr] + d
}

func (accounting *AccountingState) updateAccountType(addr types.Address, ktype string) {
	if accounting.AccountTypes == nil {
		accounting.AccountTypes = make(map[[32]byte]string)
	}
	accounting.AccountTypes[addr] = ktype
}

func (accounting *AccountingState) updateAccountData(addr types.Address, key string, field interface{}) {
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

func (accounting *AccountingState) updateAsset(addr types.Address, assetId uint64, add, sub uint64) {
	updatelist := accounting.AssetUpdates[addr]
	for i, up := range updatelist {
		if up.AssetId == assetId {
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
	if accounting.AssetUpdates == nil {
		accounting.AssetUpdates = make(map[[32]byte][]idb.AssetUpdate)
	}

	au := idb.AssetUpdate{AssetId: assetId, DefaultFrozen: accounting.defaultFrozen[assetId]}
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
	accounting.AssetUpdates[addr] = append(updatelist, au)
}

func (accounting *AccountingState) updateTxnAsset(round uint64, intra int, assetId uint64) {
	accounting.TxnAssetUpdates = append(accounting.TxnAssetUpdates, idb.TxnAssetUpdate{Round: round, Offset: intra, AssetId: assetId})
}

func (accounting *AccountingState) closeAsset(from types.Address, assetId uint64, to types.Address, round uint64, offset int) {
	accounting.AssetCloses = append(
		accounting.AssetCloses,
		idb.AssetClose{
			CloseTo:       to,
			AssetId:       assetId,
			Sender:        from,
			DefaultFrozen: accounting.defaultFrozen[assetId],
			Round:         round,
			Offset:        uint64(offset),
		},
	)
}
func (accounting *AccountingState) freezeAsset(addr types.Address, assetId uint64, frozen bool) {
	accounting.FreezeUpdates = append(accounting.FreezeUpdates, idb.FreezeUpdate{Addr: addr, AssetId: assetId, Frozen: frozen})
}
func (accounting *AccountingState) destroyAsset(assetId uint64) {
	accounting.AssetDestroys = append(accounting.AssetDestroys, assetId)
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

func (accounting *AccountingState) AddTransaction(txnr *idb.TxnRow) (err error) {
	round := txnr.Round
	intra := txnr.Intra
	txnbytes := txnr.TxnBytes
	var stxn types.SignedTxnWithAD
	err = msgpack.Decode(txnbytes, &stxn)
	if err != nil {
		return fmt.Errorf("txn r=%d i=%d failed decode, %v\n", round, intra, err)
	}
	if accounting.currentRound != round {
		err = accounting.commitRound()
		if err != nil {
			return fmt.Errorf("add tx commit round %d, %v", accounting.currentRound, err)
		}
		err = accounting.initRound(round)
		if err != nil {
			return fmt.Errorf("add tx init round %d, %v", round, err)
		}
	}
	accounting.dirty = true

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
		accounting.updateAlgo(stxn.Txn.Sender, int64(stxn.SenderRewards))
		accounting.updateAlgo(accounting.rewardAddr, -int64(stxn.SenderRewards))
	}

	if !stxn.Txn.RekeyTo.IsZero() {
		accounting.updateAccountData(stxn.Txn.Sender, "spend", stxn.Txn.RekeyTo)
	}

	if stxn.Txn.Type == "pay" {
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
			accounting.updateAlgo(stxn.Txn.Receiver, int64(stxn.ReceiverRewards))
			accounting.updateAlgo(accounting.rewardAddr, -int64(stxn.ReceiverRewards))
		}
		if stxn.CloseRewards != 0 {
			accounting.updateAlgo(stxn.Txn.CloseRemainderTo, int64(stxn.CloseRewards))
			accounting.updateAlgo(accounting.rewardAddr, -int64(stxn.CloseRewards))
		}
	} else if stxn.Txn.Type == "keyreg" {
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
	} else if stxn.Txn.Type == "acfg" {
		assetId := uint64(stxn.Txn.ConfigAsset)
		if assetId == 0 {
			assetId = txnr.AssetId
		}
		if stxn.Txn.AssetParams.IsZero() {
			accounting.destroyAsset(assetId)
		} else {
			accounting.AcfgUpdates = append(accounting.AcfgUpdates, idb.AcfgUpdate{AssetId: assetId, Creator: stxn.Txn.Sender, Params: stxn.Txn.AssetParams})
			accounting.defaultFrozen[assetId] = stxn.Txn.AssetParams.DefaultFrozen
			if stxn.Txn.ConfigAsset == 0 {
				// initial creation, give all initial value to creator
				if stxn.Txn.AssetParams.Total != 0 {
					accounting.updateAsset(stxn.Txn.Sender, assetId, stxn.Txn.AssetParams.Total, 0)
				}
			}
		}
	} else if stxn.Txn.Type == "axfer" {
		sender := stxn.Txn.AssetSender // clawback
		if sender.IsZero() {
			sender = stxn.Txn.Sender
		}
		if stxn.Txn.AssetAmount != 0 {
			accounting.updateAsset(sender, uint64(stxn.Txn.XferAsset), 0, stxn.Txn.AssetAmount)
			accounting.updateAsset(stxn.Txn.AssetReceiver, uint64(stxn.Txn.XferAsset), stxn.Txn.AssetAmount, 0)
		} else if stxn.Txn.Sender == stxn.Txn.AssetReceiver {
			// mark receivable accounts with the send-self-zero txn
			accounting.updateAsset(stxn.Txn.AssetReceiver, uint64(stxn.Txn.XferAsset), 0, 0)
		}
		if !stxn.Txn.AssetCloseTo.IsZero() {
			accounting.closeAsset(sender, uint64(stxn.Txn.XferAsset), stxn.Txn.AssetCloseTo, round, intra)
		}
	} else if stxn.Txn.Type == "afrz" {
		accounting.freezeAsset(stxn.Txn.FreezeAccount, uint64(stxn.Txn.FreezeAsset), stxn.Txn.AssetFrozen)
	} else if stxn.Txn.Type == "appl" {
		hasGlobal := (len(stxn.EvalDelta.GlobalDelta) > 0) || (len(stxn.Txn.ApprovalProgram) > 0) || (len(stxn.Txn.ClearStateProgram) > 0)
		appid := uint64(stxn.Txn.ApplicationID)
		if appid == 0 {
			// creation
			appid = txnr.AssetId
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
			fmt.Printf("agset %d %s\n", appid, agd.String())
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
	} else {
		return fmt.Errorf("txn r=%d i=%d UNKNOWN TYPE %#v\n", round, intra, stxn.Txn.Type)
	}
	return nil
}
