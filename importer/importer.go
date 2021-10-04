package importer

import (
	"bytes"
	"fmt"

	"github.com/algorand/go-algorand-sdk/encoding/msgpack"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/types"
)

// Importer is used to import blocks into an idb.IndexerDb object.
type Importer interface {
	ImportBlock(blockbytes []byte) (txCount int, err error)
	ImportDecodedBlock(block *types.EncodedBlockCert) (txCount int, err error)
}

type dbImporter struct {
	db idb.IndexerDb
}

var zeroAddr = [32]byte{}

func participate(participants [][]byte, addr []byte) [][]byte {
	if len(addr) == 0 || bytes.Equal(addr, zeroAddr[:]) {
		return participants
	}
	for _, pp := range participants {
		if bytes.Equal(pp, addr) {
			return participants
		}
	}
	return append(participants, addr)
}

// ImportBlock processes a block and adds it to the IndexerDb
func (imp *dbImporter) ImportBlock(blockbytes []byte) (txCount int, err error) {
	var blockContainer types.EncodedBlockCert
	err = msgpack.Decode(blockbytes, &blockContainer)
	if err != nil {
		return txCount, fmt.Errorf("error decoding blockbytes, %v", err)
	}
	return imp.ImportDecodedBlock(&blockContainer)
}

func countInnerAndAddParticipation(stxns []types.SignedTxnWithAD, participants [][]byte) (int, [][]byte) {
	copyAddr := func(addr types.Address) []byte {
		return append([]byte(nil), addr[:]...)
	}
	num := 0
	for _, stxn := range stxns {
		participants = participate(participants, copyAddr(stxn.Txn.Sender))
		participants = participate(participants, copyAddr(stxn.Txn.Receiver))
		participants = participate(participants, copyAddr(stxn.Txn.CloseRemainderTo))
		participants = participate(participants, copyAddr(stxn.Txn.AssetSender))
		participants = participate(participants, copyAddr(stxn.Txn.AssetReceiver))
		participants = participate(participants, copyAddr(stxn.Txn.AssetCloseTo))
		participants = participate(participants, copyAddr(stxn.Txn.FreezeAccount))
		num++
		var tempNum int
		tempNum, participants = countInnerAndAddParticipation(stxn.EvalDelta.InnerTxns, participants)
		num += tempNum
	}
	return num, participants
}

// ImportBlock processes a block and adds it to the IndexerDb
func (imp *dbImporter) ImportDecodedBlock(blockContainer *types.EncodedBlockCert) (txCount int, err error) {
	txCount = 0
	proto, err := types.Protocol(string(blockContainer.Block.CurrentProtocol))
	if err != nil {
		return txCount, fmt.Errorf("block %d, %v", blockContainer.Block.Round, err)
	}
	err = imp.db.StartBlock()
	if err != nil {
		return txCount, fmt.Errorf("error starting block, %v", err)
	}
	block := blockContainer.Block
	round := uint64(block.Round)
	intra := 0

	getAssetID := func(txtypeenum idb.TxnTypeEnum, stxn *types.SignedTxnWithAD) uint64 {
		assetid := uint64(0)
		switch txtypeenum {
		case 3:
			assetid = uint64(stxn.Txn.ConfigAsset)
			if assetid == 0 {
				assetid = uint64(stxn.ApplyData.ConfigAsset)
			}
			if assetid == 0 {
				assetid = block.TxnCounter - uint64(len(block.Payset)) + uint64(intra) + 1
			}
		case 4:
			assetid = uint64(stxn.Txn.XferAsset)
		case 5:
			assetid = uint64(stxn.Txn.FreezeAsset)
		case 6:
			assetid = uint64(stxn.Txn.ApplicationID)
			if assetid == 0 {
				assetid = uint64(stxn.ApplyData.ApplicationID)
			}
			if assetid == 0 {
				assetid = block.TxnCounter - uint64(len(block.Payset)) + uint64(intra) + 1
			}
		}
		return assetid
	}

	for i := range block.Payset {
		stxn := &block.Payset[i]
		txtypeenum, ok := idb.GetTypeEnum(stxn.Txn.Type)
		if !ok {
			return txCount,
				fmt.Errorf("%d:%d unknown txn type %v", round, intra, stxn.Txn.Type)
		}
		assetid := getAssetID(txtypeenum, &stxn.SignedTxnWithAD)
		if stxn.HasGenesisID {
			stxn.Txn.GenesisID = block.GenesisID
		}
		if stxn.HasGenesisHash || proto.RequireGenesisHash {
			stxn.Txn.GenesisHash = block.GenesisHash
		}
		stxnad := stxn.SignedTxnWithAD
		participants := make([][]byte, 0, 10)
		participants = participate(participants, stxn.Txn.Sender[:])
		participants = participate(participants, stxn.Txn.Receiver[:])
		participants = participate(participants, stxn.Txn.CloseRemainderTo[:])
		participants = participate(participants, stxn.Txn.AssetSender[:])
		participants = participate(participants, stxn.Txn.AssetReceiver[:])
		participants = participate(participants, stxn.Txn.AssetCloseTo[:])
		participants = participate(participants, stxn.Txn.FreezeAccount[:])

		_, participants = countInnerAndAddParticipation(stxn.EvalDelta.InnerTxns, participants)
		err = imp.db.AddTransaction(
			round, intra, int(txtypeenum), assetid, stxnad, participants)
		intra++
		if err != nil {
			return txCount, fmt.Errorf("error importing txn r=%d i=%d, %v", round, intra, err)
		}

		// Add one level of inner transaction.
		for _, innerTxn := range stxn.ApplyData.EvalDelta.InnerTxns {
			// Basic AVM 1.0 support only.
			if len(innerTxn.ApplyData.EvalDelta.InnerTxns) != 0 {
				return txCount, fmt.Errorf("error importing txn r=%d i=%d, only one layer of inner transactions is supported", round, intra)
			}

			txtypeenum, ok := idb.GetTypeEnum(innerTxn.Txn.Type)
			if !ok {
				return txCount,
					fmt.Errorf("%d:%d unknown txn type %v", round, intra, innerTxn.Txn.Type)
			}
			assetid := getAssetID(txtypeenum, &innerTxn)

			err = imp.db.AddTransaction(
				round, intra, int(txtypeenum), assetid, innerTxn, nil)
			if err != nil {
				return txCount, fmt.Errorf("error importing txn r=%d i=%d, %v", round, intra, err)
			}

			// Incrementing intra for the inner txn allow upgrading directly to 2.7.2
			intra++
		}

		txCount++
	}
	blockheaderBytes := msgpack.Encode(block.BlockHeader)
	err = imp.db.CommitBlock(round, block.TimeStamp, block.RewardsLevel, blockheaderBytes)
	if err != nil {
		return txCount, fmt.Errorf("error committing block, %v", err)
	}
	return
}

// NewDBImporter creates a new importer object.
func NewDBImporter(db idb.IndexerDb) Importer {
	return &dbImporter{db: db}
}
