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

package accounting

import (
	"context"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	atypes "github.com/algorand/go-algorand-sdk/types"
	models "github.com/algorand/indexer/api/generated"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

func assetUpdate(account *models.Account, assetid uint64, add, sub uint64) {
	if account.Assets == nil {
		account.Assets = new([]models.AssetHolding)
	}
	assets := *account.Assets
	for i, ah := range assets {
		if ah.AssetId == assetid {
			ah.Amount += add
			ah.Amount -= sub
			assets[i] = ah
			return
		}
	}
	assets = append(assets, models.AssetHolding{
		Amount:  add - sub,
		AssetId: assetid,
		//Creator: base32 addr string of asset creator, TODO
		//IsFrozen: leave nil? // TODO: on close record frozen state for rewind
	})
	*account.Assets = assets
}

func AccountAtRound(account models.Account, round uint64, db idb.IndexerDb) (acct models.Account, err error) {
	acct = account
	addr, err := atypes.DecodeAddress(account.Address)
	if err != nil {
		return
	}
	tf := idb.TransactionFilter{
		Address:     addr[:],
		MinRound:    round + 1,
		MaxRound:    account.Round,
	}
	txns := db.Transactions(context.Background(), tf)
	txcount := 0
	for txnrow := range txns {
		if txnrow.Error != nil {
			err = txnrow.Error
			return
		}
		txcount++
		var stxn types.SignedTxnWithAD
		err = msgpack.Decode(txnrow.TxnBytes, &stxn)
		if err != nil {
			return
		}
		if addr == stxn.Txn.Sender {
			acct.AmountWithoutPendingRewards += uint64(stxn.Txn.Fee)
			acct.AmountWithoutPendingRewards -= uint64(stxn.SenderRewards)
		}
		switch stxn.Txn.Type {
		case atypes.PaymentTx:
			if addr == stxn.Txn.Sender {
				acct.AmountWithoutPendingRewards += uint64(stxn.Txn.Amount)
			}
			if addr == stxn.Txn.Receiver {
				acct.AmountWithoutPendingRewards -= uint64(stxn.Txn.Amount)
				acct.AmountWithoutPendingRewards -= uint64(stxn.ReceiverRewards)
			}
			if addr == stxn.Txn.CloseRemainderTo {
				acct.AmountWithoutPendingRewards += uint64(stxn.ClosingAmount)
				acct.AmountWithoutPendingRewards -= uint64(stxn.CloseRewards)
			}
		case atypes.KeyRegistrationTx:
			// TODO: keyreg does not rewind. workaround: query for txns on an account with typeenum=2 to find previous values it was set to.
		case atypes.AssetConfigTx:
			if stxn.Txn.ConfigAsset == 0 {
				// create asset, unwind the application of the value
				var block types.Block
				block, err = db.GetBlock(round - 1)
				if err != nil {
					return
				}
				assetId := block.TxnCounter + uint64(txnrow.Intra) + 1
				assetUpdate(&acct, assetId, 0, stxn.Txn.AssetParams.Total)
			}
		case atypes.AssetTransferTx:
			if addr == stxn.Txn.AssetSender || addr == stxn.Txn.Sender {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), stxn.Txn.AssetAmount+txnrow.Extra.AssetCloseAmount, 0)
			}
			if addr == stxn.Txn.AssetReceiver {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), 0, stxn.Txn.AssetAmount)
			}
			if addr == stxn.Txn.AssetCloseTo {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), 0, txnrow.Extra.AssetCloseAmount)
			}
		case atypes.AssetFreezeTx:
		default:
			panic("unknown txn type")
		}
	}

	if txcount > 0 {
		// If we found any txns above, we need to find one
		// more so we can know what the previous RewardsBase
		// of the account was so we can get the accurate
		// pending rewards at the target round.
		//
		// (If there weren't any txns above, the recorded
		// RewardsBase is current from whatever previous txn
		// happened to this account.)

		tf.MaxRound = round
		tf.MinRound = 0
		tf.Limit = 1
		txns = db.Transactions(context.Background(), tf)
		for txnrow := range txns {
			if txnrow.Error != nil {
				err = txnrow.Error
				return
			}
			var stxn types.SignedTxnWithAD
			err = msgpack.Decode(txnrow.TxnBytes, &stxn)
			if err != nil {
				return
			}
			var baseBlock types.Block
			baseBlock, err = db.GetBlock(txnrow.Round)
			if err != nil {
				return
			}
			prevRewardsBase := baseBlock.RewardsLevel
			var blockheader types.Block
			blockheader, err = db.GetBlock(round)
			if err != nil {
				return
			}
			var proto types.ConsensusParams
			proto, err = db.GetProto(string(blockheader.CurrentProtocol))
			if err != nil {
				return
			}
			rewardsUnits := acct.AmountWithoutPendingRewards / proto.RewardUnit
			rewardsDelta := blockheader.RewardsLevel - prevRewardsBase
			acct.PendingRewards = rewardsDelta * rewardsUnits
			acct.Amount = acct.PendingRewards + acct.AmountWithoutPendingRewards
			acct.Round = round
			return
		}

		// There were no prior transactions, it must have been empty before, zero out things
		acct.PendingRewards = 0
		acct.Amount = acct.AmountWithoutPendingRewards
	}

	acct.Round = round
	return
}
