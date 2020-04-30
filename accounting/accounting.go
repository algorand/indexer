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
	accounting.AlgoUpdates = nil
	accounting.AccountTypes = nil
	accounting.KeyregUpdates = nil
	accounting.AssetUpdates = nil
	accounting.AcfgUpdates = nil
	accounting.TxnAssetUpdates = nil
	accounting.FreezeUpdates = nil
	accounting.AssetCloses = nil
	accounting.AssetDestroys = nil
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

func (accounting *AccountingState) AddTransaction(round uint64, intra int, txnbytes []byte) (err error) {
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
		kru := idb.KeyregUpdate{
			Addr:            stxn.Txn.Sender,
			VoteID:          stxn.Txn.VotePK,
			SelectionID:     stxn.Txn.SelectionPK,
			VoteFirstValid:  uint64(stxn.Txn.VoteFirst),
			VoteLastValid:   uint64(stxn.Txn.VoteLast),
			VoteKeyDilution: stxn.Txn.VoteKeyDilution,
		}
		if bytesAreZero(stxn.Txn.VotePK[:]) || bytesAreZero(stxn.Txn.SelectionPK[:]) {
			if stxn.Txn.Nonparticipation {
				kru.Status = 2
			} else {
				kru.Status = 0
			}
			kru.VoteFirstValid = 0
			kru.VoteLastValid = 0
			kru.VoteKeyDilution = 0
		} else {
			kru.Status = 1 // online
		}
		accounting.KeyregUpdates = append(accounting.KeyregUpdates, kru)
	} else if stxn.Txn.Type == "acfg" {
		assetId := uint64(stxn.Txn.ConfigAsset)
		if assetId == 0 {
			// create an asset
			txnCounter, err := accounting.getTxnCounter(round - 1)
			if err != nil {
				return fmt.Errorf("acfg get TxnCounter round %d, %v", round, err)
			}
			assetId = txnCounter + uint64(intra) + 1
			accounting.updateTxnAsset(round, intra, assetId)
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
		}
		// TODO: mark receivable accounts with the send-self-zero txn?
		if !stxn.Txn.AssetCloseTo.IsZero() {
			accounting.closeAsset(sender, uint64(stxn.Txn.XferAsset), stxn.Txn.AssetCloseTo, round, intra)
		}
	} else if stxn.Txn.Type == "afrz" {
		accounting.freezeAsset(stxn.Txn.FreezeAccount, uint64(stxn.Txn.FreezeAsset), stxn.Txn.AssetFrozen)
	} else {
		return fmt.Errorf("txn r=%d i=%d UNKNOWN TYPE %#v\n", round, intra, stxn.Txn.Type)
	}
	return nil
}
