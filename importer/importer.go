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

package importer

import (
	"bytes"
	"fmt"

	idb "github.com/algorand/indexer/db"
	"github.com/algorand/indexer/types"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
)

type Importer interface {
	ImportBlock(blockbytes []byte) error
}

type printImporter struct {
}

func (imp *printImporter) ImportBlock(blockbytes []byte) (err error) {
	var blockContainer map[string]interface{}
	err = msgpack.Decode(blockbytes, &blockContainer)
	if err != nil {
		return fmt.Errorf("error decoding blockbytes, %v", err)
	}
	block := blockContainer["block"].(map[interface{}]interface{})
	txnsi, haveTxns := block["txns"]
	numTxns := 0
	if haveTxns {
		txns := txnsi.([]interface{})
		numTxns = len(txns)
		/*
			for intra, txni := range txns {
				txn := txni.(map[interface{}]interface{})
			}*/
	}
	blockHeader := block
	delete(blockHeader, "txns")
	blockheaderBytes := msgpack.Encode(blockHeader)
	fmt.Printf("%d block header bytes. %d txns\n", len(blockheaderBytes), numTxns)
	//fmt.Printf("blockbytes decoded %#v\n", block)
	return nil
}

func NewPrintImporter() Importer {
	return &printImporter{}
}

type dbImporter struct {
	db idb.IndexerDb
}

type stringInt struct {
	s string
	i int
}

var typeEnumList = []stringInt{
	{"pay", 1},
	{"keyreg", 2},
	{"acfg", 3},
	{"axfer", 4},
	{"afrz", 5},
}
var typeEnumMap map[string]int

func init() {
	typeEnumMap = make(map[string]int, len(typeEnumList))
	for _, si := range typeEnumList {
		typeEnumMap[si.s] = si.i
	}
}

var zeroAddr = [32]byte{}

func participate(participants [][]byte, addr []byte) [][]byte {
	if !bytes.Equal(addr, zeroAddr[:]) {
		return append(participants, addr)
	}
	return participants
}

func (imp *dbImporter) ImportBlock(blockbytes []byte) (err error) {
	var blockContainer types.EncodedBlockCert
	err = msgpack.Decode(blockbytes, &blockContainer)
	if err != nil {
		return fmt.Errorf("error decoding blockbytes, %v", err)
	}
	err = imp.db.StartBlock()
	if err != nil {
		return fmt.Errorf("error starting block, %v", err)
	}
	block := blockContainer.Block
	round := uint64(block.Round)
	for intra, stxn := range block.Payset {
		txtype := stxn.Txn.Type
		txtypeenum := typeEnumMap[txtype]
		assetid := uint64(0)
		switch txtypeenum {
		case 3:
			assetid = uint64(stxn.Txn.ConfigAsset)
		case 4:
			assetid = uint64(stxn.Txn.XferAsset)
		case 5:
			assetid = uint64(stxn.Txn.FreezeAsset)
		}
		txnbytes := msgpack.Encode(stxn)
		participants := make([][]byte, 0, 10)
		participants = participate(participants, stxn.Txn.Sender[:])
		participants = participate(participants, stxn.Txn.Receiver[:])
		participants = participate(participants, stxn.Txn.CloseRemainderTo[:])
		participants = participate(participants, stxn.Txn.AssetSender[:])
		participants = participate(participants, stxn.Txn.AssetReceiver[:])
		participants = participate(participants, stxn.Txn.AssetCloseTo[:])
		err = imp.db.AddTransaction(round, intra, txtypeenum, assetid, txnbytes, stxn, participants)
		if err != nil {
			return fmt.Errorf("error importing txn r=%d i=%d, %v", round, intra, err)
		}
	}
	blockHeader := block
	blockHeader.Payset = nil
	blockheaderBytes := msgpack.Encode(blockHeader)
	err = imp.db.CommitBlock(round, block.TimeStamp, blockheaderBytes)
	if err != nil {
		return fmt.Errorf("error committing block, %v", err)
	}
	return nil
}

func NewDBImporter(db idb.IndexerDb) Importer {
	return &dbImporter{db: db}
}
