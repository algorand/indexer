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
	"bytes"
	"fmt"

	//"github.com/algorand/go-algorand-sdk/encoding/json"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

type acfgUpdate struct {
	assetId uint64
	creator types.Address
	params  types.AssetParams
}

type assetUpdate struct {
	addr    types.Address
	assetId uint64
	delta   int64
}

type freezeUpdate struct {
	addr    types.Address
	assetId uint64
	frozen  bool
}

type assetClose struct {
	closeTo types.Address
	assetId uint64
	sender  types.Address
}

type AccountingState struct {
	db idb.IndexerDb

	defaultFrozen map[uint64]bool

	currentRound uint64

	algoUpdates   map[[32]byte]int64
	acfgUpdates   []acfgUpdate
	assetUpdates  []assetUpdate
	freezeUpdates []freezeUpdate
	assetCloses   []assetClose

	feeAddr    types.Address
	rewardAddr types.Address

	// number of txns at the end of the previous block
	txnCounter uint64
}

func New(db idb.IndexerDb) *AccountingState {
	return &AccountingState{db: db}
}

func foo(db idb.IndexerDb) {
	//var defaultFrozen map[uint64]bool
}

func (accounting *AccountingState) commitRound() error {
	return nil
}

func (accounting *AccountingState) Close() error {
	// TODO: commit any pending round
	return nil
}

var zeroAddr = [32]byte{}

func addrIsZero(a types.Address) bool {
	return bytes.Equal(a[:], zeroAddr[:])
}

func (accounting *AccountingState) AddTransaction(round uint64, intra int, txnbytes []byte) (err error) {
	var stxn types.SignedTxnInBlock
	err = msgpack.Decode(txnbytes, &stxn)
	if err != nil {
		return fmt.Errorf("txn r=%d i=%d failed decode, %v\n", round, intra, err)
	}
	if accounting.currentRound != round {
		err = accounting.commitRound()
		if err != nil {
			return
		}
	}

	accounting.algoUpdates[stxn.Txn.Sender] = accounting.algoUpdates[stxn.Txn.Sender] - int64(stxn.Txn.Fee)
	accounting.algoUpdates[accounting.feeAddr] = accounting.algoUpdates[accounting.feeAddr] + int64(stxn.Txn.Fee)

	if stxn.SenderRewards != 0 {
		accounting.algoUpdates[stxn.Txn.Sender] = accounting.algoUpdates[stxn.Txn.Sender] + int64(stxn.SenderRewards)
		accounting.algoUpdates[accounting.rewardAddr] = accounting.algoUpdates[accounting.rewardAddr] - int64(stxn.SenderRewards)
	}

	if stxn.Txn.Type == "pay" {
		amount := int64(stxn.Txn.Amount)
		if amount != 0 {
			accounting.algoUpdates[stxn.Txn.Sender] = accounting.algoUpdates[stxn.Txn.Sender] - amount
			accounting.algoUpdates[stxn.Txn.Receiver] = accounting.algoUpdates[stxn.Txn.Receiver] + amount
		}
		if stxn.ClosingAmount != 0 {
			accounting.algoUpdates[stxn.Txn.Sender] = accounting.algoUpdates[stxn.Txn.Sender] - int64(stxn.ClosingAmount)
			accounting.algoUpdates[stxn.Txn.CloseRemainderTo] = accounting.algoUpdates[stxn.Txn.CloseRemainderTo] + int64(stxn.ClosingAmount)
		}
		if stxn.ReceiverRewards != 0 {
			accounting.algoUpdates[stxn.Txn.Receiver] = accounting.algoUpdates[stxn.Txn.Receiver] + int64(stxn.ReceiverRewards)
			accounting.algoUpdates[accounting.rewardAddr] = accounting.algoUpdates[accounting.rewardAddr] - int64(stxn.ReceiverRewards)
		}
		if stxn.CloseRewards != 0 {
			accounting.algoUpdates[stxn.Txn.CloseRemainderTo] = accounting.algoUpdates[stxn.Txn.CloseRemainderTo] + int64(stxn.CloseRewards)
			accounting.algoUpdates[accounting.rewardAddr] = accounting.algoUpdates[accounting.rewardAddr] - int64(stxn.CloseRewards)
		}
	} else if stxn.Txn.Type == "keyreg" {
		// TODO: record keys?
	} else if stxn.Txn.Type == "acfg" {
		assetId := uint64(stxn.Txn.ConfigAsset)
		if assetId == 0 {
			assetId = accounting.txnCounter + uint64(intra) + 1
		}
	} else if stxn.Txn.Type == "axfer" {
	} else if stxn.Txn.Type == "afrz" {
	} else {
		return fmt.Errorf("txn r=%d i=%d UNKNOWN TYPE %#v\n", round, intra, stxn.Txn.Type)
	}
	return nil
}
